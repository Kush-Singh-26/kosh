package fs

import "testing"

func TestNormalizeAssetURLHashesQuotedURL(t *testing.T) {
	in := []byte(`@font-face{src:url("../assets/KaTeX_Main-Regular.ABCDEF12.woff2") format("woff2")}`)
	out := string(normalizeAssetURLHashes(in))
	want := `@font-face{src:url("../assets/KaTeX_Main-Regular.abcdef12.woff2") format("woff2")}`

	if out != want {
		t.Fatalf("unexpected output:\nwant: %s\ngot:  %s", want, out)
	}
}
