package searchpkg_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/tinylib/msgp/msgp"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
)

func sampleIndex() *searchpkg.SearchIndex {
	return &searchpkg.SearchIndex{
		SchemaVersion: searchpkg.CurrentSchemaVersion,
		Items: []searchpkg.ContentRecord{
			{
				Title:          "Hello World",
				Link:           "/blogs/hello.html",
				Description:    "A first post",
				Taxonomies:     map[string][]string{"tags": {"go", "web"}},
				NormalizedTaxs: map[string][]string{"tags": {"go", "web"}},
				Content:        "body text & more",
				Date:           1700000000,
			},
			{Title: "Second", Link: "/blogs/second.html", Taxonomies: map[string][]string{}},
		},
		ItemLens:   []int32{3, 7},
		AvgDocLen:  5.25,
		TotalItems: 2,
		Ranking: searchpkg.SearchRankingConfig{
			TitleBoost: 2, TagBoost: 1.5, DescriptionBoost: 1.2, BM25K1: 1.2, BM25B: 0.75,
		},
		StemMap:             map[string][]string{"run": {"running", "runs"}},
		Terms:               []string{"body", "hello", "second"},
		PostingOffsets:      []uint32{0, 1, 3},
		DocIDs:              []uint32{0, 0, 1},
		DocPosOffsets:       []uint32{0, 1, 2},
		PosOffsets:          []uint32{0, 1, 2},
		Positions:           []uint32{0, 0},
		TitlePostingOffsets: []uint32{0, 1},
		TitleDocIDs:         []uint32{0},
	}
}

func TestDecodeSearchIndexMatchesMsgp(t *testing.T) {
	want := sampleIndex()

	var buf bytes.Buffer
	mw := msgp.NewWriter(&buf)
	if err := want.EncodeMsg(mw); err != nil {
		t.Fatalf("EncodeMsg: %v", err)
	}
	if err := mw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// msgp's own decoder is the reference implementation.
	var reference searchpkg.SearchIndex
	if _, err := reference.UnmarshalMsg(buf.Bytes()); err != nil {
		t.Fatalf("UnmarshalMsg: %v", err)
	}

	got, err := searchpkg.DecodeSearchIndex(buf.Bytes())
	if err != nil {
		t.Fatalf("DecodeSearchIndex: %v", err)
	}
	if !reflect.DeepEqual(got, &reference) {
		t.Fatalf("decoded index differs from msgp\n got: %+v\nwant: %+v", got, &reference)
	}
}

func TestDecodeSearchIndexSkipsUnknownFields(t *testing.T) {
	// {"Unknown": [1, 2, {"k": "v"}], "TotalItems": 5}
	payload := []byte{
		0x82,
		0xa7, 'U', 'n', 'k', 'n', 'o', 'w', 'n',
		0x93, 0x01, 0x02, 0x81, 0xa1, 'k', 0xa1, 'v',
		0xaa, 'T', 'o', 't', 'a', 'l', 'I', 't', 'e', 'm', 's',
		0x05,
	}

	idx, err := searchpkg.DecodeSearchIndex(payload)
	if err != nil {
		t.Fatalf("DecodeSearchIndex: %v", err)
	}
	if idx.TotalItems != 5 {
		t.Fatalf("TotalItems = %d, want 5", idx.TotalItems)
	}
}

func TestDecodeSearchIndexEmptyPayload(t *testing.T) {
	if _, err := searchpkg.DecodeSearchIndex(nil); err == nil {
		t.Fatal("expected error for empty payload")
	}
	if _, err := searchpkg.DecodeSearchIndex([]byte{0xcb, 0x00}); err == nil {
		t.Fatal("expected error for non-map payload")
	}
}
