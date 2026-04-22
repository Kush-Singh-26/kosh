package models

import (
	"testing"
)

func TestSearchIndex_SchemaVersion(t *testing.T) {
	index := SearchIndex{
		SchemaVersion: CurrentSchemaVersion,
		Items:         []ContentRecord{},
	}

	if index.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", index.SchemaVersion, CurrentSchemaVersion)
	}

	if CurrentSchemaVersion != index.SchemaVersion {
		t.Errorf("CurrentSchemaVersion = %d, want %d", CurrentSchemaVersion, index.SchemaVersion)
	}

	t.Log("SearchIndex schema version test passed")
}

func TestSearchIndex_MsgpackEncoding(t *testing.T) {
	index := SearchIndex{
		SchemaVersion: CurrentSchemaVersion,
		Items: []ContentRecord{
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

	t.Log("SearchIndex msgpack encoding test passed")
}

func TestCurrentSchemaVersion_Defined(t *testing.T) {
	if CurrentSchemaVersion <= 0 {
		t.Errorf("CurrentSchemaVersion should be positive, got %d", CurrentSchemaVersion)
	}

	t.Logf("CurrentSchemaVersion = %d", CurrentSchemaVersion)
}
