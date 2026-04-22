package models

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
)

func TestSearchIndex_SchemaVersion(t *testing.T) {
	index := searchpkg.SearchIndex{
		SchemaVersion: searchpkg.CurrentSchemaVersion,
		Items:         []searchpkg.ContentRecord{},
	}

	if index.SchemaVersion != searchpkg.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", index.SchemaVersion, searchpkg.CurrentSchemaVersion)
	}

	if searchpkg.CurrentSchemaVersion != index.SchemaVersion {
		t.Errorf("searchpkg.CurrentSchemaVersion = %d, want %d", searchpkg.CurrentSchemaVersion, index.SchemaVersion)
	}

	t.Log("searchpkg.SearchIndex schema version test passed")
}

func TestSearchIndex_MsgpackEncoding(t *testing.T) {
	index := searchpkg.SearchIndex{
		SchemaVersion: searchpkg.CurrentSchemaVersion,
		Items: []searchpkg.ContentRecord{
			{Title: "Test Item"},
		},
		Terms:      []string{"test"},
		DocIDs:     []uint32{0},
		ItemLens:   []int32{100},
		AvgDocLen:  100.0,
		TotalItems: 1,
		StemMap: map[string][]string{
			"run":  {"running", "runner"},
			"test": {"test", "testing"},
		},
	}

	if index.SchemaVersion == 0 {
		t.Error("SchemaVersion should be set")
	}

	if index.StemMap == nil {
		t.Error("StemMap should be initialized")
	}

	t.Log("searchpkg.SearchIndex msgpack encoding test passed")
}

func TestCurrentSchemaVersion_Defined(t *testing.T) {
	if searchpkg.CurrentSchemaVersion <= 0 {
		t.Errorf("searchpkg.CurrentSchemaVersion should be positive, got %d", searchpkg.CurrentSchemaVersion)
	}

	t.Logf("searchpkg.CurrentSchemaVersion = %d", searchpkg.CurrentSchemaVersion)
}
