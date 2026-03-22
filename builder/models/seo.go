package models

import (
	"encoding/json"
	"html/template"
)

type JSONLDArticle struct {
	Context       string   `json:"@context"`
	Type          string   `json:"@type"`
	Headline      string   `json:"headline"`
	DatePublished string   `json:"datePublished"`
	DateModified  string   `json:"dateModified,omitempty"`
	Author        []Author `json:"author,omitempty"`
	Description   string   `json:"description,omitempty"`
	Image         string   `json:"image,omitempty"`
}

type Author struct {
	Type string `json:"@type"`
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

func GeneratePostJSONLD(post PostMetadata, author AuthorConfig, imageURL string) template.HTML {
	article := JSONLDArticle{
		Context:       "https://schema.org",
		Type:          "BlogPosting",
		Headline:      post.Title,
		DatePublished: post.DateObj.Format("2006-01-02"),
		Description:   post.Description,
		Image:         imageURL,
	}

	if author.Name != "" {
		article.Author = []Author{{
			Type: "Person",
			Name: author.Name,
			URL:  author.URL,
		}}
	}

	data, err := json.Marshal(article)
	if err != nil {
		return ""
	}

	return template.HTML(data)
}
