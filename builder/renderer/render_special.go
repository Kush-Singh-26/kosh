package renderer

import (
	"bufio"
	"bytes"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (r *Renderer) RenderIndex(path string, data models.PageData) {
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

	bw := bufio.NewWriterSize(f, utils.MaxBufferSize)
	defer func() { _ = bw.Flush() }()

	r.mu.RLock()
	index := r.Index
	layout := r.Layout
	r.mu.RUnlock()

	var buf bytes.Buffer
	var errExec error
	if index != nil {
		errExec = index.Execute(&buf, data)
	} else if layout != nil {
		errExec = layout.Execute(&buf, data)
	} else {
		r.logger.Error("No template available for index", "path", path)
		return
	}

	if errExec != nil {
		r.logger.Error("Failed to render index", "path", path, "error", errExec)
		return
	}

	// Process HTML
	processedHTML := utils.ProcessHTML(buf.String(), data.BaseURL, data.RelativePrefix, r.Compress)
	finalBytes := []byte(processedHTML)

	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	if _, err := bw.Write(finalBytes); err != nil {
		r.logger.Error("Failed to write processed index", "path", path, "error", err)
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) RenderGraph(path string, data models.PageData) {
	r.mu.RLock()
	graph := r.Graph
	r.mu.RUnlock()

	if graph == nil {
		return
	}

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

	bw := bufio.NewWriterSize(f, utils.MaxBufferSize)
	defer func() { _ = bw.Flush() }()

	var buf bytes.Buffer
	if err := graph.Execute(&buf, data); err != nil {
		r.logger.Error("Failed to render graph", "path", path, "error", err)
		return
	}

	// Process HTML
	processedHTML := utils.ProcessHTML(buf.String(), data.BaseURL, data.RelativePrefix, r.Compress)
	finalBytes := []byte(processedHTML)

	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	if _, err := bw.Write(finalBytes); err != nil {
		r.logger.Error("Failed to write processed graph", "path", path, "error", err)
	} else {
		r.RegisterFile(path)
	}
}

func (r *Renderer) Render404(path string, data models.PageData) {
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

	bw := bufio.NewWriterSize(f, utils.MaxBufferSize)
	defer func() { _ = bw.Flush() }()

	r.mu.RLock()
	notFound := r.NotFound
	layout := r.Layout
	r.mu.RUnlock()

	var buf bytes.Buffer
	var errExec error
	if notFound != nil {
		errExec = notFound.Execute(&buf, data)
	} else if layout != nil {
		errExec = layout.Execute(&buf, data)
	} else {
		r.logger.Error("No template available for 404", "path", path)
		return
	}

	if errExec != nil {
		r.logger.Error("Failed to render 404", "path", path, "error", errExec)
		return
	}

	// Process HTML
	processedHTML := utils.ProcessHTML(buf.String(), data.BaseURL, data.RelativePrefix, r.Compress)
	finalBytes := []byte(processedHTML)

	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	if _, err := bw.Write(finalBytes); err != nil {
		r.logger.Error("Failed to write processed 404", "path", path, "error", err)
	} else {
		r.RegisterFile(path)
	}
}
