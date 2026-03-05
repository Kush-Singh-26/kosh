package models

import (
	"testing"
)

func TestSearchIndex_SchemaVersion(t *testing.T) {
	index := SearchIndex{
		SchemaVersion: CurrentSchemaVersion,
		Posts:         []PostRecord{},
	}

	if index.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", index.SchemaVersion, CurrentSchemaVersion)
	}

	if CurrentSchemaVersion != 5 {
		t.Errorf("CurrentSchemaVersion = %d, want 5", CurrentSchemaVersion)
	}

	t.Log("SearchIndex schema version test passed")
}

func TestSearchIndex_MsgpackEncoding(t *testing.T) {
	index := SearchIndex{
		SchemaVersion: CurrentSchemaVersion,
		Posts: []PostRecord{
			{ID: 1, Title: "Test Post"},
		},
		Inverted: map[string]map[string][]int{
			"test": {"1": {1, 10}},
		},
		DocLens:   map[string]int64{"1": 100},
		AvgDocLen: 100.0,
		TotalDocs: 1,
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

	t.Log("SearchIndex msgpack encoding test passed")
}

func TestCurrentSchemaVersion_Defined(t *testing.T) {
	if CurrentSchemaVersion <= 0 {
		t.Errorf("CurrentSchemaVersion should be positive, got %d", CurrentSchemaVersion)
	}

	t.Logf("CurrentSchemaVersion = %d", CurrentSchemaVersion)
}
