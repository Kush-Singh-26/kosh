package utils

import (
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// FindPrevNext finds previous and next pages in version context
// currentPost: the current post metadata
// allPosts: all posts in the current version (including fallback posts)
// Returns: previous page, next page (nil if not found)
func FindPrevNext(currentPost models.PostMetadata, allPosts []models.PostMetadata) (*models.NavPage, *models.NavPage) {
	if len(allPosts) <= 1 {
		return nil, nil
	}

	// Ensure posts are sorted using our robust logic
	sortedPosts := make([]models.PostMetadata, len(allPosts))
	copy(sortedPosts, allPosts)
	SortPosts(sortedPosts)

	// Find current post index
	currentIdx := -1
	for i, post := range sortedPosts {
		// Match by Link since it's unique and version-prefixed
		if post.Link == currentPost.Link {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		// Fallback to Title if Link doesn't match
		for i, post := range sortedPosts {
			if post.Title == currentPost.Title {
				currentIdx = i
				break
			}
		}
	}

	if currentIdx == -1 {
		return nil, nil
	}

	var prev, next *models.NavPage

	// Previous post (comes before in sorted list)
	if currentIdx > 0 {
		p := sortedPosts[currentIdx-1]
		prev = &models.NavPage{
			Title: p.Title,
			Link:  p.Link,
		}
	}

	// Next post (comes after in sorted list)
	if currentIdx < len(sortedPosts)-1 {
		n := sortedPosts[currentIdx+1]
		next = &models.NavPage{
			Title: n.Title,
			Link:  n.Link,
		}
	}

	return prev, next
}
