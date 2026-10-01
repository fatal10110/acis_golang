package clientpackets

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeRequestSetPledgeCrest is the wire opcode for RequestSetPledgeCrest,
// valid once a client is in game.
const OpcodeRequestSetPledgeCrest = 0x53

// OpcodeRequestExSetPledgeCrestLarge is the extended sub-opcode of
// RequestExSetPledgeCrestLarge.
const OpcodeRequestExSetPledgeCrestLarge uint16 = 0x0011

// The largest image each crest upload carries; a longer declared length is
// read as no image at all.
const (
	PledgeCrestMaxLength      = 256
	LargePledgeCrestMaxLength = 2176
)

// errNegativeCrestLength rejects a crest upload declaring a negative image
// length. It is not a short packet: the packet is dropped without counting
// as a buffer underflow.
var errNegativeCrestLength = errors.New("negative crest length")

// CrestUpload is a crest image the client sends: Length is the declared
// image length and Data the image. When Length exceeds the packet's limit
// the image is not read and Data is nil.
type CrestUpload struct {
	Length int32
	Data   []byte
}

// DecodeRequestSetPledgeCrest parses a raw RequestSetPledgeCrest payload
// (opcode byte included).
func DecodeRequestSetPledgeCrest(payload []byte) (CrestUpload, error) {
	return readCrestUpload(newReader(payload), "RequestSetPledgeCrest", PledgeCrestMaxLength)
}

// DecodeRequestExSetPledgeCrestLarge parses a raw extended
// RequestExSetPledgeCrestLarge payload (opcode byte included).
func DecodeRequestExSetPledgeCrestLarge(payload []byte) (CrestUpload, error) {
	r := newReader(payload)
	second := r.ReadUint16()
	if err := r.Err(); err != nil {
		return CrestUpload{}, fmt.Errorf("clientpackets: RequestExSetPledgeCrestLarge: %w", err)
	}
	if second != OpcodeRequestExSetPledgeCrestLarge {
		return CrestUpload{}, fmt.Errorf("clientpackets: RequestExSetPledgeCrestLarge: extended opcode %#x", second)
	}
	return readCrestUpload(r, "RequestExSetPledgeCrestLarge", LargePledgeCrestMaxLength)
}

// readCrestUpload reads a crest upload's declared length, then that many
// image bytes unless the length exceeds limit.
func readCrestUpload(r *wire.Reader, name string, limit int32) (CrestUpload, error) {
	u := CrestUpload{Length: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return CrestUpload{}, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	if u.Length > limit {
		return u, nil
	}
	if u.Length < 0 {
		return CrestUpload{}, fmt.Errorf("clientpackets: %s: %w %d", name, errNegativeCrestLength, u.Length)
	}
	u.Data = r.ReadBytes(int(u.Length))
	if err := r.Err(); err != nil {
		return CrestUpload{}, fmt.Errorf("clientpackets: %s: image of %d bytes: %w", name, u.Length, err)
	}
	return u, nil
}
