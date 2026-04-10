package models

import "sort"

// SortPosts sorts posts by weight and date, descending.
func SortPosts(posts []PostMetadata) {
	sort.Slice(posts, func(i, j int) bool {
		weightI, weightJ := posts[i].Weight, posts[j].Weight

		// Sort by Weight Descending (Higher weight first)
		if weightI != weightJ {
			return weightI > weightJ
		}

		// Use Unix timestamps for faster integer comparison
		timeI, timeJ := posts[i].DateObj.Unix(), posts[j].DateObj.Unix()
		if timeI == timeJ {
			// Title Descending if dates match (arbitrary, stable)
			return posts[i].Title > posts[j].Title
		}
		// Date Descending (Newer first)
		return timeI > timeJ
	})
}
