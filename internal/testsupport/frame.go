package testsupport

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// ReadFrame reads one length-prefixed frame from r and returns its payload
// in a fresh allocation. It reads no byte past the frame, so a test may call
// it repeatedly on one connection.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [wire.FrameHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint16(header[:])
	if length < wire.FrameHeaderSize {
		return nil, fmt.Errorf("%w: %d < %d", wire.ErrFrameTooShort, length, wire.FrameHeaderSize)
	}
	payload := make([]byte, int(length)-wire.FrameHeaderSize)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
