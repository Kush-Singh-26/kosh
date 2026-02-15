package renderer

import (
	"io"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (r *Renderer) RenderPage(path string, data models.PageData) {
	data.Assets = r.GetAssets()

	if err := r.DestFs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		r.logger.Error("Failed to create directory", "path", path, "error", err)
		return
	}

	f, err := r.DestFs.Create(path)
	if err != nil {
		r.logger.Error("Failed to create file", "path", path, "error", err)
		return
	}
	defer func() { _ = f.Close() }()

	bw := utils.SharedBufioWriterPool.Get(f)
	defer func() {
		_ = bw.Flush()
		utils.SharedBufioWriterPool.Put(bw)
	}()

	var w io.Writer = bw

	if r.Compress {
		mw := utils.Minifier.Writer("text/html", bw)
		defer func() { _ = mw.Close() }()
		w = mw
	}

	if err := r.Layout.Execute(w, data); err != nil {
		r.logger.Error("Failed to render layout", "path", path, "error", err)
	} else {
		r.RegisterFile(path)
	}
}
