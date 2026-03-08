package generators

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/spf13/afero"
)

func TestGenerateSocialCard(t *testing.T) {
	sink := testutil.NewMemSink()
	srcFs := afero.NewMemMapFs()
	
	cfg := &config.SocialCardsConfig{
		Background: "#ffffff",
		TextColor:  "#000000",
		Gradient:   []string{"#f0f0f0", "#e0e0e0"},
		Angle:      45,
	}

	siteTitle := "Test Site"
	title := "Test Post Title"
	description := "This is a test description for the social card."
	dateStr := "March 6, 2026"
	destPath := "static/images/cards/test.webp"
	faviconPath := ""

	err := GenerateSocialCard(sink, srcFs, cfg, siteTitle, title, description, dateStr, destPath, faviconPath)
	if err != nil {
		t.Fatalf("GenerateSocialCard failed: %v", err)
	}

	content, ok := sink.Files[destPath]
	if !ok {
		t.Fatalf("Social card not written to sink at %s", destPath)
	}

	if len(content) == 0 {
		t.Error("Social card is empty")
	}
}

func TestGenerateSocialCard_LongTitle(t *testing.T) {
	sink := testutil.NewMemSink()
	srcFs := afero.NewMemMapFs()
	
	cfg := &config.SocialCardsConfig{
		Background: "#ffffff",
		TextColor:  "#000000",
	}

	siteTitle := "Test Site"
	title := "This is an extremely long title that should definitely wrap across multiple lines in the generated social card image to ensure the wrapping logic works correctly"
	description := "Short desc"
	dateStr := "March 6, 2026"
	destPath := "static/images/cards/long.webp"

	err := GenerateSocialCard(sink, srcFs, cfg, siteTitle, title, description, dateStr, destPath, "")
	if err != nil {
		t.Fatalf("GenerateSocialCard with long title failed: %v", err)
	}

	if len(sink.Files[destPath]) == 0 {
		t.Error("Long title social card is empty")
	}
}

func TestHexToRGBA(t *testing.T) {
	tests := []struct {
		hex  string
		want [4]uint8
	}{
		{"#ffffff", [4]uint8{255, 255, 255, 255}},
		{"#000000", [4]uint8{0, 0, 0, 255}},
		{"#ff0000", [4]uint8{255, 0, 0, 255}},
		{"invalid", [4]uint8{0, 0, 0, 255}},
	}

	for _, tt := range tests {
		got := hexToRGBA(tt.hex)
		if got.R != tt.want[0] || got.G != tt.want[1] || got.B != tt.want[2] || got.A != tt.want[3] {
			t.Errorf("hexToRGBA(%q) = %v, want %v", tt.hex, got, tt.want)
		}
	}
}
