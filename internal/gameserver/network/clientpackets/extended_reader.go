package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func newExtendedReader(payload []byte, name string, opcode uint16, size int) (*wire.Reader, error) {
	r := newReader(payload)
	if r.Remaining() < size {
		return nil, fmt.Errorf("clientpackets: %s: need %d bytes, got %d", name, size, r.Remaining())
	}
	if second := r.ReadUint16(); second != opcode {
		return nil, fmt.Errorf("clientpackets: %s: extended opcode %#x", name, second)
	}
	return r, nil
}
