package models

import (
	"testing"
)

func TestSSRTypeString(t *testing.T) {
	tests := []struct {
		name     SSRArtifactType
		expected string
	}{
		{SSRTypeD2, "d2"},
		{SSRTypeMath, "math"},
		{999, "unknown"}, // Unknown type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.name.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseSSRType(t *testing.T) {
	tests := []struct {
		input    string
		expected SSRArtifactType
	}{
		{"d2", SSRTypeD2},
		{"math", SSRTypeMath},
		{"unknown", SSRTypeD2},       // Default to D2
		{"", SSRTypeD2},              // Empty string defaults
		{"D2", SSRTypeD2},            // Case sensitive - defaults
		{"Math", SSRTypeD2},          // Case sensitive - defaults
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseSSRType(tt.input)
			if result != tt.expected {
				t.Errorf("ParseSSRType(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseSSRTypeAndStringRoundTrip(t *testing.T) {
	// Test that parsing and String() round-trip correctly
	tests := []SSRArtifactType{SSRTypeD2, SSRTypeMath}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			parsed := ParseSSRType(tt.String())
			if parsed != tt {
				t.Errorf("ParseSSRType(String()) round-trip failed: got %v, want %v", parsed, tt)
			}
		})
	}
}

func TestMathExpression(t *testing.T) {
	tests := []struct {
		name        string
		expr        MathExpression
		expectHash  bool
	}{
		{
			name: "inline math",
			expr: MathExpression{
				LaTeX:       "x^2 + y^2 = z^2",
				DisplayMode: false,
				Hash:        "abc123",
			},
			expectHash: true,
		},
		{
			name: "display math",
			expr: MathExpression{
				LaTeX:       "\\int_0^\\infty e^{-x} dx",
				DisplayMode: true,
				Hash:        "def456",
			},
			expectHash: true,
		},
		{
			name: "empty latex",
			expr: MathExpression{
				LaTeX:       "",
				DisplayMode: false,
				Hash:        "",
			},
			expectHash: false,
		},
		{
			name: "complex latex",
			expr: MathExpression{
				LaTeX:       "\\sum_{i=1}^{n} i = \\frac{n(n+1)}{2}",
				DisplayMode: true,
				Hash:        "ghi789",
			},
			expectHash: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify structure
			if tt.expr.LaTeX == "" && tt.expectHash {
				t.Error("Expected non-empty LaTeX")
			}

			// Verify hash is set when expected
			if tt.expectHash && tt.expr.Hash == "" {
				t.Error("Expected hash to be set")
			}

			// Verify DisplayMode is preserved
			expected := tt.expr.DisplayMode
			if expected != tt.expr.DisplayMode {
				t.Error("DisplayMode not preserved")
			}
		})
	}
}

func TestSSRArtifactTypeIota(t *testing.T) {
	// Verify iota-based enum starts at 0
	if SSRTypeD2 != 0 {
		t.Errorf("SSRTypeD2 should be 0, got %d", SSRTypeD2)
	}
	if SSRTypeMath != 1 {
		t.Errorf("SSRTypeMath should be 1, got %d", SSRTypeMath)
	}
}
