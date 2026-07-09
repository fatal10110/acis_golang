package network

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

type discardConn struct{}

func (discardConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (discardConn) Write(p []byte) (int, error)      { return len(p), nil }
func (discardConn) Close() error                     { return nil }
func (discardConn) LocalAddr() net.Addr              { return discardAddr("local") }
func (discardConn) RemoteAddr() net.Addr             { return discardAddr("remote") }
func (discardConn) SetDeadline(time.Time) error      { return nil }
func (discardConn) SetReadDeadline(time.Time) error  { return nil }
func (discardConn) SetWriteDeadline(time.Time) error { return nil }

type discardAddr string

func (a discardAddr) Network() string { return string(a) }
func (a discardAddr) String() string  { return string(a) }

func benchmarkSession(b *testing.B) *Session {
	b.Helper()
	cipher, err := NewCipher(bytes.Repeat([]byte{0x11}, keySize))
	if err != nil {
		b.Fatalf("NewCipher: %v", err)
	}
	s := NewSession(newConn(discardConn{}, nil), cipher)
	b.Cleanup(func() { s.conn.Close() })
	return s
}

func BenchmarkSessionSendPayload(b *testing.B) {
	s := benchmarkSession(b)

	b.ReportAllocs()
	for b.Loop() {
		if !s.Send(serverpackets.EncodeAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater)) {
			b.Fatal("Send returned false")
		}
	}
}

func BenchmarkSessionSendFrameWriter(b *testing.B) {
	s := benchmarkSession(b)

	b.ReportAllocs()
	for b.Loop() {
		if !s.SendFrame(serverpackets.FrameAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater)) {
			b.Fatal("SendFrame returned false")
		}
	}
}
