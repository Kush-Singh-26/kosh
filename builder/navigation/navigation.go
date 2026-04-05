package navigation

import (
	"errors"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

var (
	ErrEmptyList    = errors.New("post list is empty")
	ErrPostNotFound = errors.New("post not found in list")
)

// FindPrevNext finds previous and next pages.
// currentPost: the current post metadata
// allPosts: all posts, must be pre-sorted via timeutil.SortPosts.
func FindPrevNext(currentPost models.PostMetadata, allPosts []models.PostMetadata) (*models.NavPage, *models.NavPage, error) {
	if len(allPosts) == 0 {
		return nil, nil, ErrEmptyList
	}

	if len(allPosts) == 1 {
		if allPosts[0].Link == currentPost.Link || allPosts[0].Title == currentPost.Title {
			return nil, nil, nil
		}
		return nil, nil, ErrPostNotFound
	}

	sortedPosts := allPosts

	// Find current post index
	currentIdx := -1
	for i, post := range sortedPosts {
		// Match by Link since it's unique
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
		return nil, nil, ErrPostNotFound
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

	return prev, next, nil
}
