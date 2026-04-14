//go:build !wasm

package assets

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
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

var rgbaPixPool = pools.SharedImageSlicePool

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflectValue := reflect.ValueOf(value)
	return reflectValue.Kind() == reflect.Pointer && reflectValue.IsNil()
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

func maybeCopyOriginal(options MaybeCopyOriginalOptions) error {
	if !options.KeepOriginal {
		return nil
	}
	extension := strings.ToLower(filepath.Ext(options.SrcPath))
	if extension != ".jpg" && extension != ".jpeg" && extension != ".png" {
		return nil
	}
	if strings.ToLower(filepath.Ext(options.DstWebp)) != ".webp" {
		return nil
	}
	originalDestination := strings.TrimSuffix(options.DstWebp, filepath.Ext(options.DstWebp)) + extension
	modTime := int64(0)
	if options.SrcInfo != nil {
		modTime = options.SrcInfo.ModTime().UnixNano()
	}
	return fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   options.SrcFs,
		Sink:    options.Sink,
		SrcPath: options.SrcPath,
		DstPath: originalDestination,
		ModTime: modTime,
		OnWrite: options.OnWrite,
	})
}

func isConvertibleImage(ext string) bool {
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png"
}

func copyFileWithMetrics(options ProcessImageOptions) error {
	modTime := int64(0)
	if options.SrcInfo != nil {
		modTime = options.SrcInfo.ModTime().UnixNano()
	}
	err := fspkg.CopyFileVFS(fspkg.CopyFileOptions{
		SrcFs:   options.SrcFs,
		Sink:    options.Sink,
		SrcPath: options.SrcPath,
		DstPath: options.DstPath,
		ModTime: modTime,
		OnWrite: options.Opts.OnWrite,
	})
	if err == nil && !isNil(options.Opts.Metrics) {
		options.Opts.Metrics.IncrementAssetsProcessed()
	}
	return err
}

func convertImageToWebP(options ProcessImageOptions) error {
	destinationPath := options.DstPath[:len(options.DstPath)-len(filepath.Ext(options.DstPath))] + ".webp"
	newOptions := options
	newOptions.DstPath = destinationPath
	if options.Scheduler == nil {
		newOptions.Scheduler = options.Opts.Scheduler
	}
	if err := convertToWebPVFS(newOptions); err != nil {
		return err
	}
	if options.Opts.OnWrite != nil {
		options.Opts.OnWrite(destinationPath)
	}
	return nil
}

func getSVGCachePath(srcPath string, size int64, modTime int64) string {
	hash := xxh3.Hash128([]byte(srcPath + strconv.FormatInt(size, 10) + strconv.FormatInt(modTime, 10)))
	hashBytes := hash.Bytes()
	return hex.EncodeToString(hashBytes[:])
}

func maybeMinifySVG(options ProcessImageOptions) (bool, error) {
	if !options.Opts.MinifySVGs || strings.ToLower(filepath.Ext(options.SrcPath)) != ".svg" {
		return false, nil
	}

	cacheDir := options.Opts.CacheDir
	if cacheDir != "" {
		cacheDir = filepath.Join(cacheDir, "svg-cache")
	}

	srcInfo := options.SrcInfo
	if srcInfo == nil {
		if f, err := options.SrcFs.Stat(options.SrcPath); err == nil {
			srcInfo = f
		}
	}
	if srcInfo == nil {
		return false, nil
	}

	if cacheDir != "" {
		cacheFile := filepath.Join(cacheDir, getSVGCachePath(options.SrcPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())+".svg")
		if cachedData, err := afero.ReadFile(afero.NewOsFs(), cacheFile); err == nil {
			if err := options.Sink.MkdirAll(filepath.Dir(options.DstPath)); err != nil {
				return true, err
			}
			if err := options.Sink.WriteFile(options.DstPath, cachedData); err == nil {
				if options.Opts.OnWrite != nil {
					options.Opts.OnWrite(options.DstPath)
				}
				return true, nil
			}
		}
	}

	buildScheduler := options.Scheduler
	if buildScheduler == nil {
		buildScheduler = options.Opts.Scheduler
	}
	if buildScheduler != nil {
		if err := buildScheduler.Acquire(options.Ctx, scheduler.TaskDefault); err != nil {
			return true, err
		}
		defer buildScheduler.Release(scheduler.TaskDefault)
	}

	data, err := afero.ReadFile(options.SrcFs, options.SrcPath)
	if err != nil {
		return false, nil
	}
	minifier := koshMinify.GetHTMLMinifier()
	minified, err := minifier.Bytes("image/svg+xml", data)
	if err != nil {
		return false, nil
	}
	if !isNil(options.Opts.Metrics) {
		options.Opts.Metrics.IncrementSVGsMinified()
		options.Opts.Metrics.IncrementAssetsProcessed()
	}
	if err := options.Sink.MkdirAll(filepath.Dir(options.DstPath)); err != nil {
		return true, err
	}
	if err := options.Sink.WriteFile(options.DstPath, minified); err == nil {
		if cacheDir != "" {
			cacheFile := filepath.Join(cacheDir, getSVGCachePath(options.SrcPath, srcInfo.Size(), srcInfo.ModTime().UnixNano())+".svg")
			_ = os.MkdirAll(filepath.Dir(cacheFile), 0755)
			_ = os.WriteFile(cacheFile, minified, 0644)
		}
		if options.Opts.OnWrite != nil {
			options.Opts.OnWrite(options.DstPath)
		}
		return true, nil
	}

	return false, nil
}

// CopyFileWithOptionalImageProcessing copies or converts an image based on options.
func CopyFileWithOptionalImageProcessing(options ProcessImageOptions) error {
	extension := strings.ToLower(filepath.Ext(options.SrcPath))
	if options.Opts.Compress && isConvertibleImage(extension) {
		return convertImageToWebP(options)
	}

	if handled, err := maybeMinifySVG(options); handled {
		return err
	}

	return copyFileWithMetrics(options)
}

// ProcessCacheMissImage processes an image that is known to not be in any cache.
// Called from the asset service background image workers.
// Skips cache lookups (memory + disk) and goes directly to decode/encode.
func ProcessCacheMissImage(options ProcessImageOptions) error {
	return convertToWebPVFS(options)
}

func retryWriteFile(sink fspkg.ArtifactSink, path string, data []byte) error {
	var lastErr error
	for index := 0; index < retryWriteAttempts; index++ {
		if err := sink.WriteFile(path, data); err == nil {
			return nil
		} else {
			lastErr = err
			if index < retryWriteAttempts-1 {
				select {
				case <-time.After(retryWriteDelay):
				default:
				}
			}
		}
	}
	return fmt.Errorf("failed to write file after retries: %w", lastErr)
}

func ensureSrcInfo(options *ProcessImageOptions) error {
	if options.SrcInfo != nil {
		return nil
	}
	info, err := options.SrcFs.Stat(options.SrcPath)
	if err != nil {
		return fmt.Errorf("failed to stat source image %s: %w", options.SrcPath, err)
	}
	options.SrcInfo = info
	return nil
}

func registerWebPRelPath(relPath string) {
	if relPath == "" {
		return
	}
	relativeSrc := "/" + strings.TrimPrefix(filepath.ToSlash(relPath), "/")
	relativeDst := relativeSrc[:len(relativeSrc)-len(filepath.Ext(relativeSrc))] + ".webp"
	registerImageVariants(relativeSrc, relativeDst)
}

func maybeCopyOriginalBestEffort(options ProcessImageOptions) {
	_ = maybeCopyOriginal(MaybeCopyOriginalOptions{
		SrcFs:        options.SrcFs,
		Sink:         options.Sink,
		SrcPath:      options.SrcPath,
		DstWebp:      options.DstPath,
		SrcInfo:      options.SrcInfo,
		OnWrite:      options.Opts.OnWrite,
		KeepOriginal: options.Opts.KeepOriginal,
	})
}

func tryMemoryCache(options ProcessImageOptions, key imageCacheKey) (bool, error) {
	cached, ok := GetImageCache().get(key)
	if !ok {
		return false, nil
	}
	if err := options.Sink.MkdirAll(filepath.Dir(options.DstPath)); err != nil {
		return true, fmt.Errorf("failed to create image directory: %w", err)
	}
	if !isNil(options.Opts.Metrics) {
		options.Opts.Metrics.RecordImageOptimization(options.SrcInfo.Size(), int64(len(cached)))
		options.Opts.Metrics.IncrementAssetsProcessed()
	}
	err := retryWriteFile(options.Sink, options.DstPath, cached)
	if err == nil {
		registerWebPRelPath(options.RelPath)
		maybeCopyOriginalBestEffort(options)
	}
	return true, err
}

func resolveCacheFile(options ProcessImageOptions, key imageCacheKey) string {
	if options.Opts.CacheDir == "" {
		return ""
	}
	hashStr := getImageHash(key)
	return filepath.Join(options.Opts.CacheDir, hashStr+".webp")
}

func tryDiskCache(options ProcessImageOptions, cacheFile string, key imageCacheKey) (bool, error) {
	if cacheFile == "" {
		return false, nil
	}
	cacheFs := afero.NewOsFs()
	cacheInfo, err := cacheFs.Stat(cacheFile)
	if err != nil || cacheInfo.IsDir() {
		return false, nil
	}

	file, err := cacheFs.Open(cacheFile)
	if err != nil {
		return true, fmt.Errorf("failed to open cached image %s: %w", cacheFile, err)
	}
	defer func() { _ = file.Close() }()

	if err := options.Sink.MkdirAll(filepath.Dir(options.DstPath)); err != nil {
		return true, fmt.Errorf("failed to create image directory: %w", err)
	}

	cachedData, readErr := afero.ReadAll(file)
	if readErr != nil {
		return true, fmt.Errorf("failed to read cached image %s: %w", cacheFile, readErr)
	}
	GetImageCache().set(key, cachedData)
	if !isNil(options.Opts.Metrics) {
		options.Opts.Metrics.RecordImageOptimization(options.SrcInfo.Size(), int64(len(cachedData)))
		options.Opts.Metrics.IncrementAssetsProcessed()
	}
	if err := options.Sink.WriteFile(options.DstPath, cachedData); err != nil {
		return true, fmt.Errorf("failed to write cached image %s: %w", options.DstPath, err)
	}
	registerWebPRelPath(options.RelPath)
	maybeCopyOriginalBestEffort(options)

	_ = options.Sink.SetMtime(options.DstPath, options.SrcInfo.ModTime())
	return true, nil
}

func decodeSourceImage(srcFs afero.Fs, srcPath string) (image.Image, error) {
	file, err := srcFs.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image %s: %w", srcPath, err)
	}
	defer func() { _ = file.Close() }()

	reader := pools.SharedBufioReaderPool.Get(file)
	defer func() {
		reader.Reset(nil)
		pools.SharedBufioReaderPool.Put(reader)
	}()

	srcImg, _, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image %s: %w", srcPath, err)
	}
	return srcImg, nil
}

func resizeImageIfNeeded(sourceImage image.Image, skipResize bool, metrics ImageMetrics) (image.Image, *[]byte) {
	bounds := sourceImage.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width > maxResizeWidth && !skipResize {
		newWidth := maxResizeWidth
		newHeight := (height * newWidth) / width

		neededSize := newWidth * newHeight * rgbaBytesPerPixel
		var pixelData []byte
		var pixelDataPointer *[]byte
		if neededSize <= maxResizeWidth*maxResizeHeight*rgbaBytesPerPixel {
			pixelDataPointer = rgbaPixPool.Get()
			pixelData = *pixelDataPointer
		} else {
			pixelData = make([]byte, neededSize)
		}

		destination := &image.RGBA{
			Pix:    pixelData[:neededSize],
			Stride: newWidth * rgbaBytesPerPixel,
			Rect:   image.Rect(0, 0, newWidth, newHeight),
		}

		draw.BiLinear.Scale(destination, destination.Bounds(), sourceImage, sourceImage.Bounds(), draw.Over, nil)
		return destination, pixelDataPointer
	}

	if skipResize {
		if !isNil(metrics) {
			metrics.RecordImageResizeSkipped()
		}
	}
	return sourceImage, nil
}

func resolveWebPQuality(quality int) int {
	if quality < minWebPQuality || quality > maxWebPQuality {
		return defaultWebPQuality
	}
	return quality
}

func encodeWebP(image image.Image, quality int) ([]byte, error) {
	buffer := webpBufferPool.Get().(*bytes.Buffer)
	buffer.Reset()
	defer webpBufferPool.Put(buffer)

	if err := webp.Encode(buffer, image, &webp.Options{Lossless: false, Quality: float32(quality)}); err != nil {
		return nil, err
	}

	encodedData := buffer.Bytes()
	cacheData := make([]byte, len(encodedData))
	copy(cacheData, encodedData)
	return cacheData, nil
}

func writeEncodedWebP(options ProcessImageOptions, key imageCacheKey, cacheFile string, cacheData []byte) error {
	GetImageCache().set(key, cacheData)
	if cacheFile != "" {
		queueImageCacheWrite(cacheFile, cacheData, true)
	}

	if !isNil(options.Opts.Metrics) {
		options.Opts.Metrics.RecordImageOptimization(options.SrcInfo.Size(), int64(len(cacheData)))
	}

	if err := options.Sink.WriteFile(options.DstPath, cacheData); err != nil {
		return err
	}
	_ = options.Sink.SetMtime(options.DstPath, options.SrcInfo.ModTime())
	if !isNil(options.Opts.Metrics) {
		options.Opts.Metrics.IncrementAssetsProcessed()
	}
	registerWebPRelPath(options.RelPath)
	maybeCopyOriginalBestEffort(options)
	return nil
}

func convertToWebPVFS(options ProcessImageOptions) error {
	if err := ensureSrcInfo(&options); err != nil {
		return err
	}
	skipResize := options.SrcInfo.Size() <= smallImageResizeThresholdBytes

	memCacheKey := imageCacheKey{
		path:    options.SrcPath,
		size:    options.SrcInfo.Size(),
		modTime: options.SrcInfo.ModTime().UnixNano(),
	}

	if ok, err := tryMemoryCache(options, memCacheKey); ok {
		return err
	}

	cacheFile := resolveCacheFile(options, memCacheKey)
	if ok, err := tryDiskCache(options, cacheFile, memCacheKey); ok {
		return err
	}

	select {
	case <-options.Ctx.Done():
		return options.Ctx.Err()
	default:
	}

	buildScheduler := options.Scheduler
	if buildScheduler == nil {
		buildScheduler = options.Opts.Scheduler
	}
	if buildScheduler != nil {
		if err := buildScheduler.Acquire(options.Ctx, scheduler.TaskImage); err != nil {
			return err
		}
		defer buildScheduler.Release(scheduler.TaskImage)
	}

	sourceImage, err := decodeSourceImage(options.SrcFs, options.SrcPath)
	if err != nil {
		return err
	}

	finalImg, pooledBuffer := resizeImageIfNeeded(sourceImage, skipResize, options.Opts.Metrics)
	if pooledBuffer != nil {
		defer rgbaPixPool.Put(pooledBuffer)
	}

	if err := options.Sink.MkdirAll(filepath.Dir(options.DstPath)); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	webpQuality := resolveWebPQuality(options.Opts.WebPQuality)
	cacheData, err := encodeWebP(finalImg, webpQuality)
	if err != nil {
		return fmt.Errorf("failed to encode webp %s: %w", options.DstPath, err)
	}

	return writeEncodedWebP(options, memCacheKey, cacheFile, cacheData)
}
