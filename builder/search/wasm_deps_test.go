package search_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The search WASM runs in the browser, so anything linked into it is downloaded
// by every visitor. These packages pulled in megabytes of unused code before
// (builder/models pulls D2 + html/template, msgp's init() pulls encoding/json).
func TestWasmSearchDependencyGraphStaysLean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go list in short mode")
	}

	cmd := exec.Command("go", "list", "-buildvcs=false", "-deps", "-f", "{{.ImportPath}}", "./cmd/search")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list failed: %v\n%s", err, stderr)
	}

	forbidden := map[string]string{
		"github.com/Kush-Singh-26/kosh/builder/models": "drags in D2, html/template and encoding/xml",
		"github.com/tinylib/msgp/msgp":                 "init() links encoding/json and time",
		"oss.terrastruct.com/d2/d2target":              "D2 diagram engine has no place in the browser",
		"log/slog":                                     "logging in the browser search is not worth the bytes",
		"encoding/json/v2":                             "unused JSON machinery",
	}

	for _, pkg := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if reason, bad := forbidden[strings.TrimSpace(pkg)]; bad {
			t.Errorf("search.wasm links %q: %s\nKeep it out of the wasm build (build tags on the offending imports).", pkg, reason)
		}
	}
}
