package renderer

import (
	"io"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (r *Renderer) RenderIndex(path string, data models.PageData) {
	r.PreparePageData(&data)

	r.mu.RLock()
	index := r.Index
	layout := r.Layout
	r.mu.RUnlock()

	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	var errExec error
	if index != nil {
		errExec = index.Execute(buf, data)
	} else if layout != nil {
		errExec = layout.Execute(buf, data)
	} else {
		r.logger.Error("No template available for index", "path", path)
		return
	}

	if errExec != nil {
		r.logger.Error("Failed to render index", "path", path, "error", errExec)
		return
	}

	// Process HTML
	var finalBytes []byte
	if r.EnableLegacyProcessHTML {
		finalBytes = utils.ProcessHTMLBytes(buf.Bytes(), data.BaseURL, data.RelativePrefix, r.Compress)
	} else {
		finalBytes = buf.Bytes()
	}

	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	errExec = r.Sink.WriteStream(path, func(w io.Writer) error {
		_, err := w.Write(finalBytes)
		return err
	})

	if errExec != nil {
		r.logger.Error("Failed to write processed index", "path", path, "error", errExec)
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) RenderGraph(path string, data models.PageData) {
	r.PreparePageData(&data)

	r.mu.RLock()
	graph := r.Graph
	r.mu.RUnlock()

	if graph == nil {
		return
	}

	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	if err := graph.Execute(buf, data); err != nil {
		r.logger.Error("Failed to render graph", "path", path, "error", err)
		return
	}

	// Process HTML
	var finalBytes []byte
	if r.EnableLegacyProcessHTML {
		finalBytes = utils.ProcessHTMLBytes(buf.Bytes(), data.BaseURL, data.RelativePrefix, r.Compress)
	} else {
		finalBytes = buf.Bytes()
	}

	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	errWrite := r.Sink.WriteStream(path, func(w io.Writer) error {
		_, err := w.Write(finalBytes)
		return err
	})

	if errWrite != nil {
		r.logger.Error("Failed to write processed graph", "path", path, "error", errWrite)
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) Render404(path string, data models.PageData) {
	r.PreparePageData(&data)

	r.mu.RLock()
	notFound := r.NotFound
	layout := r.Layout
	r.mu.RUnlock()

	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	var errExec error
	if notFound != nil {
		errExec = notFound.Execute(buf, data)
	} else if layout != nil {
		errExec = layout.Execute(buf, data)
	} else {
		r.logger.Error("No template available for 404", "path", path)
		return
	}

	if errExec != nil {
		r.logger.Error("Failed to render 404", "path", path, "error", errExec)
		return
	}

	// Process HTML
	var finalBytes []byte
	if r.EnableLegacyProcessHTML {
		finalBytes = utils.ProcessHTMLBytes(buf.Bytes(), data.BaseURL, data.RelativePrefix, r.Compress)
	} else {
		finalBytes = buf.Bytes()
	}

	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	errWrite := r.Sink.WriteStream(path, func(w io.Writer) error {
		_, err := w.Write(finalBytes)
		return err
	})

	if errWrite != nil {
		r.logger.Error("Failed to write processed 404", "path", path, "error", errWrite)
	} else {
		r.RegisterFile(path)
	}
}
