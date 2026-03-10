package renderer

import (
	"io"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (r *Renderer) RenderPage(path string, data models.PageData) {
	r.PreparePageData(&data)

	// Directory creation is handled by WriteStream → ensureDir (with dirCache).
	// No explicit MkdirAll needed here — avoids redundant uncached syscalls.

	r.mu.RLock()
	layout := r.Layout
	r.mu.RUnlock()

	if layout == nil {
		r.logger.Error("Layout template not loaded", "path", path)
		return
	}

	// 1. Execute template to buffer
	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	if err := layout.Execute(buf, data); err != nil {
		r.logger.Error("Failed to render layout", "path", path, "error", err)
		return
	}

	// 2. Process HTML (Fix images and internal paths) - Legacy regex post-pass
	var finalBytes []byte
	if r.EnableLegacyProcessHTML {
		finalBytes = utils.ProcessHTMLBytes(buf.Bytes(), data.BaseURL, data.RelativePrefix, r.Compress)
	} else {
		finalBytes = buf.Bytes()
	}

	// 3. Optional Minification
	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	// 4. Final Write
	err := r.Sink.WriteStream(path, func(w io.Writer) error {
		_, err := w.Write(finalBytes)
		return err
	})

	if err != nil {
		r.logger.Error("Failed to write processed HTML", "path", path, "error", err)
		r.recordError("Failed to write processed HTML", path, err)
	} else {
		r.RegisterFile(path)
	}
}
