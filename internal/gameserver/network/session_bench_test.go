package network

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/rs/zerolog"
)

type discardConn struct{}

func (discardConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (discardConn) Write(p []byte) (int, error)      { return len(p), nil }
func (discardConn) Close() error                     { return nil }
func (discardConn) LocalAddr() net.Addr              { return discardAddr{} }
func (discardConn) RemoteAddr() net.Addr             { return discardAddr{} }
func (discardConn) SetDeadline(time.Time) error      { return nil }
func (discardConn) SetReadDeadline(time.Time) error  { return nil }
func (discardConn) SetWriteDeadline(time.Time) error { return nil }

type discardAddr struct{}

func (discardAddr) Network() string { return "discard" }
func (discardAddr) String() string  { return "discard" }

func benchmarkSession(b *testing.B) *Session {
	b.Helper()
	cipher, err := NewCipher(make([]byte, keySize))
	if err != nil {
		b.Fatalf("NewCipher: %v", err)
	}
	conn := newConn(discardConn{}, zerolog.Nop())
	b.Cleanup(func() { conn.Close() })
	return NewSession(conn, cipher)
}

func BenchmarkSessionSendAuthLoginFailFrame(b *testing.B) {
	session := benchmarkSession(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		frame := serverpackets.FrameAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater)
		if !session.SendFrame(frame) {
			b.Fatal("SendFrame returned false")
		}
	}
}

func benchmarkUserInfoSnapshot() serverpackets.UserInfoSnapshot {
	return serverpackets.UserInfoSnapshot{
		Character: &player.Character{Name: "Benchmark"},
		Template:  &player.Template{},
	}
}

func benchmarkUserInfoPayload(s serverpackets.UserInfoSnapshot) []byte {
	return serverpackets.EncodeUserInfo(s)
}

func TestBenchmarkUserInfoPayloadMatchesFrame(t *testing.T) {
	snapshot := benchmarkUserInfoSnapshot()
	frame := serverpackets.FrameUserInfo(snapshot)
	defer frame.Release()

	if got, want := benchmarkUserInfoPayload(snapshot), frame.Bytes()[frameHeaderSize:]; !bytes.Equal(got, want) {
		t.Fatal("unpooled UserInfo payload differs from FrameUserInfo")
	}
}

func BenchmarkSessionSendUserInfoFrame(b *testing.B) {
	snapshot := benchmarkUserInfoSnapshot()
	session := benchmarkSession(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if !session.SendFrame(serverpackets.FrameUserInfo(snapshot)) {
			b.Fatal("SendFrame returned false")
		}
	}
}
