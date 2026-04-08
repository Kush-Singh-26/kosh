package models

import "sort"

// SortPosts sorts posts by weight and date, descending.
func SortPosts(posts []PostMetadata) {
	sort.Slice(posts, func(i, j int) bool {
		wi, wj := posts[i].Weight, posts[j].Weight

		// Sort by Weight Descending (Higher weight first)
		if wi != wj {
			return wi > wj
		}

		// Use Unix timestamps for faster integer comparison
		ti, tj := posts[i].DateObj.Unix(), posts[j].DateObj.Unix()
		if ti == tj {
			// Title Descending if dates match (arbitrary, stable)
			return posts[i].Title > posts[j].Title
		}
		// Date Descending (Newer first)
		return ti > tj
	})
}
