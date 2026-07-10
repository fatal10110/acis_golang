package network

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
	conn := newConn(discardConn{}, nil)
	b.Cleanup(func() { conn.Close() })
	return NewSession(conn, cipher)
}

func BenchmarkSessionSendAuthLoginFailPayload(b *testing.B) {
	session := benchmarkSession(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if !session.Send(serverpackets.EncodeAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater)) {
			b.Fatal("Send returned false")
		}
	}
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
	w := wire.NewPacketWriter(serverpackets.OpcodeUserInfo)
	writeZeroInt32s(w, 5)
	w.WriteString(s.Character.Name)
	writeZeroInt32s(w, 4)
	w.WriteInt64(0)
	writeZeroInt32s(w, 13)
	w.WriteInt32(20)
	writeZeroInt32s(w, item.PaperdollSlots*2)
	writeZeroUint16s(w, 14)
	w.WriteInt32(0)
	writeZeroUint16s(w, 12)
	w.WriteInt32(0)
	writeZeroUint16s(w, 4)
	writeZeroInt32s(w, 12)
	writeZeroInt32s(w, 8)
	w.WriteFloat32(1)
	w.WriteFloat32(1)
	w.WriteFloat32(0)
	w.WriteFloat32(0)
	writeZeroInt32s(w, 4)
	w.WriteString(s.Character.Title)
	writeZeroInt32s(w, 5)
	writeZeroUint8s(w, 3)
	writeZeroInt32s(w, 2)
	w.WriteUint16(0)
	w.WriteUint8(0)
	w.WriteInt32(0)
	w.WriteUint8(0)
	w.WriteInt32(0)
	writeZeroUint16s(w, 2)
	w.WriteInt32(0)
	w.WriteUint16(80)
	writeZeroInt32s(w, 4)
	writeZeroUint8s(w, 2)
	w.WriteInt32(0)
	writeZeroUint8s(w, 3)
	writeZeroInt32s(w, 3)
	w.WriteInt32(0xFFFFFF)
	w.WriteUint8(1)
	writeZeroInt32s(w, 2)
	w.WriteInt32(0xFFFF77)
	w.WriteInt32(0)
	return w.Bytes()
}

func writeZeroInt32s(w *wire.Writer, n int) {
	for range n {
		w.WriteInt32(0)
	}
}

func writeZeroUint16s(w *wire.Writer, n int) {
	for range n {
		w.WriteUint16(0)
	}
}

func writeZeroUint8s(w *wire.Writer, n int) {
	for range n {
		w.WriteUint8(0)
	}
}

func benchmarkUserInfoPayloadMatchesFrame(b *testing.B, s serverpackets.UserInfoSnapshot) {
	b.Helper()
	frame := serverpackets.FrameUserInfo(s)
	defer frame.Release()
	want := frame.Bytes()[frameHeaderSize:]
	if got := benchmarkUserInfoPayload(s); !bytes.Equal(got, want) {
		b.Fatal("unpooled UserInfo payload differs from FrameUserInfo")
	}
}

func BenchmarkSessionSendUserInfoPayload(b *testing.B) {
	snapshot := benchmarkUserInfoSnapshot()
	benchmarkUserInfoPayloadMatchesFrame(b, snapshot)
	session := benchmarkSession(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if !session.Send(benchmarkUserInfoPayload(snapshot)) {
			b.Fatal("Send returned false")
		}
	}
}

func BenchmarkSessionSendUserInfoFrame(b *testing.B) {
	snapshot := benchmarkUserInfoSnapshot()
	benchmarkUserInfoPayloadMatchesFrame(b, snapshot)
	session := benchmarkSession(b)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if !session.SendFrame(serverpackets.FrameUserInfo(snapshot)) {
			b.Fatal("SendFrame returned false")
		}
	}
}
