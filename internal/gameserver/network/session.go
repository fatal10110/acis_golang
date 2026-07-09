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
}

// NewSession pairs conn with cipher for framed, encrypted read/write.
func NewSession(conn *Conn, cipher *Cipher) *Session {
	return &Session{conn: conn, cipher: cipher}
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

// SendFrame encrypts and queues w's complete frame. If it returns true,
// ownership of w transfers to the connection writer until the write attempt
// finishes.
func (s *Session) SendFrame(w *wire.Writer) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	frame := w.Frame()
	s.cipher.Encrypt(frame[frameHeaderSize:])
	return s.conn.send(frame, w.Release)
}

// ReadFrame blocks for the next inbound frame, decrypts it, and returns its
// payload with the length header stripped. A network or EOF error from the
// underlying connection propagates as-is.
func (s *Session) ReadFrame() ([]byte, error) {
	payload, err := wire.ReadFrame(s.conn)
	if err != nil {
		return nil, err
	}
	s.cipher.Decrypt(payload)
	return payload, nil
}
