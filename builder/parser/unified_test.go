package parser

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestTransformContext_HeadingTracking(t *testing.T) {
	ctx := &transformContext{}

	// Simulate heading entry
	ctx.inHeading = true
	ctx.headingLevel = 2
	ctx.headingID = "test-heading"
	ctx.headingText.Reset()
	ctx.headingText.WriteString("Test Heading")

	// Verify state
	if !ctx.inHeading {
		t.Error("Expected inHeading to be true")
	}
	if ctx.headingLevel != 2 {
		t.Errorf("Expected headingLevel 2, got %d", ctx.headingLevel)
	}
	if ctx.headingID != "test-heading" {
		t.Errorf("Expected headingID 'test-heading', got %q", ctx.headingID)
	}
	if ctx.headingText.String() != "Test Heading" {
		t.Errorf("Expected headingText 'Test Heading', got %q", ctx.headingText.String())
	}

	// Simulate heading exit - create TOC entry
	if ctx.headingID != "" {
		_ = models.TOCEntry{
			ID:    ctx.headingID,
			Text:  ctx.headingText.String(),
			Level: ctx.headingLevel,
		}
	}
	ctx.inHeading = false
	ctx.headingLevel = 0
	ctx.headingID = ""

	// Verify reset
	if ctx.inHeading {
		t.Error("Expected inHeading to be false after exit")
	}
}

func TestReplacementStruct(t *testing.T) {
	// Test that replacement struct works correctly
	oldNode := ast.NewText()
	newNode := ast.NewText()

	replacement := &replacement{
		old: oldNode,
		new: newNode,
	}

	if replacement.old != oldNode {
		t.Error("Expected old to match oldNode")
	}
	if replacement.new != newNode {
		t.Error("Expected new to match newNode")
	}
}

func TestD2BlockInfoStruct(t *testing.T) {
	// Test that d2BlockInfo struct fields work correctly
	info := &d2BlockInfo{
		code: "test d2 code",
		hash: "abc123",
	}

	if info.code != "test d2 code" {
		t.Errorf("Expected code 'test d2 code', got %q", info.code)
	}
	if info.hash != "abc123" {
		t.Errorf("Expected hash 'abc123', got %q", info.hash)
	}
	// node can be nil for this test
	if info.node != nil {
		t.Error("Expected node to be nil")
	}
}

func TestASTWalk_HeadingExtraction(t *testing.T) {
	// Create a simple markdown document with headings
	markdown := `# Title

## Section 1

Some content here.

### Subsection 1.1

More content.

## Section 2

Final content.
`

	source := []byte(markdown)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(source)).(*ast.Document)

	headingCount := 0

	// Walk the AST to count headings
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if n.Kind() == ast.KindHeading && entering {
			heading := n.(*ast.Heading)
			headingCount++
			t.Logf("Found heading level %d", heading.Level)
		}
		return ast.WalkContinue, nil
	})

	// Verify headings were found (including H1)
	if headingCount < 3 {
		t.Errorf("Expected at least 3 headings, got %d", headingCount)
	}
}

func TestASTWalk_TextCollection(t *testing.T) {
	markdown := `Just plain text here with some content.`

	source := []byte(markdown)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(source)).(*ast.Document)

	var plainText strings.Builder

	// Walk and collect text
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n.Kind() {
			case ast.KindText:
				textNode := n.(*ast.Text)
				segment := textNode.Segment.Value(source)
				plainText.Write(segment)
			case ast.KindString:
				strNode := n.(*ast.String)
				plainText.Write(strNode.Value)
			}
		}
		return ast.WalkContinue, nil
	})

	// Verify text was collected
	if plainText.Len() == 0 {
		t.Error("Expected plain text to be collected")
	}
}

func TestUnifiedTransformerStruct(t *testing.T) {
	// Test that unifiedTransformer can be created
	transformer := &unifiedTransformer{
		BaseURL:  "https://example.com",
		Compress: false,
	}

	if transformer.BaseURL != "https://example.com" {
		t.Errorf("Expected BaseURL 'https://example.com', got %q", transformer.BaseURL)
	}
	if transformer.Compress {
		t.Error("Expected Compress to be false")
	}
}

func TestTransformState_HeadingHandling(t *testing.T) {
	state := &transformState{
		ctx: &transformContext{},
	}

	heading := ast.NewHeading(2)
	heading.SetAttributeString("id", []byte("existing-id"))

	// Enter heading
	state.handleHeading(heading, true)
	if !state.ctx.inHeading {
		t.Error("Expected inHeading to be true")
	}
	if state.ctx.headingID != "existing-id" {
		t.Errorf("Expected headingID 'existing-id', got %q", state.ctx.headingID)
	}

	// Exit heading
	state.handleHeading(heading, false)
	if state.ctx.inHeading {
		t.Error("Expected inHeading to be false")
	}
	if len(state.toc) != 1 {
		t.Errorf("Expected 1 TOC entry, got %d", len(state.toc))
	}
	if state.toc[0].ID != "existing-id" {
		t.Errorf("Expected TOC ID 'existing-id', got %q", state.toc[0].ID)
	}
}

func TestTransformState_TextCollection(t *testing.T) {
	source := []byte("hello world")
	state := &transformState{
		source: source,
		ctx:    &transformContext{},
	}

	tNode := ast.NewText()
	tNode.Segment = text.Segment{Start: 0, Stop: 5} // "hello"

	state.collectText(tNode)
	if strings.TrimSpace(state.plainText.String()) != "hello" {
		t.Errorf("Expected plainText 'hello', got %q", state.plainText.String())
	}

	// Test heading text collection
	state.ctx.inHeading = true
	state.ctx.headingText.Reset()
	state.collectText(tNode)
	if state.ctx.headingText.String() != "hello" {
		t.Errorf("Expected headingText 'hello', got %q", state.ctx.headingText.String())
	}
}
