package network

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// frameHeaderSize is the length of a frame's little-endian size prefix,
// which itself counts toward the length it encodes. Because the prefix is
// a uint16, a frame can never exceed 65535 bytes — the wire format itself
// bounds the allocation ReadFrame makes for a frame's payload.
const frameHeaderSize = wire.FrameHeaderSize

// Session pairs a connection with the rolling cipher securing it. Encrypting
// a frame and queueing it for send must happen as one step in send order —
// mu is the only thing allowed to call cipher.Encrypt or conn.Send, so two
// goroutines calling Send concurrently can never queue frames in an order
// that disagrees with the order their bytes were encrypted in.
type Session struct {
	conn   *Conn
	cipher *Cipher
	mu     sync.Mutex

	// frames reuses one payload buffer across inbound frames; it belongs to
	// the single goroutine calling ReadFrame.
	frames *wire.FrameReader
}

// NewSession pairs conn with cipher for framed, encrypted read/write.
func NewSession(conn *Conn, cipher *Cipher) *Session {
	return &Session{conn: conn, cipher: cipher, frames: wire.NewFrameReader(conn)}
}

// Send frames payload behind a little-endian length header (header included
// in the count), encrypts it, and queues it on the connection's writer
// goroutine. It returns false if the connection is already closed.
func (s *Session) Send(payload []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	frame := wire.FrameBytes(payload)
	s.cipher.Encrypt(frame[frameHeaderSize:])
	return s.conn.Send(frame)
}

// SendFrame encrypts and queues frame, which must already include the
// little-endian length header. It takes ownership of frame and releases it
// once the connection writer is done with it.
func (s *Session) SendFrame(frame wire.Frame) bool {
	frameBytes := frame.Bytes()
	if len(frameBytes) < frameHeaderSize {
		frame.Release()
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cipher.Encrypt(frameBytes[frameHeaderSize:])
	return s.conn.SendFrame(frame)
}

// ReadFrame blocks for the next inbound frame, decrypts it, and returns its
// payload with the length header stripped. A network or EOF error from the
// underlying connection propagates as-is.
//
// The payload reuses a per-session buffer and is only valid until the next
// ReadFrame call: decode it before reading again, or copy it. Only one
// goroutine may call ReadFrame.
func (s *Session) ReadFrame() ([]byte, error) {
	payload, err := s.frames.ReadFrame()
	if err != nil {
		return nil, err
	}
	s.cipher.Decrypt(payload)
	return payload, nil
}
