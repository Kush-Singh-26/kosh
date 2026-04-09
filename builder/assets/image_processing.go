package assets

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/spf13/afero"
	"golang.org/x/image/draw"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	koshMinify "github.com/Kush-Singh-26/kosh/builder/minify"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

const (
	smallImageResizeThresholdBytes int64 = 32 * 1024
	maxResizeWidth                       = 1200
	maxResizeHeight                      = 1600
	rgbaBytesPerPixel                    = 4
	minWebPQuality                       = 1
	maxWebPQuality                       = 100
	defaultWebPQuality                   = 80
	retryWriteAttempts                   = 3
	retryWriteDelay                      = 10 * time.Millisecond
)

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// ImageMetrics records image processing metrics.
type ImageMetrics interface {
	RecordImageOptimization(original, optimized int64)
	RecordImageResizeSkipped()
	IncrementAssetsProcessed()
	IncrementSVGsMinified()
}

// ProcessImageOptions configures image processing or copying.
type ProcessImageOptions struct {
	Ctx       context.Context
	SrcFs     afero.Fs
	Sink      fspkg.ArtifactSink
	SrcPath   string
	DstPath   string
	RelPath   string
	SrcInfo   fs.FileInfo
	Opts      CopyOptions
	Scheduler scheduler.BuildScheduler
}

// MaybeCopyOriginalOptions configures optional source image copying.
type MaybeCopyOriginalOptions struct {
	SrcFs        afero.Fs
	Sink         fspkg.ArtifactSink
	SrcPath      string
	DstWebp      string
	SrcInfo      fs.FileInfo
	OnWrite      func(string)
	KeepOriginal bool
}

func maybeCopyOriginal(opts MaybeCopyOriginalOptions) error {
	if !opts.KeepOriginal {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(opts.SrcPath))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil
	}
	if strings.ToLower(filepath.Ext(opts.DstWebp)) != ".webp" {
		return nil
	}
	origDst := strings.TrimSuffix(opts.DstWebp, filepath.Ext(opts.DstWebp)) + ext
	modTime := int64(0)
	if opts.SrcInfo != nil {
		modTime = opts.SrcInfo.ModTime().UnixNano()
	}
	return fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   opts.SrcFs,
		Sink:    opts.Sink,
		SrcPath: opts.SrcPath,
		DstPath: origDst,
		ModTime: modTime,
		OnWrite: opts.OnWrite,
	})
}

func isConvertibleImage(ext string) bool {
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png"
}

func copyFileWithMetrics(opts ProcessImageOptions) error {
	modTime := int64(0)
	if opts.SrcInfo != nil {
		modTime = opts.SrcInfo.ModTime().UnixNano()
	}
	err := fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   opts.SrcFs,
		Sink:    opts.Sink,
		SrcPath: opts.SrcPath,
		DstPath: opts.DstPath,
		ModTime: modTime,
		OnWrite: opts.Opts.OnWrite,
	})
	if err == nil && !isNil(opts.Opts.Metrics) {
		opts.Opts.Metrics.IncrementAssetsProcessed()
	}
	return err
}

func convertImageToWebP(opts ProcessImageOptions) error {
	dstPath := opts.DstPath[:len(opts.DstPath)-len(filepath.Ext(opts.DstPath))] + ".webp"
	newOpts := opts
	newOpts.DstPath = dstPath
	if opts.Scheduler == nil {
		newOpts.Scheduler = opts.Opts.Scheduler
	}
	if err := convertToWebPVFS(newOpts); err != nil {
		return err
	}
	if opts.Opts.OnWrite != nil {
		opts.Opts.OnWrite(dstPath)
	}
	return nil
}

func maybeMinifySVG(opts ProcessImageOptions) (bool, error) {
	if !opts.Opts.MinifySVGs || strings.ToLower(filepath.Ext(opts.SrcPath)) != ".svg" {
		return false, nil
	}

	sched := opts.Scheduler
	if sched == nil {
		sched = opts.Opts.Scheduler
	}
	if sched != nil {
		if err := sched.Acquire(opts.Ctx, scheduler.TaskDefault); err != nil {
			return true, err
		}
		defer sched.Release(scheduler.TaskDefault)
	}

	data, err := afero.ReadFile(opts.SrcFs, opts.SrcPath)
	if err != nil {
		return false, nil
	}
	m := koshMinify.GetHTMLMinifier()
	minified, err := m.Bytes("image/svg+xml", data)
	if err != nil {
		return false, nil
	}
	if !isNil(opts.Opts.Metrics) {
		opts.Opts.Metrics.IncrementSVGsMinified()
		opts.Opts.Metrics.IncrementAssetsProcessed()
	}
	if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
		return true, err
	}
	if err := opts.Sink.WriteFile(opts.DstPath, minified); err == nil {
		if opts.Opts.OnWrite != nil {
			opts.Opts.OnWrite(opts.DstPath)
		}
		return true, nil
	}

	return false, nil
}

// CopyFileWithOptionalImageProcessing copies or converts an image based on options.
func CopyFileWithOptionalImageProcessing(opts ProcessImageOptions) error {
	ext := strings.ToLower(filepath.Ext(opts.SrcPath))
	if opts.Opts.Compress && isConvertibleImage(ext) {
		return convertImageToWebP(opts)
	}

	if handled, err := maybeMinifySVG(opts); handled {
		return err
	}

	return copyFileWithMetrics(opts)
}

// ProcessCacheMissImage processes an image that is known to not be in any cache.
// Called from the asset service background image workers.
// Skips cache lookups (memory + disk) and goes directly to decode/encode.
func ProcessCacheMissImage(opts ProcessImageOptions) error {
	return convertToWebPVFS(opts)
}

func retryWriteFile(sink fspkg.ArtifactSink, path string, data []byte) error {
	var lastErr error
	for i := 0; i < retryWriteAttempts; i++ {
		if err := sink.WriteFile(path, data); err == nil {
			return nil
		} else {
			lastErr = err
			if i < retryWriteAttempts-1 {
				select {
				case <-time.After(retryWriteDelay):
				default:
				}
			}
		}
	}
	return fmt.Errorf("failed to write file after retries: %w", lastErr)
}

func ensureSrcInfo(opts *ProcessImageOptions) error {
	if opts.SrcInfo != nil {
		return nil
	}
	info, err := opts.SrcFs.Stat(opts.SrcPath)
	if err != nil {
		return fmt.Errorf("failed to stat source image %s: %w", opts.SrcPath, err)
	}
	opts.SrcInfo = info
	return nil
}

func registerWebPRelPath(relPath string) {
	if relPath == "" {
		return
	}
	relSrc := "/" + strings.TrimPrefix(filepath.ToSlash(relPath), "/")
	relDst := relSrc[:len(relSrc)-len(filepath.Ext(relSrc))] + ".webp"
	registerImageVariants(relSrc, relDst)
}

func maybeCopyOriginalBestEffort(opts ProcessImageOptions) {
	_ = maybeCopyOriginal(MaybeCopyOriginalOptions{
		SrcFs:        opts.SrcFs,
		Sink:         opts.Sink,
		SrcPath:      opts.SrcPath,
		DstWebp:      opts.DstPath,
		SrcInfo:      opts.SrcInfo,
		OnWrite:      opts.Opts.OnWrite,
		KeepOriginal: opts.Opts.KeepOriginal,
	})
}

func tryMemoryCache(opts ProcessImageOptions, key imageCacheKey) (bool, error) {
	cached, ok := GetImageCache().get(key)
	if !ok {
		return false, nil
	}
	if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
		return true, fmt.Errorf("failed to create image directory: %w", err)
	}
	if !isNil(opts.Opts.Metrics) {
		opts.Opts.Metrics.RecordImageOptimization(opts.SrcInfo.Size(), int64(len(cached)))
		opts.Opts.Metrics.IncrementAssetsProcessed()
	}
	err := retryWriteFile(opts.Sink, opts.DstPath, cached)
	if err == nil {
		registerWebPRelPath(opts.RelPath)
		maybeCopyOriginalBestEffort(opts)
	}
	return true, err
}

func resolveCacheFile(opts ProcessImageOptions, key imageCacheKey) string {
	if opts.Opts.CacheDir == "" {
		return ""
	}
	hashStr := getImageHash(key)
	return filepath.Join(opts.Opts.CacheDir, hashStr+".webp")
}

func tryDiskCache(opts ProcessImageOptions, cacheFile string, key imageCacheKey) (bool, error) {
	if cacheFile == "" {
		return false, nil
	}
	cacheFs := afero.NewOsFs()
	cacheInfo, err := cacheFs.Stat(cacheFile)
	if err != nil || cacheInfo.IsDir() {
		return false, nil
	}

	f, err := cacheFs.Open(cacheFile)
	if err != nil {
		return true, fmt.Errorf("failed to open cached image %s: %w", cacheFile, err)
	}
	defer func() { _ = f.Close() }()

	if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
		return true, fmt.Errorf("failed to create image directory: %w", err)
	}

	cachedData, readErr := afero.ReadAll(f)
	if readErr != nil {
		return true, fmt.Errorf("failed to read cached image %s: %w", cacheFile, readErr)
	}
	GetImageCache().set(key, cachedData)
	if !isNil(opts.Opts.Metrics) {
		opts.Opts.Metrics.RecordImageOptimization(opts.SrcInfo.Size(), int64(len(cachedData)))
		opts.Opts.Metrics.IncrementAssetsProcessed()
	}
	if err := opts.Sink.WriteFile(opts.DstPath, cachedData); err != nil {
		return true, fmt.Errorf("failed to write cached image %s: %w", opts.DstPath, err)
	}
	registerWebPRelPath(opts.RelPath)
	maybeCopyOriginalBestEffort(opts)

	_ = opts.Sink.SetMtime(opts.DstPath, opts.SrcInfo.ModTime())
	return true, nil
}

func decodeSourceImage(srcFs afero.Fs, srcPath string) (image.Image, error) {
	f, err := srcFs.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image %s: %w", srcPath, err)
	}
	defer func() { _ = f.Close() }()

	br := pools.SharedBufioReaderPool.Get(f)
	defer func() {
		br.Reset(nil)
		pools.SharedBufioReaderPool.Put(br)
	}()

	src, _, err := image.Decode(br)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image %s: %w", srcPath, err)
	}
	return src, nil
}

func resizeImageIfNeeded(src image.Image, skipResize bool, metrics ImageMetrics) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width > maxResizeWidth && !skipResize {
		newWidth := maxResizeWidth
		newHeight := (height * newWidth) / width

		neededSize := newWidth * newHeight * rgbaBytesPerPixel
		var pix []byte
		var pixPtr *[]byte
		if neededSize <= maxResizeWidth*maxResizeHeight*rgbaBytesPerPixel {
			pixPtr = rgbaPixPool.Get().(*[]byte)
			pix = *pixPtr
			defer rgbaPixPool.Put(pixPtr)
		} else {
			pix = make([]byte, neededSize)
		}

		dst := &image.RGBA{
			Pix:    pix[:neededSize],
			Stride: newWidth * rgbaBytesPerPixel,
			Rect:   image.Rect(0, 0, newWidth, newHeight),
		}

		draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		return dst
	}

	if skipResize {
		if _, isYCbCr := src.(*image.YCbCr); isYCbCr {
			b := src.Bounds()
			rgba := image.NewRGBA(b)
			draw.Draw(rgba, b, src, b.Min, draw.Src)
			src = rgba
		}
		if !isNil(metrics) {
			metrics.RecordImageResizeSkipped()
		}
	}
	return src
}

func resolveWebPQuality(quality int) int {
	if quality < minWebPQuality || quality > maxWebPQuality {
		return defaultWebPQuality
	}
	return quality
}

func encodeWebP(img image.Image, quality int) ([]byte, error) {
	buf := webpBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer webpBufferPool.Put(buf)

	if err := webp.Encode(buf, img, &webp.Options{Lossless: false, Quality: float32(quality)}); err != nil {
		return nil, err
	}

	encodedData := buf.Bytes()
	cacheData := make([]byte, len(encodedData))
	copy(cacheData, encodedData)
	return cacheData, nil
}

func writeEncodedWebP(opts ProcessImageOptions, key imageCacheKey, cacheFile string, cacheData []byte) error {
	GetImageCache().set(key, cacheData)
	if cacheFile != "" {
		queueImageCacheWrite(cacheFile, cacheData, true)
	}

	if !isNil(opts.Opts.Metrics) {
		opts.Opts.Metrics.RecordImageOptimization(opts.SrcInfo.Size(), int64(len(cacheData)))
	}

	if err := opts.Sink.WriteFile(opts.DstPath, cacheData); err != nil {
		return err
	}
	_ = opts.Sink.SetMtime(opts.DstPath, opts.SrcInfo.ModTime())
	if !isNil(opts.Opts.Metrics) {
		opts.Opts.Metrics.IncrementAssetsProcessed()
	}
	registerWebPRelPath(opts.RelPath)
	maybeCopyOriginalBestEffort(opts)
	return nil
}

func convertToWebPVFS(opts ProcessImageOptions) error {
	if err := ensureSrcInfo(&opts); err != nil {
		return err
	}
	skipResize := opts.SrcInfo.Size() <= smallImageResizeThresholdBytes

	memCacheKey := imageCacheKey{
		path:    opts.SrcPath,
		size:    opts.SrcInfo.Size(),
		modTime: opts.SrcInfo.ModTime().UnixNano(),
	}

	if ok, err := tryMemoryCache(opts, memCacheKey); ok {
		return err
	}

	cacheFile := resolveCacheFile(opts, memCacheKey)
	if ok, err := tryDiskCache(opts, cacheFile, memCacheKey); ok {
		return err
	}

	select {
	case <-opts.Ctx.Done():
		return opts.Ctx.Err()
	default:
	}

	sched := opts.Scheduler
	if sched == nil {
		sched = opts.Opts.Scheduler
	}
	if sched != nil {
		if err := sched.Acquire(opts.Ctx, scheduler.TaskImage); err != nil {
			return err
		}
		defer sched.Release(scheduler.TaskImage)
	}

	src, err := decodeSourceImage(opts.SrcFs, opts.SrcPath)
	if err != nil {
		return err
	}

	finalImg := resizeImageIfNeeded(src, skipResize, opts.Opts.Metrics)

	if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	webpQuality := resolveWebPQuality(opts.Opts.WebPQuality)
	cacheData, err := encodeWebP(finalImg, webpQuality)
	if err != nil {
		return fmt.Errorf("failed to encode webp %s: %w", opts.DstPath, err)
	}

	return writeEncodedWebP(opts, memCacheKey, cacheFile, cacheData)
}
