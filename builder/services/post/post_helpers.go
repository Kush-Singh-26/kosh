package services

const wordsPerMinute = 120.0

type socialCardTask struct {
	path, relPath, cardDestPath string
	metadata                    map[string]any
	frontmatterHash             string
}
