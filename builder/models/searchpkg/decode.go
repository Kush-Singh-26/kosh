package searchpkg

import (
	"encoding/binary"
	"errors"
	"math"
)

// This is a hand-rolled MessagePack decoder on purpose: it keeps tinylib/msgp
// out of the WASM build. msgp's init() builds JSON conversion jump tables that
// reference encoding/json, time and bufio, and init() is a linker root, so
// dead-code elimination cannot drop them (~2.7MB of unused code in search.wasm).
//
// Keep the field names below in sync with the names msgp writes for the structs
// in search.go. Unknown fields are skipped, so adding fields is safe.

var errCorruptPayload = errors.New("search index: corrupt payload")

type mpReader struct {
	b   []byte
	err error
}

func (r *mpReader) fail() {
	if r.err == nil {
		r.err = errCorruptPayload
	}
	r.b = nil
}

func (r *mpReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || len(r.b) < n {
		r.fail()
		return nil
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func (r *mpReader) u8() byte {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *mpReader) u16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}

func (r *mpReader) u32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

func (r *mpReader) u64() uint64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

// isNil reports whether the next value is MessagePack nil and consumes it.
func (r *mpReader) isNil() bool {
	if r.err != nil || len(r.b) == 0 || r.b[0] != 0xc0 {
		return false
	}
	r.b = r.b[1:]
	return true
}

// container reads a map or array header.
func (r *mpReader) container() (count int, isMap bool) {
	count, isMap, ok := r.containerHeader(r.u8())
	if !ok {
		r.fail()
	}
	return count, isMap
}

func (r *mpReader) containerHeader(c byte) (count int, isMap bool, ok bool) {
	switch {
	case c >= 0x80 && c <= 0x8f:
		return int(c & 0x0f), true, true
	case c >= 0x90 && c <= 0x9f:
		return int(c & 0x0f), false, true
	case c == 0xde:
		return int(r.u16()), true, true
	case c == 0xdc:
		return int(r.u16()), false, true
	case c == 0xdf:
		return int(r.u32()), true, true
	case c == 0xdd:
		return int(r.u32()), false, true
	default:
		return 0, false, false
	}
}

func (r *mpReader) str() string {
	c := r.u8()
	var n int
	switch {
	case c >= 0xa0 && c <= 0xbf:
		n = int(c & 0x1f)
	case c == 0xd9:
		n = int(r.u8())
	case c == 0xda:
		n = int(r.u16())
	case c == 0xdb:
		n = int(r.u32())
	default:
		r.fail()
		return ""
	}
	return string(r.take(n))
}

func (r *mpReader) int64() int64 {
	c := r.u8()
	switch {
	case c <= 0x7f:
		return int64(c)
	case c >= 0xe0:
		return int64(int8(c))
	case c == 0xd0:
		return int64(int8(r.u8()))
	case c == 0xcc:
		return int64(r.u8())
	case c == 0xd1:
		return int64(int16(r.u16()))
	case c == 0xcd:
		return int64(r.u16())
	case c == 0xd2:
		return int64(int32(r.u32()))
	case c == 0xce:
		return int64(r.u32())
	case c == 0xd3:
		return int64(r.u64())
	case c == 0xcf:
		u := r.u64()
		if u > math.MaxInt64 {
			r.fail()
			return 0
		}
		return int64(u)
	default:
		r.fail()
		return 0
	}
}

func (r *mpReader) uint32() uint32 {
	c := r.u8()
	switch {
	case c <= 0x7f:
		return uint32(c)
	case c == 0xcc:
		return uint32(r.u8())
	case c == 0xcd:
		return uint32(r.u16())
	case c == 0xce:
		return r.u32()
	case c == 0xd0:
		v := int8(r.u8())
		if v < 0 {
			r.fail()
		}
		return uint32(v)
	case c == 0xd1:
		v := int16(r.u16())
		if v < 0 {
			r.fail()
		}
		return uint32(v)
	case c == 0xd2:
		v := int32(r.u32())
		if v < 0 {
			r.fail()
		}
		return uint32(v)
	default:
		r.fail()
		return 0
	}
}

func (r *mpReader) float64() float64 {
	c := r.u8()
	switch {
	case c == 0xcb:
		return math.Float64frombits(r.u64())
	case c == 0xca:
		return float64(math.Float32frombits(r.u32()))
	case c <= 0x7f:
		return float64(c)
	case c >= 0xe0:
		return float64(int8(c))
	default:
		r.fail()
		return 0
	}
}

// skip consumes any value so unknown fields stay forward compatible.
func (r *mpReader) skip() {
	c := r.u8()
	switch {
	case c <= 0x7f || c >= 0xe0 || c == 0xc0 || c == 0xc2 || c == 0xc3:
		return
	case c == 0xcc || c == 0xd0:
		r.take(1)
	case c == 0xcd || c == 0xd1:
		r.take(2)
	case c == 0xce || c == 0xd2 || c == 0xca:
		r.take(4)
	case c == 0xcf || c == 0xd3 || c == 0xcb:
		r.take(8)
	case c >= 0xa0 && c <= 0xbf:
		r.take(int(c & 0x1f))
	case c == 0xd9:
		r.take(int(r.u8()))
	case c == 0xda:
		r.take(int(r.u16()))
	case c == 0xdb:
		r.take(int(r.u32()))
	case c == 0xc4:
		r.take(int(r.u8()))
	case c == 0xc5:
		r.take(int(r.u16()))
	case c == 0xc6:
		r.take(int(r.u32()))
	case c == 0xc7:
		r.take(int(r.u8()) + 1)
	case c == 0xc8:
		r.take(int(r.u16()) + 1)
	case c == 0xc9:
		r.take(int(r.u32()) + 1)
	case c == 0xd4:
		r.take(2)
	case c == 0xd5:
		r.take(3)
	case c == 0xd6:
		r.take(5)
	case c == 0xd7:
		r.take(9)
	case c == 0xd8:
		r.take(17)
	default:
		count, isMap, ok := r.containerHeader(c)
		if !ok {
			r.fail()
			return
		}
		if isMap {
			count *= 2
		}
		for range count {
			r.skip()
		}
	}
}

func (r *mpReader) stringSlice() []string {
	if r.isNil() {
		return nil
	}
	count, isMap := r.container()
	if isMap {
		r.fail()
		return nil
	}
	out := make([]string, count)
	for i := range out {
		out[i] = r.str()
	}
	return out
}

func (r *mpReader) stringSliceMap() map[string][]string {
	if r.isNil() {
		return nil
	}
	count, isMap := r.container()
	if !isMap {
		r.fail()
		return nil
	}
	out := make(map[string][]string, count)
	for range count {
		key := r.str()
		out[key] = r.stringSlice()
	}
	return out
}

func (r *mpReader) uint32Slice() []uint32 {
	if r.isNil() {
		return nil
	}
	count, isMap := r.container()
	if isMap {
		r.fail()
		return nil
	}
	out := make([]uint32, count)
	for i := range out {
		out[i] = r.uint32()
	}
	return out
}

func (r *mpReader) int32Slice() []int32 {
	if r.isNil() {
		return nil
	}
	count, isMap := r.container()
	if isMap {
		r.fail()
		return nil
	}
	out := make([]int32, count)
	for i := range out {
		out[i] = int32(r.int64())
	}
	return out
}

func (r *mpReader) record() ContentRecord {
	var rec ContentRecord
	count, isMap := r.container()
	if !isMap {
		r.fail()
		return rec
	}
	for range count {
		switch r.str() {
		case "Title":
			rec.Title = r.str()
		case "Link":
			rec.Link = r.str()
		case "Description":
			rec.Description = r.str()
		case "Taxonomies":
			rec.Taxonomies = r.stringSliceMap()
		case "NormalizedTaxs":
			rec.NormalizedTaxs = r.stringSliceMap()
		case "Content":
			rec.Content = r.str()
		case "Date":
			rec.Date = r.int64()
		default:
			r.skip()
		}
	}
	return rec
}

func (r *mpReader) ranking() SearchRankingConfig {
	var cfg SearchRankingConfig
	count, isMap := r.container()
	if !isMap {
		r.fail()
		return cfg
	}
	for range count {
		switch r.str() {
		case "TitleBoost":
			cfg.TitleBoost = r.float64()
		case "TagBoost":
			cfg.TagBoost = r.float64()
		case "DescriptionBoost":
			cfg.DescriptionBoost = r.float64()
		case "BM25K1":
			cfg.BM25K1 = r.float64()
		case "BM25B":
			cfg.BM25B = r.float64()
		default:
			r.skip()
		}
	}
	return cfg
}

// DecodeSearchIndex parses the MessagePack encoded search index written by
// builder/generators/search.go.
func DecodeSearchIndex(bts []byte) (*SearchIndex, error) {
	r := &mpReader{b: bts}
	idx := &SearchIndex{}
	count, isMap := r.container()
	if !isMap {
		return nil, errCorruptPayload
	}
	for range count {
		switch r.str() {
		case "SchemaVersion":
			idx.SchemaVersion = r.int64()
		case "Items":
			if !r.isNil() {
				items, _ := r.container()
				idx.Items = make([]ContentRecord, items)
				for i := range idx.Items {
					idx.Items[i] = r.record()
				}
			}
		case "ItemLens":
			idx.ItemLens = r.int32Slice()
		case "AvgDocLen":
			idx.AvgDocLen = r.float64()
		case "TotalItems":
			idx.TotalItems = r.int64()
		case "Ranking":
			idx.Ranking = r.ranking()
		case "StemMap":
			idx.StemMap = r.stringSliceMap()
		case "Terms":
			idx.Terms = r.stringSlice()
		case "PostingOffsets":
			idx.PostingOffsets = r.uint32Slice()
		case "DocIDs":
			idx.DocIDs = r.uint32Slice()
		case "DocPosOffsets":
			idx.DocPosOffsets = r.uint32Slice()
		case "PosOffsets":
			idx.PosOffsets = r.uint32Slice()
		case "Positions":
			idx.Positions = r.uint32Slice()
		case "TitlePostingOffsets":
			idx.TitlePostingOffsets = r.uint32Slice()
		case "TitleDocIDs":
			idx.TitleDocIDs = r.uint32Slice()
		default:
			r.skip()
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	return idx, nil
}
