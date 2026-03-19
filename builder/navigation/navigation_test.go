package navigation

import (
	"errors"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestFindPrevNext(t *testing.T) {
	posts := []models.PostMetadata{
		{Title: "Post 1", Link: "/posts/1.html"},
		{Title: "Post 2", Link: "/posts/2.html"},
		{Title: "Post 3", Link: "/posts/3.html"},
	}

	tests := []struct {
		name     string
		current  models.PostMetadata
		all      []models.PostMetadata
		wantPrev string
		wantNext string
		wantErr  error
	}{
		{
			name:     "middle post",
			current:  posts[1],
			all:      posts,
			wantPrev: "Post 1",
			wantNext: "Post 3",
			wantErr:  nil,
		},
		{
			name:     "first post",
			current:  posts[0],
			all:      posts,
			wantPrev: "",
			wantNext: "Post 2",
			wantErr:  nil,
		},
		{
			name:     "last post",
			current:  posts[2],
			all:      posts,
			wantPrev: "Post 2",
			wantNext: "",
			wantErr:  nil,
		},
		{
			name:     "only post",
			current:  posts[0],
			all:      []models.PostMetadata{posts[0]},
			wantPrev: "",
			wantNext: "",
			wantErr:  nil,
		},
		{
			name:    "empty list",
			current: posts[0],
			all:     []models.PostMetadata{},
			wantErr: ErrEmptyList,
		},
		{
			name:    "post not found",
			current: models.PostMetadata{Title: "Other", Link: "/other.html"},
			all:     posts,
			wantErr: ErrPostNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev, next, err := FindPrevNext(tt.current, tt.all)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("FindPrevNext() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("FindPrevNext() unexpected error = %v", err)
				return
			}

			if tt.wantPrev == "" {
				if prev != nil {
					t.Errorf("FindPrevNext() prev = %v, want nil", prev.Title)
				}
			} else {
				if prev == nil || prev.Title != tt.wantPrev {
					t.Errorf("FindPrevNext() prev = %v, want %v", prev, tt.wantPrev)
				}
			}

			if tt.wantNext == "" {
				if next != nil {
					t.Errorf("FindPrevNext() next = %v, want nil", next.Title)
				}
			} else {
				if next == nil || next.Title != tt.wantNext {
					t.Errorf("FindPrevNext() next = %v, want %v", next, tt.wantNext)
				}
			}
		})
	}
}
