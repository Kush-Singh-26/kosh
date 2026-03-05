package renderer

import (
	"bytes"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (r *Renderer) RenderPage(path string, data models.PageData) {
	if data.Assets == nil {
		data.Assets = r.GetAssets()
	}

	if err := r.DestFs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		r.logger.Error("Failed to create directory", "path", path, "error", err)
		return
	}

	f, err := r.DestFs.Create(path)
	if err != nil {
		r.logger.Error("Failed to create file", "path", path, "error", err)
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			r.recordError("Failed to close file", path, err)
		}
	}()

	bw := utils.SharedBufioWriterPool.Get(f)
	defer func() {
		if err := bw.Flush(); err != nil {
			r.recordError("Failed to flush buffer", path, err)
		}
		utils.SharedBufioWriterPool.Put(bw)
	}()

	r.mu.RLock()
	layout := r.Layout
	r.mu.RUnlock()

	if layout == nil {
		r.logger.Error("Layout template not loaded", "path", path)
		return
	}

	// 1. Execute template to buffer
	var buf bytes.Buffer
	if err := layout.Execute(&buf, data); err != nil {
		r.logger.Error("Failed to render layout", "path", path, "error", err)
		return
	}

	// 2. Process HTML (Fix images and internal paths)
	processedHTML := utils.ProcessHTML(buf.String(), data.BaseURL, data.RelativePrefix, r.Compress)

	// 3. Optional Minification
	finalBytes := []byte(processedHTML)
	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	// 4. Final Write
	if _, err := bw.Write(finalBytes); err != nil {
		r.logger.Error("Failed to write processed HTML", "path", path, "error", err)
	} else {
		r.RegisterFile(path)
	}
}
