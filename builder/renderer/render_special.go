package renderer

import (
	"bufio"
	"io"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (r *Renderer) RenderIndex(path string, data models.PageData) {
	data.Assets = r.Assets

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

	bw := bufio.NewWriterSize(f, utils.MaxBufferSize)
	defer func() { _ = bw.Flush() }()

	var w io.Writer = bw

	if r.Compress {
		mw := utils.Minifier.Writer("text/html", bw)
		defer func() { _ = mw.Close() }()
		w = mw
	}

	var errExec error
	if r.Index != nil {
		errExec = r.Index.Execute(w, data)
	} else {
		errExec = r.Layout.Execute(w, data)
	}
	if errExec != nil {
		r.logger.Error("Failed to render index", "path", path, "error", errExec)
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) RenderGraph(path string, data models.PageData) {
	if r.Graph == nil {
		return
	}
	data.Assets = r.Assets

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

	bw := bufio.NewWriterSize(f, utils.MaxBufferSize)
	defer func() { _ = bw.Flush() }()

	var w io.Writer = bw
	if r.Compress {
		mw := utils.Minifier.Writer("text/html", bw)
		defer func() { _ = mw.Close() }()
		w = mw
	}

	if err := r.Graph.Execute(w, data); err != nil {
		r.logger.Error("Failed to render graph", "path", path, "error", err)
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) Render404(path string, data models.PageData) {
	data.Assets = r.Assets

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

	bw := bufio.NewWriterSize(f, utils.MaxBufferSize)
	defer func() { _ = bw.Flush() }()

	var w io.Writer = bw

	if r.Compress {
		mw := utils.Minifier.Writer("text/html", bw)
		defer func() { _ = mw.Close() }()
		w = mw
	}

	var errExec error
	if r.NotFound != nil {
		errExec = r.NotFound.Execute(w, data)
	} else {
		errExec = r.Layout.Execute(w, data)
	}
	if errExec != nil {
		r.logger.Error("Failed to render 404", "path", path, "error", errExec)
	} else {
		r.RegisterFile(path)
	}
}
