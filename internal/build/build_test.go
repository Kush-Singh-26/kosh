package build

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func TestCheckWASMFs(t *testing.T) {
	utils.TestingMode = true
	defer func() { utils.TestingMode = false }()

	fs := afero.NewMemMapFs()
	
	// Test initial write — files should land under outputDir, not CWD
	changed := CheckWASMFs(fs, "public")
	if !changed {
		t.Error("CheckWASMFs should return true on first write")
	}

	exists, _ := afero.Exists(fs, "public/static/wasm/search.wasm")
	if !exists {
		t.Error("search.wasm was not created under outputDir")
	}

	exists, _ = afero.Exists(fs, "public/static/wasm/search.wasm.gz")
	if !exists {
		t.Error("search.wasm.gz was not created under outputDir")
	}

	// Verify nothing was written to the bare CWD-relative path (the old bug)
	wrongPath, _ := afero.Exists(fs, "static/wasm/search.wasm")
	if wrongPath {
		t.Error("search.wasm must NOT be written to bare 'static/wasm/' (source tree contamination)")
	}

	// Test skip when identical
	changed = CheckWASMFs(fs, "public")
	if changed {
		t.Error("CheckWASMFs should return false when WASM is already up-to-date")
	}
}
