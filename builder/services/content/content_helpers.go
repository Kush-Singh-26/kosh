package content

const wordsPerMinute = 120.0

type socialCardTask struct {
	path, relPath, cardDestPath string
	// metadata contains YAML frontmatter values for social card generation.
	// Expected types: string, bool, int/float64, time.Time, []any, map[string]any.
	metadata        map[string]any
	frontmatterHash string
}
