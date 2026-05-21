package searchpkg

// nolint: funlen

import (
	"github.com/tinylib/msgp/msgp"
)

// DecodeMsg implements msgp.Decodable
//
//nolint:funlen
func (z *SearchRankingConfig) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "TitleBoost":
			z.TitleBoost, err = dc.ReadFloat64()
			if err != nil {
				err = msgp.WrapError(err, "TitleBoost")
				return
			}
		case "TagBoost":
			z.TagBoost, err = dc.ReadFloat64()
			if err != nil {
				err = msgp.WrapError(err, "TagBoost")
				return
			}
		case "DescriptionBoost":
			z.DescriptionBoost, err = dc.ReadFloat64()
			if err != nil {
				err = msgp.WrapError(err, "DescriptionBoost")
				return
			}
		case "BM25K1":
			z.BM25K1, err = dc.ReadFloat64()
			if err != nil {
				err = msgp.WrapError(err, "BM25K1")
				return
			}
		case "BM25B":
			z.BM25B, err = dc.ReadFloat64()
			if err != nil {
				err = msgp.WrapError(err, "BM25B")
				return
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *SearchRankingConfig) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 5
	// write "TitleBoost"
	err = en.Append(0x85, 0xaa, 0x54, 0x69, 0x74, 0x6c, 0x65, 0x42, 0x6f, 0x6f, 0x73, 0x74)
	if err != nil {
		return
	}
	err = en.WriteFloat64(z.TitleBoost)
	if err != nil {
		err = msgp.WrapError(err, "TitleBoost")
		return
	}
	// write "TagBoost"
	err = en.Append(0xa8, 0x54, 0x61, 0x67, 0x42, 0x6f, 0x6f, 0x73, 0x74)
	if err != nil {
		return
	}
	err = en.WriteFloat64(z.TagBoost)
	if err != nil {
		err = msgp.WrapError(err, "TagBoost")
		return
	}
	// write "DescriptionBoost"
	err = en.Append(0xb0, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x42, 0x6f, 0x6f, 0x73, 0x74)
	if err != nil {
		return
	}
	err = en.WriteFloat64(z.DescriptionBoost)
	if err != nil {
		err = msgp.WrapError(err, "DescriptionBoost")
		return
	}
	// write "BM25K1"
	err = en.Append(0xa6, 0x42, 0x4d, 0x32, 0x35, 0x4b, 0x31)
	if err != nil {
		return
	}
	err = en.WriteFloat64(z.BM25K1)
	if err != nil {
		err = msgp.WrapError(err, "BM25K1")
		return
	}
	// write "BM25B"
	err = en.Append(0xa5, 0x42, 0x4d, 0x32, 0x35, 0x42)
	if err != nil {
		return
	}
	err = en.WriteFloat64(z.BM25B)
	if err != nil {
		err = msgp.WrapError(err, "BM25B")
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *SearchRankingConfig) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 5
	// string "TitleBoost"
	o = append(o, 0x85, 0xaa, 0x54, 0x69, 0x74, 0x6c, 0x65, 0x42, 0x6f, 0x6f, 0x73, 0x74)
	o = msgp.AppendFloat64(o, z.TitleBoost)
	// string "TagBoost"
	o = append(o, 0xa8, 0x54, 0x61, 0x67, 0x42, 0x6f, 0x6f, 0x73, 0x74)
	o = msgp.AppendFloat64(o, z.TagBoost)
	// string "DescriptionBoost"
	o = append(o, 0xb0, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x42, 0x6f, 0x6f, 0x73, 0x74)
	o = msgp.AppendFloat64(o, z.DescriptionBoost)
	// string "BM25K1"
	o = append(o, 0xa6, 0x42, 0x4d, 0x32, 0x35, 0x4b, 0x31)
	o = msgp.AppendFloat64(o, z.BM25K1)
	// string "BM25B"
	o = append(o, 0xa5, 0x42, 0x4d, 0x32, 0x35, 0x42)
	o = msgp.AppendFloat64(o, z.BM25B)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
//
//nolint:funlen
func (z *SearchRankingConfig) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "TitleBoost":
			z.TitleBoost, bts, err = msgp.ReadFloat64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "TitleBoost")
				return
			}
		case "TagBoost":
			z.TagBoost, bts, err = msgp.ReadFloat64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "TagBoost")
				return
			}
		case "DescriptionBoost":
			z.DescriptionBoost, bts, err = msgp.ReadFloat64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "DescriptionBoost")
				return
			}
		case "BM25K1":
			z.BM25K1, bts, err = msgp.ReadFloat64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "BM25K1")
				return
			}
		case "BM25B":
			z.BM25B, bts, err = msgp.ReadFloat64Bytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "BM25B")
				return
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *SearchRankingConfig) Msgsize() (s int) {
	s = 1 + 11 + msgp.Float64Size + 9 + msgp.Float64Size + 17 + msgp.Float64Size + 7 + msgp.Float64Size + 6 + msgp.Float64Size
	return
}
