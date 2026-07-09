package network

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func pipeSessions(t *testing.T) (server *Session, client net.Conn) {
	t.Helper()
	serverRaw, clientRaw := net.Pipe()
	t.Cleanup(func() { serverRaw.Close(); clientRaw.Close() })

	key := bytes.Repeat([]byte{0x11}, keySize)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return NewSession(newConn(serverRaw, nil), cipher), clientRaw
}

func TestSessionSendFramesWithLittleEndianLengthHeader(t *testing.T) {
	s, client := pipeSessions(t)

	if !s.Send([]byte{0xAA, 0xBB, 0xCC}) {
		t.Fatal("Send returned false")
	}

	frame := make([]byte, frameHeaderSize+3)
	if _, err := io.ReadFull(client, frame); err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if got := binary.LittleEndian.Uint16(frame); got != uint16(len(frame)) {
		t.Fatalf("length header = %d, want %d", got, len(frame))
	}
}

func TestSessionSendFrameWritesAndReleasesOwnedFrame(t *testing.T) {
	s, client := pipeSessions(t)

	released := make(chan struct{}, 1)
	frameBytes := []byte{0x05, 0x00, 0xAA, 0xBB, 0xCC}
	frame := wire.OwnedFrame(frameBytes, nil, func(*wire.Writer) { released <- struct{}{} })
	if !s.SendFrame(frame) {
		t.Fatal("SendFrame returned false")
	}

	got := make([]byte, len(frameBytes))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if !bytes.Equal(got, frameBytes) {
		t.Fatalf("frame = % X, want % X", got, frameBytes)
	}
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("owned frame was not released after write")
	}
}

func TestSessionSendArmsCipherSoFirstPacketIsCleartext(t *testing.T) {
	s, client := pipeSessions(t)

	payload := []byte{0x01, 0x02, 0x03, 0x04}
	if !s.Send(payload) {
		t.Fatal("Send returned false")
	}

	frame := make([]byte, frameHeaderSize+len(payload))
	if _, err := io.ReadFull(client, frame); err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if !bytes.Equal(frame[frameHeaderSize:], payload) {
		t.Fatalf("first frame payload = % X, want cleartext % X", frame[frameHeaderSize:], payload)
	}
}

func TestSessionReadFrameDecryptsAfterCipherArmed(t *testing.T) {
	s, client := pipeSessions(t)

	// Arm the session's cipher exactly like the real handshake does: the
	// server's first outbound packet flips encryption on for both
	// directions without transforming that packet's own bytes.
	if !s.Send([]byte{0x00}) {
		t.Fatal("Send returned false")
	}
	armFrame := make([]byte, frameHeaderSize+1)
	if _, err := io.ReadFull(client, armFrame); err != nil {
		t.Fatalf("read arming frame: %v", err)
	}

	clientCipher, err := NewCipher(bytes.Repeat([]byte{0x11}, keySize))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	clientCipher.Encrypt(make([]byte, 0)) // arm the client-side mirror the same way

	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	encrypted := append([]byte(nil), payload...)
	clientCipher.Encrypt(encrypted)

	frame := make([]byte, frameHeaderSize+len(encrypted))
	binary.LittleEndian.PutUint16(frame, uint16(len(frame)))
	copy(frame[frameHeaderSize:], encrypted)

	go client.Write(frame)

	got, err := s.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("ReadFrame() = % X, want % X", got, payload)
	}
}

func TestSessionReadFrameRejectsHeaderShorterThanItself(t *testing.T) {
	s, client := pipeSessions(t)

	var header [frameHeaderSize]byte
	binary.LittleEndian.PutUint16(header[:], 1) // claims a total size smaller than the header itself
	go client.Write(header[:])

	if _, err := s.ReadFrame(); err == nil {
		t.Fatal("ReadFrame() err = nil, want an error for a length shorter than the header")
	}
}

func TestSessionSendSerializesConcurrentCallers(t *testing.T) {
	s, client := pipeSessions(t)

	// Session.Send arms the shared cipher on the first frame (sent
	// cleartext, per the cipher's first-packet rule) and rolls it on every
	// frame after; mirror decrypts each frame in receipt order to recover
	// the original byte, proving send order matched encrypt order (a
	// corrupted interleaving would fail to decrypt cleanly).
	mirror, err := NewCipher(bytes.Repeat([]byte{0x11}, keySize))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	const senders = 20
	var wg sync.WaitGroup
	wg.Add(senders)
	for i := 0; i < senders; i++ {
		go func(i int) {
			defer wg.Done()
			s.Send([]byte{byte(i)})
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	seen := make(map[byte]bool)
	for i := 0; i < senders; i++ {
		header := make([]byte, frameHeaderSize)
		if _, err := io.ReadFull(client, header); err != nil {
			t.Fatalf("read header %d: %v", i, err)
		}
		size := binary.LittleEndian.Uint16(header)
		payload := make([]byte, int(size)-frameHeaderSize)
		if _, err := io.ReadFull(client, payload); err != nil {
			t.Fatalf("read payload %d: %v", i, err)
		}
		if len(payload) != 1 {
			t.Fatalf("payload %d length = %d, want 1", i, len(payload))
		}
		if i == 0 {
			// The arm frame: Session.Send's first call leaves it cleartext,
			// so there is nothing to decrypt.
			mirror.enabled = true
		} else {
			mirror.Decrypt(payload)
		}
		seen[payload[0]] = true
	}
	<-done
	if len(seen) != senders {
		t.Fatalf("saw %d distinct payloads, want %d (frames corrupted by interleaving)", len(seen), senders)
	}
}
