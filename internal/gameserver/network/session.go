package network

import (
	"runtime"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// frameHeaderSize is the length of a frame's little-endian size prefix,
// which itself counts toward the length it encodes. Because the prefix is
// a uint16, a frame can never exceed 65535 bytes — the wire format itself
// bounds the allocation ReadFrame makes for a frame's payload.
const frameHeaderSize = wire.FrameHeaderSize

// trySendLockAttempts lets brief healthy contention clear before the blocking
// fallback; each retry checks for saturation and aborts a slow peer instead.
const trySendLockAttempts = 64

const (
	clientReadHandshakeTimeout = time.Minute
	clientReadIdleTimeout      = 15 * time.Minute
)

// Session pairs a connection with the rolling cipher securing it. Encrypting
// a frame and queueing it for send must happen as one step in send order —
// mu is the only thing allowed to call cipher.Encrypt or conn.SendFrame, so two
// goroutines calling SendFrame concurrently can never queue frames in an order
// that disagrees with the order their bytes were encrypted in. SendFrame is
// the only path that can hold mu while blocking, and it can do so only after
// the outbound queue is full; TrySendFrame checks that condition before it
// waits for mu and aborts the connection instead.
type Session struct {
	conn   *Conn
	cipher *gamecipher.Cipher
	mu     sync.Mutex

	// cryptEnabled gates the rolling cipher: frames cross in cleartext
	// until the VersionCheck reply goes out, matching the reference where
	// encryption starts only once the key has been delivered. mu guards it
	// together with the encrypt path.
	cryptEnabled bool

	// frames reuses one payload buffer across inbound frames; it belongs to
	// the single goroutine calling ReadFrame.
	frames      *wire.FrameReader
	handshaking bool
}

// NewSession pairs conn with cipher for framed, encrypted read/write.
func NewSession(conn *Conn, cipher *gamecipher.Cipher) *Session {
	return &Session{conn: conn, cipher: cipher, frames: wire.NewFrameReader(conn), handshaking: true}
}

// SendFrame encrypts and queues frame, which must already include the
// little-endian length header. It takes ownership of frame and releases it
// once the connection writer is done with it.
func (s *Session) SendFrame(frame wire.Frame) bool {
	return s.sendFrame(frame, s.conn.SendFrame)
}

// TrySendFrame encrypts and queues frame only when the connection's outbound
// queue has capacity. A full queue disconnects the client before encryption,
// because dropping an ordered frame would desynchronize its cipher. It takes
// ownership of frame in every outcome.
func (s *Session) TrySendFrame(frame wire.Frame) bool {
	return s.trySendFrame(frame)
}

func (s *Session) trySendFrame(frame wire.Frame) bool {
	frameBytes := frame.Bytes()
	if len(frameBytes) < frameHeaderSize {
		frame.Release()
		return false
	}

	if s.conn.queueFull() {
		frame.Release()
		s.conn.abort()
		return false
	}
	for range trySendLockAttempts {
		if s.mu.TryLock() {
			defer s.mu.Unlock()
			return s.encryptAndTrySend(frame, frameBytes)
		}
		if s.conn.queueFull() {
			frame.Release()
			s.conn.abort()
			return false
		}
		runtime.Gosched()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.encryptAndTrySend(frame, frameBytes)
}

// encryptAndTrySend runs with s.mu held.
func (s *Session) encryptAndTrySend(frame wire.Frame, frameBytes []byte) bool {
	if s.conn.queueFull() {
		frame.Release()
		s.conn.abort()
		return false
	}
	if s.cryptEnabled {
		s.cipher.Encrypt(frameBytes[frameHeaderSize:])
	}
	return s.conn.trySendFrame(frame)
}

func (s *Session) sendFrame(frame wire.Frame, send func(wire.Frame) bool) bool {
	frameBytes := frame.Bytes()
	if len(frameBytes) < frameHeaderSize {
		frame.Release()
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cryptEnabled {
		s.cipher.Encrypt(frameBytes[frameHeaderSize:])
	}
	return send(frame)
}

// ReadFrame blocks for the next inbound frame, decrypts it, and returns its
// payload with the length header stripped. A network or EOF error from the
// underlying connection propagates as-is.
//
// The payload reuses a per-session buffer and is only valid until the next
// ReadFrame call: decode it before reading again, or copy it. Only one
// goroutine may call ReadFrame.
func (s *Session) ReadFrame() ([]byte, error) {
	timeout := clientReadIdleTimeout
	if s.handshaking {
		timeout = clientReadHandshakeTimeout
	}
	if err := s.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	payload, err := s.frames.ReadFrame()
	if err != nil {
		return nil, err
	}
	if s.cryptEnabled {
		s.cipher.Decrypt(payload)
	}
	return payload, nil
}

// CompleteHandshake switches later reads to the established-session idle
// deadline. It is called by the client loop only after AuthLogin succeeds.
func (s *Session) CompleteHandshake() {
	s.handshaking = false
}

// EnableCrypt turns the rolling cipher on for every later frame. It must be
// called right after the VersionCheck reply — the frame that carries the
// key — has been queued: arming here (a no-transform Encrypt) keeps the
// first encrypted outbound frame transformed exactly as the client expects.
func (s *Session) EnableCrypt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cryptEnabled = true
	s.cipher.Encrypt(nil)
}

// Close sends the ServerClose packet and closes the connection once every
// queued frame — that packet included — has been written. It is the
// server-initiated eviction path for a client whose session another
// selection took over; safe to call from any goroutine.
func (s *Session) Close() {
	s.SendFrame(serverpackets.FrameServerClose())
	_ = s.conn.Close()
}
