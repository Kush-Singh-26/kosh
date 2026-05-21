package models

import "sort"

// SortItems sorts content items using the Kosh unified sorting strategy:
// 1. Pinned items first (IsPinned DESC)
// 2. Lower weights first (Weight ASC)
// 3. Newer dates first (Date DESC)
// 4. Alphabetical title (Title ASC)
func SortItems(items []ContentMetadata) {
	sort.Slice(items, func(i, j int) bool {
		// 1. Pinned First
		if items[i].IsPinned != items[j].IsPinned {
			return items[i].IsPinned // true comes before false
		}

		// 2. Weight ASC
		if items[i].Weight != items[j].Weight {
			return items[i].Weight < items[j].Weight
		}

		// 3. Date DESC
		ti, tj := items[i].DateObj.Unix(), items[j].DateObj.Unix()
		if ti != tj {
			return ti > tj
		}

		// 4. Title ASC
		return items[i].Title < items[j].Title
	})
}
