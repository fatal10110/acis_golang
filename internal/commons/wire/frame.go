package wire

import (
	"encoding/binary"
	"fmt"
	"io"
)

// FrameHeaderSize is the length of a length-prefixed frame's own header,
// which itself counts toward the length it encodes.
const FrameHeaderSize = 2

// FrameBytes returns payload framed behind a little-endian uint16 length
// header (header included in the count), ready to write or encrypt in
// place.
func FrameBytes(payload []byte) []byte {
	frame := make([]byte, FrameHeaderSize+len(payload))
	binary.LittleEndian.PutUint16(frame, uint16(len(frame)))
	copy(frame[FrameHeaderSize:], payload)
	return frame
}

// ReadFrame reads one length-prefixed frame from r and returns its payload.
// The header is a little-endian uint16 giving the total frame length,
// header included; the payload is the header size shorter than that.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [FrameHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	length := binary.LittleEndian.Uint16(header[:])
	if length < FrameHeaderSize {
		return nil, fmt.Errorf("wire: frame length %d is smaller than the %d-byte header", length, FrameHeaderSize)
	}

	payload := make([]byte, int(length)-FrameHeaderSize)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// WriteFrame writes payload to w as one length-prefixed frame: a
// little-endian uint16 header giving the total frame length (header
// included), followed by payload itself.
func WriteFrame(w io.Writer, payload []byte) error {
	_, err := w.Write(FrameBytes(payload))
	return err
}
