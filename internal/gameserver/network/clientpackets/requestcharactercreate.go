package clientpackets

import "fmt"

// OpcodeRequestCharacterCreate is the wire opcode for RequestCharacterCreate,
// valid once a client is authenticated.
const OpcodeRequestCharacterCreate = 0x0b

// RequestCharacterCreate carries a new character's profession choice and
// appearance. Every field is exactly what the client sent, unvalidated:
// checking it against actual bounds and building a character from it is the
// roster's job, not this decoder's.
type RequestCharacterCreate struct {
	Name                       string
	Race                       int32
	Sex                        int32
	ClassID                    int32
	HairStyle, HairColor, Face byte
}

// DecodeRequestCharacterCreate parses a raw RequestCharacterCreate payload
// (opcode byte included). Six stat fields the client sends between the
// profession choice and the appearance fields are read and discarded: this
// server derives a new character's stats from its profession template, not
// from client-supplied numbers.
func DecodeRequestCharacterCreate(payload []byte) (RequestCharacterCreate, error) {
	r := newReader(payload)

	req := RequestCharacterCreate{
		Name:    r.ReadString(),
		Race:    r.ReadInt32(),
		Sex:     r.ReadInt32(),
		ClassID: r.ReadInt32(),
	}
	for i := 0; i < 6; i++ {
		r.ReadInt32()
	}
	req.HairStyle = byte(r.ReadInt32())
	req.HairColor = byte(r.ReadInt32())
	req.Face = byte(r.ReadInt32())

	if err := r.Err(); err != nil {
		return RequestCharacterCreate{}, fmt.Errorf("clientpackets: RequestCharacterCreate: %w", err)
	}
	return req, nil
}
