package wire

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// FrameHeaderSize is the length of a length-prefixed frame's own header,
// which itself counts toward the length it encodes.
const FrameHeaderSize = 2

// Frame is an outbound byte slice plus an optional release hook for pooled
// backing storage.
type Frame struct {
	bytes   []byte
	writer  *Writer
	release func(*Writer)
}

// BorrowedFrame returns a frame whose storage is not owned by a pool.
func BorrowedFrame(bytes []byte) Frame {
	return Frame{bytes: bytes}
}

// OwnedFrame returns a frame whose writer must be released after use.
func OwnedFrame(bytes []byte, writer *Writer, release func(*Writer)) Frame {
	return Frame{bytes: bytes, writer: writer, release: release}
}

// Bytes returns the frame bytes.
func (f Frame) Bytes() []byte {
	return f.bytes
}

// Release returns owned backing storage to its pool, if any.
func (f Frame) Release() {
	if f.release != nil {
		f.release(f.writer)
	}
}

// FrameBytes returns payload framed behind a little-endian uint16 length
// header (header included in the count), ready to write or encrypt in
// place.
func FrameBytes(payload []byte) []byte {
	frame := make([]byte, FrameHeaderSize+len(payload))
	binary.LittleEndian.PutUint16(frame, uint16(len(frame)))
	copy(frame[FrameHeaderSize:], payload)
	return frame
}

// ReadFrame reads one length-prefixed frame from r and returns its payload
// in a fresh allocation. The header is a little-endian uint16 giving the
// total frame length, header included; the payload is the header size
// shorter than that. Connection read loops should use a FrameReader
// instead, which reuses one payload buffer across frames.
func ReadFrame(r io.Reader) ([]byte, error) {
	return (&FrameReader{r: r}).ReadFrame()
}

// FrameReader reads length-prefixed frames from one reader, reusing a
// single payload buffer across calls so a connection's read loop costs no
// allocation per frame once the buffer has grown to the connection's usual
// frame size. Not safe for concurrent use.
type FrameReader struct {
	r io.Reader
	// header is read into a field rather than a local so the read through
	// the io.Reader interface can't force a per-call heap allocation.
	header [FrameHeaderSize]byte
	buf    []byte
}

// NewFrameReader returns a FrameReader reading frames from r.
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: bufio.NewReader(r)}
}

// ReadFrame reads one length-prefixed frame and returns its payload. The
// payload aliases the reader's internal buffer and is only valid until the
// next ReadFrame call; a caller that keeps it longer must copy it.
func (fr *FrameReader) ReadFrame() ([]byte, error) {
	if _, err := io.ReadFull(fr.r, fr.header[:]); err != nil {
		return nil, err
	}

	length := binary.LittleEndian.Uint16(fr.header[:])
	if length < FrameHeaderSize {
		return nil, fmt.Errorf("wire: frame length %d is smaller than the %d-byte header", length, FrameHeaderSize)
	}

	n := int(length) - FrameHeaderSize
	if cap(fr.buf) < n {
		fr.buf = make([]byte, n)
	}
	payload := fr.buf[:n]
	if _, err := io.ReadFull(fr.r, payload); err != nil {
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
