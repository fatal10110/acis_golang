package network

import (
	"errors"
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

const (
	clientReadHandshakeTimeout = time.Minute
	clientReadIdleTimeout      = 15 * time.Minute
)

// Session pairs a connection with the rolling cipher securing it. Encrypting
// a frame and queueing it for send must happen as one step in send order —
// mu is the only thing allowed to call cipher.Encrypt or conn.SendFrame, so two
// goroutines calling SendFrame concurrently can never queue frames in an order
// that disagrees with the order their bytes were encrypted in. Queueing never
// blocks, so mu is held only for one encrypt and one append.
type Session struct {
	conn   *Conn
	cipher *gamecipher.Cipher
	mu     sync.Mutex

	// cryptEnabled gates the rolling cipher: frames cross in cleartext
	// until the VersionCheck reply goes out: encryption starts only once the
	// key has been delivered. mu guards it
	// together with the encrypt path.
	cryptEnabled bool

	// frames reuses one payload buffer across inbound frames; it belongs to
	// the single goroutine calling ReadFrame.
	frames      *wire.FrameReader
	handshaking bool
	// idleTimeout, when set, supplies the established-session read
	// deadline in place of clientReadIdleTimeout (see readTimeout).
	idleTimeout func() time.Duration

	// evicted is closed, once, when Close evicts the session: a kick, an
	// account takeover or a duplicate selection. A player still lingering
	// after its connection ended then leaves at once (awaitDetachDelay).
	evicted   chan struct{}
	evictOnce sync.Once
}

// NewSession pairs conn with cipher for framed, encrypted read/write.
func NewSession(conn *Conn, cipher *gamecipher.Cipher) *Session {
	return &Session{conn: conn, cipher: cipher, frames: wire.NewFrameReader(conn), handshaking: true, evicted: make(chan struct{})}
}

// SendFrame encrypts and queues frame, which must already include the
// little-endian length header. It never blocks on the peer. It takes ownership
// of frame and releases it once the connection writer is done with it, or
// immediately when the connection is closed or aborted for a stalled peer.
func (s *Session) SendFrame(frame wire.Frame) bool {
	return s.send(frame, false)
}

// sendLast sends frame as the last packet the client receives: every later
// send is dropped, and the socket closes, frame flushed, once the connection
// is closed. A session already sealed or closed drops frame. It never blocks.
func (s *Session) sendLast(frame wire.Frame) {
	s.send(frame, true)
}

func (s *Session) send(frame wire.Frame, last bool) bool {
	frameBytes := frame.Bytes()
	if len(frameBytes) < frameHeaderSize {
		// An invalid frame carries no bytes; report its own cause.
		cause := frame.Err()
		if cause == nil {
			cause = wire.ShortFrameError(len(frameBytes))
		}
		frame.Release()
		s.conn.rejects.report("session send", cause)
		return false
	}
	if observe := s.conn.observeSend; observe != nil {
		observe(frameBytes[frameHeaderSize:])
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cryptEnabled {
		s.cipher.Encrypt(frameBytes[frameHeaderSize:])
	}
	if last {
		return s.conn.sendLast(frame)
	}
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
	if err := s.conn.SetReadDeadline(time.Now().Add(s.readTimeout())); err != nil {
		return nil, err
	}
	// The read observer runs once the deadline is fixed, so a watcher that
	// sees the wait begin knows which deadline it runs under.
	if observe := s.conn.observeRead; observe != nil {
		observe()
	}
	payload, err := s.frames.ReadFrame()
	if errors.Is(err, wire.ErrFrameTooShort) {
		// A length below the header's own size is a frame with no body:
		// its header is consumed and the next frame starts right after it.
		return payload[:0], nil
	}
	if err != nil {
		return nil, err
	}
	if s.cryptEnabled {
		s.cipher.Decrypt(payload)
	}
	return payload, nil
}

// readTimeout is how long the next read may wait: the handshake deadline
// until CompleteHandshake, then the idle deadline, which idleTimeout
// overrides with any value above zero.
func (s *Session) readTimeout() time.Duration {
	if s.handshaking {
		return clientReadHandshakeTimeout
	}
	if s.idleTimeout != nil {
		if d := s.idleTimeout(); d > 0 {
			return d
		}
	}
	return clientReadIdleTimeout
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

// Close sends the ServerClose packet as the connection's last and waits until
// the connection is closed. It is the server-initiated eviction path for a
// client whose session another selection took over; safe to call from any
// goroutine.
func (s *Session) Close() {
	s.evictOnce.Do(func() { close(s.evicted) })
	s.sendLast(serverpackets.FrameServerClose())
	_ = s.conn.Close()
}
