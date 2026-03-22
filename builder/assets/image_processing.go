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

	"github.com/chai2010/webp"
	"github.com/spf13/afero"
	"golang.org/x/image/draw"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	koshMinify "github.com/Kush-Singh-26/kosh/builder/minify"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

const smallImageResizeThresholdBytes int64 = 32 * 1024

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

type ImageMetrics interface {
	RecordImageOptimization(original, optimized int64)
	RecordImageResizeSkipped()
	IncrementAssetsProcessed()
	IncrementSVGsMinified()
}

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

func CopyFileWithOptionalImageProcessing(opts ProcessImageOptions) error {
	ext := strings.ToLower(filepath.Ext(opts.SrcPath))
	isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png"
	if opts.Opts.Compress && isImage {
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

		if opts.RelPath != "" {
			relSrc := "/" + strings.TrimPrefix(filepath.ToSlash(opts.RelPath), "/")
			relDst := relSrc[:len(relSrc)-len(filepath.Ext(relSrc))] + ".webp"
			RecordConvertedImage(relSrc, relDst)
		}
		RecordConvertedImage(opts.DstPath, dstPath)
		return nil
	}

	if opts.Opts.MinifySVGs && ext == ".svg" {
		sched := opts.Scheduler
		if sched == nil {
			sched = opts.Opts.Scheduler
		}
		if sched != nil {
			if err := sched.Acquire(opts.Ctx, scheduler.TaskDefault); err != nil {
				return err
			}
			defer sched.Release(scheduler.TaskDefault)
		}

		data, err := afero.ReadFile(opts.SrcFs, opts.SrcPath)
		if err == nil {
			m := koshMinify.GetHTMLMinifier()
			minified, err := m.Bytes("image/svg+xml", data)
			if err == nil {
				if !isNil(opts.Opts.Metrics) {
					opts.Opts.Metrics.IncrementSVGsMinified()
					opts.Opts.Metrics.IncrementAssetsProcessed()
				}
				if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
					return err
				}
				if err := opts.Sink.WriteFile(opts.DstPath, minified); err == nil {
					if opts.Opts.OnWrite != nil {
						opts.Opts.OnWrite(opts.DstPath)
					}
					return nil
				}
			}
		}
	}

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

func convertToWebPVFS(opts ProcessImageOptions) error {
	if opts.SrcInfo == nil {
		var err error
		opts.SrcInfo, err = opts.SrcFs.Stat(opts.SrcPath)
		if err != nil {
			return fmt.Errorf("failed to stat source image %s: %w", opts.SrcPath, err)
		}
	}

	skipResize := opts.SrcInfo.Size() <= smallImageResizeThresholdBytes

	memCacheKey := imageCacheKey{
		path:    opts.SrcPath,
		size:    opts.SrcInfo.Size(),
		modTime: opts.SrcInfo.ModTime().UnixNano(),
	}

	if cached, ok := GetImageCache().get(memCacheKey); ok {
		if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
			return fmt.Errorf("failed to create image directory: %w", err)
		}
		if !isNil(opts.Opts.Metrics) {
			opts.Opts.Metrics.RecordImageOptimization(opts.SrcInfo.Size(), int64(len(cached)))
			opts.Opts.Metrics.IncrementAssetsProcessed()
		}
		return opts.Sink.WriteFile(opts.DstPath, cached)
	}

	var cacheFile string
	cacheFs := afero.NewOsFs()
	if opts.Opts.CacheDir != "" {
		hashStr := getImageHash(memCacheKey)
		cacheFile = filepath.Join(opts.Opts.CacheDir, hashStr+".webp")

		if cacheInfo, err := cacheFs.Stat(cacheFile); err == nil && !cacheInfo.IsDir() {
			f, err := cacheFs.Open(cacheFile)
			if err != nil {
				return fmt.Errorf("failed to open cached image %s: %w", cacheFile, err)
			}
			defer func() { _ = f.Close() }()

			if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
				return fmt.Errorf("failed to create image directory: %w", err)
			}

			cachedData, readErr := afero.ReadAll(f)
			if readErr == nil {
				GetImageCache().set(memCacheKey, cachedData)
				if !isNil(opts.Opts.Metrics) {
					opts.Opts.Metrics.RecordImageOptimization(opts.SrcInfo.Size(), int64(len(cachedData)))
					opts.Opts.Metrics.IncrementAssetsProcessed()
				}
				if err := opts.Sink.WriteFile(opts.DstPath, cachedData); err != nil {
					return fmt.Errorf("failed to write cached image %s: %w", opts.DstPath, err)
				}
			} else {
				return fmt.Errorf("failed to read cached image %s: %w", cacheFile, readErr)
			}

			_ = opts.Sink.SetMtime(opts.DstPath, opts.SrcInfo.ModTime())
			return nil
		}
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

	f, err := opts.SrcFs.Open(opts.SrcPath)
	if err != nil {
		return fmt.Errorf("failed to open image %s: %w", opts.SrcPath, err)
	}
	defer func() { _ = f.Close() }()

	br := pools.SharedBufioReaderPool.Get(f)
	defer func() {
		br.Reset(nil)
		pools.SharedBufioReaderPool.Put(br)
	}()

	src, _, err := image.Decode(br)
	if err != nil {
		return fmt.Errorf("failed to decode image %s: %w", opts.SrcPath, err)
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	finalImg := src
	if width > 1200 && !skipResize {
		newWidth := 1200
		newHeight := (height * newWidth) / width

		neededSize := newWidth * newHeight * 4
		var pix []byte
		var pixPtr *[]byte
		if neededSize <= 1200*1600*4 {
			pixPtr = rgbaPixPool.Get().(*[]byte)
			pix = *pixPtr
			defer rgbaPixPool.Put(pixPtr)
		} else {
			pix = make([]byte, neededSize)
		}

		dst := &image.RGBA{
			Pix:    pix[:neededSize],
			Stride: newWidth * 4,
			Rect:   image.Rect(0, 0, newWidth, newHeight),
		}

		draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		finalImg = dst
	} else if skipResize {
		if _, isYCbCr := finalImg.(*image.YCbCr); isYCbCr {
			b := finalImg.Bounds()
			rgba := image.NewRGBA(b)
			draw.Draw(rgba, b, finalImg, b.Min, draw.Src)
			finalImg = rgba
		}
		if !isNil(opts.Opts.Metrics) {
			opts.Opts.Metrics.RecordImageResizeSkipped()
		}
	}

	if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
		return fmt.Errorf("failed to create image directory: %w", err)
	}

	webpQuality := opts.Opts.WebPQuality
	if webpQuality < 1 || webpQuality > 100 {
		webpQuality = 80
	}

	buf := webpBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer webpBufferPool.Put(buf)

	err = webp.Encode(buf, finalImg, &webp.Options{Lossless: false, Quality: float32(webpQuality)})
	if err != nil {
		return fmt.Errorf("failed to encode webp %s: %w", opts.DstPath, err)
	}
	encodedData := buf.Bytes()

	cacheData := make([]byte, len(encodedData))
	copy(cacheData, encodedData)
	GetImageCache().set(memCacheKey, cacheData)

	if cacheFile != "" {
		queueImageCacheWrite(cacheFile, cacheData, true)
	}

	if !isNil(opts.Opts.Metrics) {
		opts.Opts.Metrics.RecordImageOptimization(opts.SrcInfo.Size(), int64(len(cacheData)))
	}

	err = opts.Sink.WriteFile(opts.DstPath, cacheData)
	if err == nil {
		_ = opts.Sink.SetMtime(opts.DstPath, opts.SrcInfo.ModTime())
		if !isNil(opts.Opts.Metrics) {
			opts.Opts.Metrics.IncrementAssetsProcessed()
		}
	}

	return err
}
