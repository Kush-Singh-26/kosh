package models

import "sort"

// SortItems sorts content items by weight and date, descending.
func SortItems(items []ContentMetadata) {
	sort.Slice(items, func(i, j int) bool {
		weightI, weightJ := items[i].Weight, items[j].Weight

		// Sort by Weight Descending (Higher weight first)
		if weightI != weightJ {
			return weightI > weightJ
		}

		// Use Unix timestamps for faster integer comparison
		timeI, timeJ := items[i].DateObj.Unix(), items[j].DateObj.Unix()
		if timeI == timeJ {
			// Title Descending if dates match (arbitrary, stable)
			return items[i].Title > items[j].Title
		}
		// Date Descending (Newer first)
		return timeI > timeJ
	})
}
