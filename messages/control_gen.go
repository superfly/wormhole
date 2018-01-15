package messages

// NOTE: THIS FILE WAS PRODUCED BY THE
// MSGP CODE GENERATION TOOL (github.com/tinylib/msgp)
// DO NOT EDIT

import (
	"github.com/tinylib/msgp/msgp"
)

// DecodeMsg implements msgp.Decodable
func (z *AuthControl) DecodeMsg(dc *msgp.Reader) (err error) {
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
		case "Token":
			z.Token, err = dc.ReadString()
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
func (z AuthControl) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "Token"
	err = en.Append(0x81, 0xa5, 0x54, 0x6f, 0x6b, 0x65, 0x6e)
	if err != nil {
		return
	}
	err = en.WriteString(z.Token)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z AuthControl) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "Token"
	o = append(o, 0x81, 0xa5, 0x54, 0x6f, 0x6b, 0x65, 0x6e)
	o = msgp.AppendString(o, z.Token)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *AuthControl) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
		case "Token":
			z.Token, bts, err = msgp.ReadStringBytes(bts)
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
func (z AuthControl) Msgsize() (s int) {
	s = 1 + 6 + msgp.StringPrefixSize + len(z.Token)
	return
}

// DecodeMsg implements msgp.Decodable
func (z *AuthTunnel) DecodeMsg(dc *msgp.Reader) (err error) {
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
		case "ClientID":
			z.ClientID, err = dc.ReadString()
			if err != nil {
				return
			}
		case "Token":
			z.Token, err = dc.ReadString()
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
func (z AuthTunnel) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 2
	// write "ClientID"
	err = en.Append(0x82, 0xa8, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x49, 0x44)
	if err != nil {
		return
	}
	err = en.WriteString(z.ClientID)
	if err != nil {
		return
	}
	// write "Token"
	err = en.Append(0xa5, 0x54, 0x6f, 0x6b, 0x65, 0x6e)
	if err != nil {
		return
	}
	err = en.WriteString(z.Token)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z AuthTunnel) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 2
	// string "ClientID"
	o = append(o, 0x82, 0xa8, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x49, 0x44)
	o = msgp.AppendString(o, z.ClientID)
	// string "Token"
	o = append(o, 0xa5, 0x54, 0x6f, 0x6b, 0x65, 0x6e)
	o = msgp.AppendString(o, z.Token)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *AuthTunnel) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
		case "ClientID":
			z.ClientID, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "Token":
			z.Token, bts, err = msgp.ReadStringBytes(bts)
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
func (z AuthTunnel) Msgsize() (s int) {
	s = 1 + 9 + msgp.StringPrefixSize + len(z.ClientID) + 6 + msgp.StringPrefixSize + len(z.Token)
	return
}

// DecodeMsg implements msgp.Decodable
func (z *Envelope) DecodeMsg(dc *msgp.Reader) (err error) {
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
		case "Type":
			{
				var zb0002 int
				zb0002, err = dc.ReadInt()
				if err != nil {
					return
				}
				z.Type = MessageType(zb0002)
			}
		case "Payload":
			err = z.Payload.DecodeMsg(dc)
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
func (z *Envelope) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 2
	// write "Type"
	err = en.Append(0x82, 0xa4, 0x54, 0x79, 0x70, 0x65)
	if err != nil {
		return
	}
	err = en.WriteInt(int(z.Type))
	if err != nil {
		return
	}
	// write "Payload"
	err = en.Append(0xa7, 0x50, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64)
	if err != nil {
		return
	}
	err = z.Payload.EncodeMsg(en)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *Envelope) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 2
	// string "Type"
	o = append(o, 0x82, 0xa4, 0x54, 0x79, 0x70, 0x65)
	o = msgp.AppendInt(o, int(z.Type))
	// string "Payload"
	o = append(o, 0xa7, 0x50, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64)
	o, err = z.Payload.MarshalMsg(o)
	if err != nil {
		return
	}
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *Envelope) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
		case "Type":
			{
				var zb0002 int
				zb0002, bts, err = msgp.ReadIntBytes(bts)
				if err != nil {
					return
				}
				z.Type = MessageType(zb0002)
			}
		case "Payload":
			bts, err = z.Payload.UnmarshalMsg(bts)
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
func (z *Envelope) Msgsize() (s int) {
	s = 1 + 5 + msgp.IntSize + 8 + z.Payload.Msgsize()
	return
}

// DecodeMsg implements msgp.Decodable
func (z *MessageType) DecodeMsg(dc *msgp.Reader) (err error) {
	{
		var zb0001 int
		zb0001, err = dc.ReadInt()
		if err != nil {
			return
		}
		(*z) = MessageType(zb0001)
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z MessageType) EncodeMsg(en *msgp.Writer) (err error) {
	err = en.WriteInt(int(z))
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z MessageType) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	o = msgp.AppendInt(o, int(z))
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *MessageType) UnmarshalMsg(bts []byte) (o []byte, err error) {
	{
		var zb0001 int
		zb0001, bts, err = msgp.ReadIntBytes(bts)
		if err != nil {
			return
		}
		(*z) = MessageType(zb0001)
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z MessageType) Msgsize() (s int) {
	s = msgp.IntSize
	return
}

// DecodeMsg implements msgp.Decodable
func (z *OpenTunnel) DecodeMsg(dc *msgp.Reader) (err error) {
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
		case "ClientID":
			z.ClientID, err = dc.ReadString()
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
func (z OpenTunnel) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "ClientID"
	err = en.Append(0x81, 0xa8, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x49, 0x44)
	if err != nil {
		return
	}
	err = en.WriteString(z.ClientID)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z OpenTunnel) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "ClientID"
	o = append(o, 0x81, 0xa8, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x49, 0x44)
	o = msgp.AppendString(o, z.ClientID)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *OpenTunnel) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
		case "ClientID":
			z.ClientID, bts, err = msgp.ReadStringBytes(bts)
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
func (z OpenTunnel) Msgsize() (s int) {
	s = 1 + 9 + msgp.StringPrefixSize + len(z.ClientID)
	return
}

// DecodeMsg implements msgp.Decodable
func (z *Ping) DecodeMsg(dc *msgp.Reader) (err error) {
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
func (z Ping) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 0
	err = en.Append(0x80)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z Ping) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 0
	o = append(o, 0x80)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *Ping) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
func (z Ping) Msgsize() (s int) {
	s = 1
	return
}

// DecodeMsg implements msgp.Decodable
func (z *Pong) DecodeMsg(dc *msgp.Reader) (err error) {
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
func (z Pong) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 0
	err = en.Append(0x80)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z Pong) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 0
	o = append(o, 0x80)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *Pong) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
func (z Pong) Msgsize() (s int) {
	s = 1
	return
}

// DecodeMsg implements msgp.Decodable
func (z *Release) DecodeMsg(dc *msgp.Reader) (err error) {
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
		case "ID":
			z.ID, err = dc.ReadString()
			if err != nil {
				return
			}
		case "Branch":
			z.Branch, err = dc.ReadString()
			if err != nil {
				return
			}
		case "Description":
			z.Description, err = dc.ReadString()
			if err != nil {
				return
			}
		case "VCSType":
			z.VCSType, err = dc.ReadString()
			if err != nil {
				return
			}
		case "VCSRevision":
			z.VCSRevision, err = dc.ReadString()
			if err != nil {
				return
			}
		case "VCSRevisionMessage":
			z.VCSRevisionMessage, err = dc.ReadString()
			if err != nil {
				return
			}
		case "VCSRevisionTime":
			z.VCSRevisionTime, err = dc.ReadTime()
			if err != nil {
				return
			}
		case "VCSRevisionAuthorName":
			z.VCSRevisionAuthorName, err = dc.ReadString()
			if err != nil {
				return
			}
		case "VCSRevisionAuthorEmail":
			z.VCSRevisionAuthorEmail, err = dc.ReadString()
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
func (z *Release) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 9
	// write "ID"
	err = en.Append(0x89, 0xa2, 0x49, 0x44)
	if err != nil {
		return
	}
	err = en.WriteString(z.ID)
	if err != nil {
		return
	}
	// write "Branch"
	err = en.Append(0xa6, 0x42, 0x72, 0x61, 0x6e, 0x63, 0x68)
	if err != nil {
		return
	}
	err = en.WriteString(z.Branch)
	if err != nil {
		return
	}
	// write "Description"
	err = en.Append(0xab, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e)
	if err != nil {
		return
	}
	err = en.WriteString(z.Description)
	if err != nil {
		return
	}
	// write "VCSType"
	err = en.Append(0xa7, 0x56, 0x43, 0x53, 0x54, 0x79, 0x70, 0x65)
	if err != nil {
		return
	}
	err = en.WriteString(z.VCSType)
	if err != nil {
		return
	}
	// write "VCSRevision"
	err = en.Append(0xab, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e)
	if err != nil {
		return
	}
	err = en.WriteString(z.VCSRevision)
	if err != nil {
		return
	}
	// write "VCSRevisionMessage"
	err = en.Append(0xb2, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65)
	if err != nil {
		return
	}
	err = en.WriteString(z.VCSRevisionMessage)
	if err != nil {
		return
	}
	// write "VCSRevisionTime"
	err = en.Append(0xaf, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x54, 0x69, 0x6d, 0x65)
	if err != nil {
		return
	}
	err = en.WriteTime(z.VCSRevisionTime)
	if err != nil {
		return
	}
	// write "VCSRevisionAuthorName"
	err = en.Append(0xb5, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x41, 0x75, 0x74, 0x68, 0x6f, 0x72, 0x4e, 0x61, 0x6d, 0x65)
	if err != nil {
		return
	}
	err = en.WriteString(z.VCSRevisionAuthorName)
	if err != nil {
		return
	}
	// write "VCSRevisionAuthorEmail"
	err = en.Append(0xb6, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x41, 0x75, 0x74, 0x68, 0x6f, 0x72, 0x45, 0x6d, 0x61, 0x69, 0x6c)
	if err != nil {
		return
	}
	err = en.WriteString(z.VCSRevisionAuthorEmail)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *Release) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 9
	// string "ID"
	o = append(o, 0x89, 0xa2, 0x49, 0x44)
	o = msgp.AppendString(o, z.ID)
	// string "Branch"
	o = append(o, 0xa6, 0x42, 0x72, 0x61, 0x6e, 0x63, 0x68)
	o = msgp.AppendString(o, z.Branch)
	// string "Description"
	o = append(o, 0xab, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e)
	o = msgp.AppendString(o, z.Description)
	// string "VCSType"
	o = append(o, 0xa7, 0x56, 0x43, 0x53, 0x54, 0x79, 0x70, 0x65)
	o = msgp.AppendString(o, z.VCSType)
	// string "VCSRevision"
	o = append(o, 0xab, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e)
	o = msgp.AppendString(o, z.VCSRevision)
	// string "VCSRevisionMessage"
	o = append(o, 0xb2, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65)
	o = msgp.AppendString(o, z.VCSRevisionMessage)
	// string "VCSRevisionTime"
	o = append(o, 0xaf, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x54, 0x69, 0x6d, 0x65)
	o = msgp.AppendTime(o, z.VCSRevisionTime)
	// string "VCSRevisionAuthorName"
	o = append(o, 0xb5, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x41, 0x75, 0x74, 0x68, 0x6f, 0x72, 0x4e, 0x61, 0x6d, 0x65)
	o = msgp.AppendString(o, z.VCSRevisionAuthorName)
	// string "VCSRevisionAuthorEmail"
	o = append(o, 0xb6, 0x56, 0x43, 0x53, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x41, 0x75, 0x74, 0x68, 0x6f, 0x72, 0x45, 0x6d, 0x61, 0x69, 0x6c)
	o = msgp.AppendString(o, z.VCSRevisionAuthorEmail)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *Release) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
		case "ID":
			z.ID, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "Branch":
			z.Branch, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "Description":
			z.Description, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "VCSType":
			z.VCSType, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "VCSRevision":
			z.VCSRevision, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "VCSRevisionMessage":
			z.VCSRevisionMessage, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "VCSRevisionTime":
			z.VCSRevisionTime, bts, err = msgp.ReadTimeBytes(bts)
			if err != nil {
				return
			}
		case "VCSRevisionAuthorName":
			z.VCSRevisionAuthorName, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "VCSRevisionAuthorEmail":
			z.VCSRevisionAuthorEmail, bts, err = msgp.ReadStringBytes(bts)
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
func (z *Release) Msgsize() (s int) {
	s = 1 + 3 + msgp.StringPrefixSize + len(z.ID) + 7 + msgp.StringPrefixSize + len(z.Branch) + 12 + msgp.StringPrefixSize + len(z.Description) + 8 + msgp.StringPrefixSize + len(z.VCSType) + 12 + msgp.StringPrefixSize + len(z.VCSRevision) + 19 + msgp.StringPrefixSize + len(z.VCSRevisionMessage) + 16 + msgp.TimeSize + 22 + msgp.StringPrefixSize + len(z.VCSRevisionAuthorName) + 23 + msgp.StringPrefixSize + len(z.VCSRevisionAuthorEmail)
	return
}

// DecodeMsg implements msgp.Decodable
func (z *Shutdown) DecodeMsg(dc *msgp.Reader) (err error) {
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
		case "Error":
			z.Error, err = dc.ReadString()
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
func (z Shutdown) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 1
	// write "Error"
	err = en.Append(0x81, 0xa5, 0x45, 0x72, 0x72, 0x6f, 0x72)
	if err != nil {
		return
	}
	err = en.WriteString(z.Error)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z Shutdown) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 1
	// string "Error"
	o = append(o, 0x81, 0xa5, 0x45, 0x72, 0x72, 0x6f, 0x72)
	o = msgp.AppendString(o, z.Error)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *Shutdown) UnmarshalMsg(bts []byte) (o []byte, err error) {
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
		case "Error":
			z.Error, bts, err = msgp.ReadStringBytes(bts)
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
func (z Shutdown) Msgsize() (s int) {
	s = 1 + 6 + msgp.StringPrefixSize + len(z.Error)
	return
}
