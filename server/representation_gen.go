package server

// NOTE: THIS FILE WAS PRODUCED BY THE
// MSGP CODE GENERATION TOOL (github.com/tinylib/msgp)
// DO NOT EDIT

import (
	"github.com/tinylib/msgp/msgp"
)

// DecodeMsg implements msgp.Decodable
func (z *Representation) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			return
		}
		switch msgp.UnsafeString(field) {
		case "url":
			z.Address, err = dc.ReadString()
			if err != nil {
				return
			}
		case "port":
			z.Port, err = dc.ReadString()
			if err != nil {
				return
			}
		case "region":
			z.Region, err = dc.ReadString()
			if err != nil {
				return
			}
		default:
			err = dc.Skip()
			if err != nil {
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z Representation) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 3
	// write "url"
	err = en.Append(0x83, 0xa3, 0x75, 0x72, 0x6c)
	if err != nil {
		return
	}
	err = en.WriteString(z.Address)
	if err != nil {
		return
	}
	// write "port"
	err = en.Append(0xa4, 0x70, 0x6f, 0x72, 0x74)
	if err != nil {
		return
	}
	err = en.WriteString(z.Port)
	if err != nil {
		return
	}
	// write "region"
	err = en.Append(0xa6, 0x72, 0x65, 0x67, 0x69, 0x6f, 0x6e)
	if err != nil {
		return
	}
	err = en.WriteString(z.Region)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z Representation) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 3
	// string "url"
	o = append(o, 0x83, 0xa3, 0x75, 0x72, 0x6c)
	o = msgp.AppendString(o, z.Address)
	// string "port"
	o = append(o, 0xa4, 0x70, 0x6f, 0x72, 0x74)
	o = msgp.AppendString(o, z.Port)
	// string "region"
	o = append(o, 0xa6, 0x72, 0x65, 0x67, 0x69, 0x6f, 0x6e)
	o = msgp.AppendString(o, z.Region)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *Representation) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			return
		}
		switch msgp.UnsafeString(field) {
		case "url":
			z.Address, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "port":
			z.Port, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "region":
			z.Region, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z Representation) Msgsize() (s int) {
	s = 1 + 4 + msgp.StringPrefixSize + len(z.Address) + 5 + msgp.StringPrefixSize + len(z.Port) + 7 + msgp.StringPrefixSize + len(z.Region)
	return
}
