package services

const wordsPerMinute = 120.0

type socialCardTask struct {
	path, relPath, cardDestPath string
	metaData                    map[string]interface{}
	frontmatterHash             string
}

func (s *postServiceImpl) isOutdatedVersion(version string) bool {
	if version == "" {
		return false
	}
	for _, v := range s.cfg.Versions {
		if v.IsLatest {
			return version != v.Path && version != v.Name
		}
	}
	return true
}
