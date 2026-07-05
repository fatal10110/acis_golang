package link

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// newReader wraps payload for decoding, discarding the leading opcode byte
// every GS-LS link packet carries. A payload shorter than one byte leaves
// the reader's Err() set rather than panicking.
func newReader(payload []byte) *wire.Reader {
	r := wire.NewReader(payload)
	r.ReadUint8() // opcode
	return r
}
