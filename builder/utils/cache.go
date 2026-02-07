package utils

import (
	"encoding/json"
	"os"
	"path/filepath"

	"my-ssg/builder/models"
)

type SocialCardCache struct {
	Hashes    map[string]string `json:"hashes"`
	GraphHash string            `json:"graph_hash"`
}

func NewSocialCardCache() *SocialCardCache {
	return &SocialCardCache{
		Hashes: make(map[string]string),
	}
}

func LoadSocialCardCache(path string) (*SocialCardCache, error) {
	cache := NewSocialCardCache()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cache, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cache); err != nil {
		return nil, err
	}

	return cache, nil
}

func SaveSocialCardCache(path string, cache *SocialCardCache) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func LoadBuildCache(path string) (*models.MetadataCache, error) {
	cache := &models.MetadataCache{
		Posts: make(map[string]models.CachedPost),
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cache, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cache); err != nil {
		return nil, err
	}

	return cache, nil
}

func SaveBuildCache(path string, cache *models.MetadataCache) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
