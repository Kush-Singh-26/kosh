package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"my-ssg/builder/models"
)

func GetFrontmatterHash(metaData map[string]interface{}) (string, error) {
	isPinned, _ := metaData["pinned"].(bool)
	socialMeta := map[string]interface{}{
		"title":       GetString(metaData, "title"),
		"description": GetString(metaData, "description"),
		"date":        GetString(metaData, "date"),
		"tags":        GetSlice(metaData, "tags"),
		"pinned":      isPinned,
	}

	data, err := json.Marshal(socialMeta)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

type GraphHashData struct {
	Posts []PostGraphInfo `json:"posts"`
}

type PostGraphInfo struct {
	Title string   `json:"title"`
	Link  string   `json:"link"`
	Tags  []string `json:"tags"`
}

func GetGraphHash(posts []models.PostMetadata) (string, error) {
	graphInfo := make([]PostGraphInfo, 0, len(posts))
	for _, p := range posts {
		graphInfo = append(graphInfo, PostGraphInfo{
			Title: p.Title,
			Link:  p.Link,
			Tags:  p.Tags,
		})
	}

	data, err := json.Marshal(graphInfo)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
