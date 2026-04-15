//go:build !wasm

package renderer

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	koshMinify "github.com/Kush-Singh-26/kosh/builder/minify"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
)

const (
	imgTagPrefixLen   = 4 // len("<img")
	tagStartLookahead = 2
)

// executeTemplateAndWrite executes a template, processes HTML, optionally minifies, and writes via sink.
// This is a unified helper to avoid duplication across RenderPage, RenderIndex, RenderGraph, and Render404.
func (r *Renderer) executeTemplateAndWrite(path string, tmpl Executor, data models.PageData, templateName string) error {
	r.PreparePageData(&data)

	buf := pools.SharedBufferPool.Get()
	defer pools.SharedBufferPool.Put(buf)

	if err := tmpl.Execute(buf, data); err != nil {
		return fmt.Errorf("failed to execute %s template for %s: %w", templateName, path, err)
	}

	finalBytes := buf.Bytes()

	// Rewrite PNG/JPG/JPEG image references to WebP if compression is enabled
	if r.Compress {
		finalBytes = rewriteImageRefs(finalBytes, path, r.devMode)
	}

	// Final Write with Streaming Minification
	if err := r.Sink.WriteStream(path, func(w io.Writer) error {
		if r.Minify {
			minifier := r.Minifier
			if minifier == nil {
				minifier = koshMinify.GetHTMLMinifier()
			}
			mw := minifier.Writer("text/html", w)
			_, err := mw.Write(finalBytes)
			if closeErr := mw.Close(); err == nil {
				err = closeErr
			}
			return err
		}
		_, err := w.Write(finalBytes)
		return err
	}); err != nil {
		r.recordError("Failed to write processed HTML", path, err)
		return fmt.Errorf("failed to write processed HTML for %s: %w", path, err)
	}

	r.RegisterFile(path)
	return nil
}

// RenderPage renders a standard content page using the layout template.
func (r *Renderer) RenderPage(path string, data models.PageData) error {
	r.mu.RLock()
	layout := r.Layout

	// Case-insensitive check for layout in metadata
	layoutReq := ""
	if l, ok := data.Meta["layout"].(string); ok {
		layoutReq = strings.ToLower(l)
	} else if l, ok := data.Meta["Layout"].(string); ok {
		layoutReq = strings.ToLower(l)
	}

	// Determine if this is the root index file
	isRoot := false
	if (data.RelativePrefix == "" || data.RelativePrefix == "./") && strings.HasSuffix(filepath.Base(path), "index.html") {
		// Only consider it root if it's NOT inside a content section or taxonomy directory
		contentPrefix := strings.Trim(data.ContentPrefix, "/")
		isContentSubpath := contentPrefix != "" && strings.Contains(path, "/"+contentPrefix+"/")

		// For taxonomies, we check if it's in a plural folder.
		// This is a bit tricky without full tree awareness here,
		// but checking for relative prefix emptiness is a strong signal for root.
		if !isContentSubpath && !data.IsTaxonomyIndex {
			isRoot = true
		}
	}

	// Choose layout
	switch {
	case layoutReq == "home" && r.Home != nil:
		layout = r.Home
	case isRoot && r.Home != nil:
		layout = r.Home
	case layoutReq == "index" && r.Index != nil:
		layout = r.Index
	}

	r.mu.RUnlock()

	if layout == nil {
		return fmt.Errorf("layout template not loaded for page %s", path)
	}

	return r.executeTemplateAndWrite(path, layout, data, "layout")
}

func rewriteImageRefs(html []byte, path string, isDev bool) []byte {
	converted := assets.GetConvertedImages()
	// Short-circuit if no images to rewrite AND no img tags to check for A11y
	if len(converted) == 0 && !strings.Contains(strings.ToLower(string(html)), "<img") {
		return html
	}

	result := make([]byte, 0, len(html))
	i := 0
	for i < len(html) {
		tagStart := findTagStart(html, i)
		if tagStart < 0 {
			result = append(result, html[i:]...)
			break
		}

		result = append(result, html[i:tagStart]...)
		i = tagStart

		end := findTagEnd(html, i)
		if end < 0 {
			result = append(result, html[i:]...)
			break
		}

		if isImgTag(html, i) {
			tagContent := html[tagStart : end+1]
			// A11y Check: Look for alt attribute
			if !isDev && !hasAltAttribute(tagContent) {
				slog.Warn("A11y Lint: Image missing alt text",
					"file", path,
					"tag", string(tagContent))
			}
			rewritten := rewriteImgTag(tagContent, converted)
			result = append(result, rewritten...)
		} else {
			result = append(result, html[tagStart:end+1]...)
		}
		i = end + 1
	}

	return result
}

func hasAltAttribute(tag []byte) bool {
	// Simple check for 'alt=' in the tag
	lowerTag := strings.ToLower(string(tag))
	return strings.Contains(lowerTag, " alt=") || strings.Contains(lowerTag, " alt\t") ||
		strings.Contains(lowerTag, " alt\n") || strings.Contains(lowerTag, " alt\r")
}

func findTagStart(html []byte, i int) int {
	for ; i+tagStartLookahead < len(html); i++ {
		if html[i] == '<' {
			c := html[i+1]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				return i
			}
		}
	}
	return -1
}

func isImgTag(html []byte, i int) bool {
	if i+imgTagPrefixLen > len(html) {
		return false
	}
	// Case insensitive "img" check
	if (html[i+1] == 'i' || html[i+1] == 'I') &&
		(html[i+2] == 'm' || html[i+2] == 'M') &&
		(html[i+imgTagPrefixLen-1] == 'g' || html[i+imgTagPrefixLen-1] == 'G') {
		// Ensure it's not just the prefix of another tag like <image>
		if i+imgTagPrefixLen < len(html) {
			c := html[i+imgTagPrefixLen]
			return c == ' ' || c == '>' || c == '/' || c == '\t' || c == '\n' || c == '\r'
		}
		return true
	}
	return false
}

func findTagEnd(html []byte, i int) int {
	inQuote := false
	var quoteChar byte
	for ; i < len(html); i++ {
		if inQuote {
			if html[i] == quoteChar {
				inQuote = false
			}
			continue
		}
		if html[i] == '"' || html[i] == '\'' {
			inQuote = true
			quoteChar = html[i]
			continue
		}
		if html[i] == '>' {
			return i
		}
	}
	return -1
}

func appendLazyAttributes(result []byte) []byte {
	lowerResult := strings.ToLower(string(result))
	isSiteLogo := strings.Contains(lowerResult, `class="site-logo`) || strings.Contains(lowerResult, `class='site-logo`)
	if isSiteLogo {
		return result
	}
	if !strings.Contains(lowerResult, " loading=") && !strings.Contains(lowerResult, " loading\t") && !strings.Contains(lowerResult, " loading\n") {
		result = append(result, []byte(" loading=\"lazy\"")...)
	}
	if !strings.Contains(lowerResult, " decoding=") && !strings.Contains(lowerResult, " decoding\t") && !strings.Contains(lowerResult, " decoding\n") {
		result = append(result, []byte(" decoding=\"async\"")...)
	}
	return result
}

func rewriteAttrValue(attrName, value string, converted map[string]string) string {
	if strings.EqualFold(attrName, "src") {
		return rewriteImgSrc(value, converted)
	}
	return value
}

func rewriteImgTag(tag []byte, converted map[string]string) []byte {
	result := make([]byte, 0, len(tag))
	i := imgTagPrefixLen
	result = append(result, tag[:i]...)

	for i < len(tag) {
		i = skipWhitespaceBytes(tag, i, &result)

		if i >= len(tag) || isTagClosingChar(tag[i]) {
			closingPart := tag[i:]
			result = appendLazyAttributes(result)
			result = append(result, closingPart...)
			break
		}

		attrName, nameEnd := extractAttributeName(tag, i)
		result = append(result, tag[i:nameEnd]...)
		i = nameEnd

		i = skipWhitespaceBytes(tag, i, &result)

		if i >= len(tag) || tag[i] != '=' {
			continue
		}

		result = append(result, '=')
		i++

		i = skipWhitespaceBytes(tag, i, &result)

		if i >= len(tag) {
			break
		}

		if tag[i] == '"' || tag[i] == '\'' {
			quote := tag[i]
			result = append(result, quote)
			i++
			valStart := i
			for i < len(tag) && tag[i] != quote {
				i++
			}
			val := string(tag[valStart:i])
			rewritten := rewriteAttrValue(attrName, val, converted)
			result = append(result, []byte(rewritten)...)

			if i < len(tag) {
				result = append(result, quote)
				i++
			}
		} else {
			valStart := i
			for i < len(tag) && !isTagClosingChar(tag[i]) && tag[i] != ' ' && tag[i] != '\t' && tag[i] != '\n' && tag[i] != '\r' {
				i++
			}
			val := string(tag[valStart:i])
			rewritten := rewriteAttrValue(attrName, val, converted)
			result = append(result, []byte(rewritten)...)
		}
	}

	return result
}

func skipWhitespaceBytes(tag []byte, i int, result *[]byte) int {
	for i < len(tag) && (tag[i] == ' ' || tag[i] == '\t' || tag[i] == '\n' || tag[i] == '\r') {
		*result = append(*result, tag[i])
		i++
	}
	return i
}

func extractAttributeName(tag []byte, i int) (string, int) {
	nameStart := i
	for i < len(tag) && tag[i] != '=' && !isTagClosingChar(tag[i]) && tag[i] != ' ' && tag[i] != '\t' && tag[i] != '\n' && tag[i] != '\r' {
		i++
	}
	return string(tag[nameStart:i]), i
}

func isTagClosingChar(c byte) bool {
	return c == '>' || c == '/'
}

func rewriteImgSrc(src string, converted map[string]string) string {
	lower := strings.ToLower(src)
	if !strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".jpeg") {
		return src
	}

	// Try to find exact match in converted map
	if webpDst, ok := converted[src]; ok {
		return webpDst
	}

	return src
}
