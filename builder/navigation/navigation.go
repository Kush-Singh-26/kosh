package navigation

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

var (
	// ErrEmptyList indicates a missing post list.
	ErrEmptyList = errors.New("post list is empty")
	// ErrPostNotFound indicates the current post was not found.
	ErrPostNotFound = errors.New("post not found in list")
)

// FindPrevNext finds previous and next pages.
// currentPost: the current post metadata
// allPosts: all posts, must be pre-sorted via timeutil.SortItems.
func FindPrevNext(currentPost models.ContentMetadata, allPosts []models.ContentMetadata) (*models.NavPage, *models.NavPage, error) {
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
			Path:  p.Path,
			Title: p.Title,
			Link:  p.Link,
		}
	}

	// Next post (comes after in sorted list)
	if currentIdx < len(sortedPosts)-1 {
		n := sortedPosts[currentIdx+1]
		next = &models.NavPage{
			Path:  n.Path,
			Title: n.Title,
			Link:  n.Link,
		}
	}

	return prev, next, nil
}

type byWeightThenTitle []*models.NodeTree

func (a byWeightThenTitle) Len() int { return len(a) }
func (a byWeightThenTitle) Less(i, j int) bool {
	// 1. Pinned First
	if a[i].Resource.IsPinned != a[j].Resource.IsPinned {
		return a[i].Resource.IsPinned
	}

	// 2. Weight ASC
	if a[i].Resource.Weight != a[j].Resource.Weight {
		return a[i].Resource.Weight < a[j].Resource.Weight
	}

	// 3. Date DESC
	ti, tj := a[i].Resource.Date.Unix(), a[j].Resource.Date.Unix()
	if ti != tj {
		return ti > tj
	}

	// 4. Title ASC
	return a[i].Resource.Title < a[j].Resource.Title
}
func (a byWeightThenTitle) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		if runes[i-1] == '-' || runes[i-1] == '_' || runes[i-1] == ' ' {
			runes[i] = unicode.ToUpper(runes[i])
		} else {
			runes[i] = unicode.ToLower(runes[i])
		}
	}
	return string(runes)
}

// BuildNavigationTree builds a hierarchical NodeTree from a flat list of ContentMetadata items.
// It infers section nodes from directory paths and assigns page nodes as leaves.
// The tree is deterministic given the same input — safe to build once and share.
func BuildNavigationTree(items []models.ContentMetadata) *models.NodeTree {
	root := &models.NodeTree{
		Resource: models.Resource{
			Title: "Home",
			Link:  "/",
			Type:  models.NodeHome,
		},
	}

	dirNodes := map[string]*models.NodeTree{}

	for _, item := range items {
		if item.IsHidden || item.IsDraft {
			continue
		}

		path := filepath.ToSlash(item.Path)
		segments := strings.Split(path, "/")

		// Filter out empty segments (handles both absolute paths like
		// "/home/.../content/docs/page.md" and relative "docs/page.md")
		var cleanSegments []string
		for _, seg := range segments {
			if seg != "" {
				cleanSegments = append(cleanSegments, seg)
			}
		}
		segments = cleanSegments

		parent := root
		currentDir := ""

		for i, seg := range segments {
			isLast := i == len(segments)-1
			seg = strings.TrimSuffix(seg, ".md")
			seg = strings.TrimSuffix(seg, ".html")

			if isLast {
				if seg == "_index" || seg == "" {
					// Apply _index.md metadata to the section node if we are at the parent
					parent.Resource.Title = item.Title
					parent.Resource.Description = item.Description
					parent.Resource.Weight = item.Weight
					parent.Resource.IsPinned = item.IsPinned
					parent.Resource.Date = item.DateObj
					continue
				}
				child := &models.NodeTree{
					Resource: models.Resource{
						Title:       item.Title,
						Link:        item.Link,
						Description: item.Description,
						Weight:      item.Weight,
						ReadingTime: item.ReadingTime,
						IsPinned:    item.IsPinned,
						IsDraft:     item.IsDraft,
						Date:        item.DateObj,
						Type:        models.NodePage,
						RelPath:     item.Path,
					},
				}
				parent.Children = append(parent.Children, child)
			} else {
				if currentDir != "" {
					currentDir += "/"
				}
				currentDir += seg

				if existing, ok := dirNodes[currentDir]; ok {
					parent = existing
				} else {
					sectionLink := "/" + currentDir
					if !strings.HasSuffix(sectionLink, "/") {
						sectionLink += "/"
					}
					section := &models.NodeTree{
						Resource: models.Resource{
							Title: titleCase(strings.ReplaceAll(seg, "-", " ")),
							Link:  sectionLink,
							Type:  models.NodeSection,
						},
					}
					section.Parent = parent
					parent.Children = append(parent.Children, section)
					dirNodes[currentDir] = section
					parent = section
				}
			}
		}
	}

	sortNodeTreeChildren(root)
	return root
}

func sortNodeTreeChildren(node *models.NodeTree) {
	sort.Sort(byWeightThenTitle(node.Children))
	for _, child := range node.Children {
		sortNodeTreeChildren(child)
	}
}

// FlattenTree returns all navigable nodes in the tree in the order they appear in navigation.
func FlattenTree(node *models.NodeTree) []models.Resource {
	var nodes []models.Resource
	// Include the node itself if it's a page, section, or home
	if node.Resource.Link != "" {
		nodes = append(nodes, node.Resource)
	}
	for _, child := range node.Children {
		nodes = append(nodes, FlattenTree(child)...)
	}
	return nodes
}
