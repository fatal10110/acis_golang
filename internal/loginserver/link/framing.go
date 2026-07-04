package link

import (
	"encoding/binary"
	"fmt"
	"io"
)

// headerSize is the length of a link frame's own length header.
const headerSize = 2

// ReadFrame reads one length-prefixed frame from r and returns its payload.
// The frame header is a little-endian uint16 giving the total frame length,
// header included; the payload is the header size shorter than that.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	length := binary.LittleEndian.Uint16(header[:])
	if length < headerSize {
		return nil, fmt.Errorf("link frame length %d is smaller than the %d-byte header", length, headerSize)
	}

	payload := make([]byte, int(length)-headerSize)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// WriteFrame writes payload to w as one length-prefixed frame: a
// little-endian uint16 header giving the total frame length (header
// included), followed by payload itself.
func WriteFrame(w io.Writer, payload []byte) error {
	frame := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint16(frame, uint16(headerSize+len(payload)))
	copy(frame[headerSize:], payload)
	_, err := w.Write(frame)
	return err
}
