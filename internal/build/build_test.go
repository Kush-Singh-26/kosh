package build

import (
	"testing"

	"github.com/spf13/afero"
)

func TestCheckWASMFs(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Test initial write — files should land under outputDir, not CWD
	changed := CheckWASMFs(fs, "public", "")
	if !changed {
		t.Error("CheckWASMFs should return true on first write")
	}

	exists, _ := afero.Exists(fs, "public/static/wasm/search.wasm")
	if !exists {
		t.Error("search.wasm was not created under outputDir")
	}

	exists, _ = afero.Exists(fs, "public/static/wasm/search.wasm.br")
	if !exists {
		t.Error("search.wasm.br was not created under outputDir")
	}

	// Verify nothing was written to the bare CWD-relative path (the old bug)
	wrongPath, _ := afero.Exists(fs, "static/wasm/search.wasm")
	if wrongPath {
		t.Error("search.wasm must NOT be written to bare 'static/wasm/' (source tree contamination)")
	}

	// Test skip when identical
	changed = CheckWASMFs(fs, "public", "")
	if changed {
		t.Error("CheckWASMFs should return false when WASM is already up-to-date")
	}
}
