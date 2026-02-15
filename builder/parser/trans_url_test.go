package parser

import (
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func TestIsCrossVersionLink(t *testing.T) {
	tests := []struct {
		name     string
		href     string
		expected bool
	}{
		{
			name:     "cross version link with v prefix",
			href:     "../v1.0/getting-started.md",
			expected: true,
		},
		{
			name:     "cross version link nested",
			href:     "../v3.0/api/reference.md",
			expected: true,
		},
		{
			name:     "same version cross directory",
			href:     "../advanced/configuration.md",
			expected: false,
		},
		{
			name:     "same version nested directory",
			href:     "../api/reference.md",
			expected: false,
		},
		{
			name:     "simple cross directory",
			href:     "../features.md",
			expected: false,
		},
		{
			name:     "absolute link",
			href:     "/docs/v2.0/intro",
			expected: false,
		},
		{
			name:     "external link",
			href:     "https://example.com",
			expected: false,
		},
		{
			name:     "relative link without parent",
			href:     "features.md",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCrossVersionLink(tt.href)
			if result != tt.expected {
				t.Errorf("isCrossVersionLink(%q) = %v, want %v", tt.href, result, tt.expected)
			}
		})
	}
}

func TestIsRootLevelLink(t *testing.T) {
	tests := []struct {
		name     string
		href     string
		expected bool
	}{
		{
			name:     "index.md at root",
			href:     "../index.md",
			expected: true,
		},
		{
			name:     "features.md at root",
			href:     "../features.md",
			expected: true,
		},
		{
			name:     "getting-started.md at root",
			href:     "../getting-started.md",
			expected: true,
		},
		{
			name:     "index.md with leading dot",
			href:     "./index.md",
			expected: true,
		},
		{
			name:     "subdirectory file should NOT be root level",
			href:     "../advanced/configuration.md",
			expected: false,
		},
		{
			name:     "api reference should NOT be root level",
			href:     "../api/reference.md",
			expected: false,
		},
		{
			name:     "simple file in same directory",
			href:     "setup.md",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRootLevelLink(tt.href)
			if result != tt.expected {
				t.Errorf("isRootLevelLink(%q) = %v, want %v", tt.href, result, tt.expected)
			}
		})
	}
}

func TestExtractVersionFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "v2.0 versioned file",
			path:     "content/v2.0/getting-started.md",
			expected: "v2.0",
		},
		{
			name:     "v1.0 versioned file",
			path:     "content/v1.0/docs/intro.md",
			expected: "v1.0",
		},
		{
			name:     "non-versioned file",
			path:     "content/blog/post.md",
			expected: "",
		},
		{
			name:     "root file",
			path:     "content/index.md",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractVersionFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetFileDepthInVersion(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{
			name:     "file at version root",
			path:     "content/v2.0/intro.md",
			expected: 0,
		},
		{
			name:     "file one level deep",
			path:     "content/v2.0/docs/intro.md",
			expected: 1,
		},
		{
			name:     "file two levels deep",
			path:     "content/v2.0/docs/api/reference.md",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFileDepthInVersion(tt.path)
			if result != tt.expected {
				t.Errorf("getFileDepthInVersion(%q) = %d, want %d", tt.path, result, tt.expected)
			}
		})
	}
}

func TestURLTransformer_VersionAwareLinking(t *testing.T) {
	tests := []struct {
		name         string
		filePath     string
		input        string
		expectedLink string
	}{
		{
			name:         "cross directory link within version - use relative path",
			filePath:     "content/v2.0/getting-started.md",
			input:        "[Advanced Config](../advanced/configuration.md)",
			expectedLink: "advanced/configuration.html",
		},
		{
			name:         "cross version link - preserve as-is",
			filePath:     "content/v2.0/intro.md",
			input:        "[v1.0 Docs](../v1.0/intro.md)",
			expectedLink: "../v1.0/intro.html",
		},
		{
			name:         "root level index link should NOT get version",
			filePath:     "content/v2.0/docs/intro.md",
			input:        "[Home](../index.md)",
			expectedLink: "../index.html",
		},
		{
			name:         "root level features link should NOT get version",
			filePath:     "content/v2.0/getting-started.md",
			input:        "[Features](../features.md)",
			expectedLink: "../features.html",
		},
		{
			name:         "same directory link - use relative path",
			filePath:     "content/v2.0/intro.md",
			input:        "[Setup](./setup.md)",
			expectedLink: "setup.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := goldmark.New(
				goldmark.WithParserOptions(
					parser.WithASTTransformers(
						util.Prioritized(&URLTransformer{BaseURL: "https://example.com"}, 100),
					),
				),
			)

			context := parser.NewContext()
			context.Set(ContextKeyFilePath, tt.filePath)

			reader := text.NewReader([]byte(tt.input))
			doc := md.Parser().Parse(reader, parser.WithContext(context))

			var foundLink string
			if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
				if !entering {
					return ast.WalkContinue, nil
				}
				if link, ok := n.(*ast.Link); ok {
					foundLink = string(link.Destination)
				}
				return ast.WalkContinue, nil
			}); err != nil {
				t.Fatalf("ast.Walk failed: %v", err)
			}

			if foundLink != tt.expectedLink {
				t.Errorf("link destination = %q, want %q", foundLink, tt.expectedLink)
			}
		})
	}
}
