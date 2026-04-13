package models

import "encoding/xml"

// --- Sitemap Structures ---

// URLSet represents a sitemap urlset.
type URLSet struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []URL    `xml:"url"`
}

// URL represents a sitemap url entry.
type URL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// --- RSS Structures ---

// Rss represents the root RSS document.
type Rss struct {
	XMLName      xml.Name `xml:"rss"`
	Version      string   `xml:"version,attr"`
	XMLNSContent string   `xml:"xmlns:content,attr"`
	Channel      Channel  `xml:"channel"`
}

// Channel represents the RSS channel metadata and items.
type Channel struct {
	Title         string      `xml:"title"`
	Link          string      `xml:"link"`
	Description   string      `xml:"description"`
	LastBuildDate string      `xml:"lastBuildDate,omitempty"`
	Image         *RSSImage   `xml:"image,omitempty"`
	Items         []Item      `xml:"item"`
}

// RSSImage represents the channel logo.
type RSSImage struct {
	URL   string `xml:"url"`
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

// Item represents a single RSS item.
type Item struct {
	Title          string   `xml:"title"`
	Link           string   `xml:"link"`
	Description    string   `xml:"description"`
	PubDate        string   `xml:"pubDate"`
	Guid           string   `xml:"guid"`
	Author         string   `xml:"author,omitempty"`
	Categories     []string `xml:"category,omitempty"`
	ContentEncoded string   `xml:"content:encoded,omitempty"`
}
