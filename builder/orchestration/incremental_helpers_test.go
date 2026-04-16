package orchestration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/incremental"
)

// TestResolveContentPaths_VariousPaths tests path resolution for incremental builds
func TestResolveContentPaths_VariousPaths(t *testing.T) {
	// Create a minimal engine for testing
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			ContentDir: "content",
		},
	}
	engine := NewEngine(WithDeps(EngineDependencies{Config: cfg, Logger: InitLogger()}))

	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name:        "simple path",
			path:        "content/posts/test.md",
			expectError: false,
		},
		{
			name:        "nested path",
			path:        "content/posts/2024/test.md",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relPath, htmlRelPath, cleanHTMLRelPath, err := engine.Incremental.ResolveContentPaths(tt.path)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, relPath)
			assert.NotEmpty(t, htmlRelPath)
			assert.NotEmpty(t, cleanHTMLRelPath)
		})
	}
}

// TestComputePostHashes_Consistency tests that hash computation is consistent
func TestComputePostHashes_Consistency(t *testing.T) {
	engine := NewEngine(WithDeps(EngineDependencies{Logger: InitLogger()}))

	// Test with frontmatter
	sourceWithFrontmatter := []byte("---\ntitle: Test\n---\n\n# Test Content\n\nThis is a test.")
	frontmatterHash1, bodyHash1 := engine.Incremental.ComputePostHashes(sourceWithFrontmatter, "Content")
	frontmatterHash2, bodyHash2 := engine.Incremental.ComputePostHashes(sourceWithFrontmatter, "Content")

	// Hashes should be deterministic
	assert.Equal(t, frontmatterHash1, frontmatterHash2)
	assert.Equal(t, bodyHash1, bodyHash2)
	assert.NotEmpty(t, frontmatterHash1, "Frontmatter hash should not be empty for content with frontmatter")
	assert.NotEmpty(t, bodyHash1, "Body hash should not be empty")

	// Different content should produce different hashes
	source2 := []byte("---\ntitle: Different\n---\n\n# Different Content\n\nThis is different.")
	frontmatterHash3, bodyHash3 := engine.Incremental.ComputePostHashes(source2, "Content")

	assert.NotEqual(t, frontmatterHash1, frontmatterHash3)
	assert.NotEqual(t, bodyHash1, bodyHash3)
}

// TestDeterminePostChange_AllCases tests all change detection scenarios
func TestDeterminePostChange_AllCases(t *testing.T) {
	engine := NewEngine(WithDeps(EngineDependencies{Logger: InitLogger()}))

	// Test with no cache (should return PostChangeNew)
	changeType := engine.Incremental.DeterminePostChange("test.md", "hash1", "hash2")
	assert.Equal(t, incremental.PostChangeNew, changeType)

	// Note: Testing with actual cache would require more setup
	// This test documents the basic behavior
}
