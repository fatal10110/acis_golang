package network

import (
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// --- fake game client, driving the wire protocol from the other side ---
//
// A real game client speaks first: it sends ProtocolVersion cleartext,
// receives VersionCheck cleartext, then arms the rolling XOR cipher from
// VersionCheck's 8 random bytes plus the fixed static key half.

type fakeGameClient struct {
	t          *testing.T
	conn       net.Conn
	handshaken bool
	cipher     *Cipher
}

func dialGameClient(t *testing.T, addr string) *fakeGameClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })

	return &fakeGameClient{t: t, conn: conn}
}

// readWithTimeout reads one frame within d, returning nil on timeout instead
// of failing the test. Used by TestGameClientLinkNeverGoesSilentOnActionRequests
// to drain however many frames a rejected request produces (a system message
// plus ActionFailed, or ActionFailed alone) without hard-coding an exact
// count, while still treating "nothing at all" as a failure.
func (f *fakeGameClient) readWithTimeout(d time.Duration) []byte {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(d))
	payload, err := wire.ReadFrame(f.conn)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil
		}
		f.t.Fatalf("ReadFrame: %v", err)
	}
	if f.cipher != nil {
		f.cipher.Decrypt(payload)
	}
	return payload
}

func (f *fakeGameClient) sendProtocolVersion(revision int32) {
	f.t.Helper()
	if err := wire.WriteFrame(f.conn, encodeProtocolVersion(revision)); err != nil {
		f.t.Fatalf("write ProtocolVersion: %v", err)
	}

	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	raw, err := wire.ReadFrame(f.conn)
	if err != nil {
		f.t.Fatalf("read VersionCheck: %v", err)
	}
	if raw[0] != serverpackets.OpcodeVersionCheck {
		f.t.Fatalf("first packet opcode = %#x, want VersionCheck (%#x)", raw[0], serverpackets.OpcodeVersionCheck)
	}
	if len(raw) != 18 {
		f.t.Fatalf("VersionCheck payload size = %d, want 18", len(raw))
	}
	if enabled := wire.NewReader(raw[10:14]).ReadInt32(); enabled != 0 {
		key := make([]byte, keySize)
		copy(key[:8], raw[2:10])
		copy(key[8:], gameCipherStaticKey[:])

		cipher, err := NewCipher(key)
		if err != nil {
			f.t.Fatalf("NewCipher: %v", err)
		}
		cipher.Encrypt(nil)
		f.cipher = cipher
	}
	f.handshaken = true
}

func (f *fakeGameClient) send(payload []byte) {
	f.t.Helper()
	if !f.handshaken {
		f.t.Fatal("send called before ProtocolVersion/VersionCheck handshake")
	}
	buf := append([]byte(nil), payload...)
	if f.cipher != nil {
		f.cipher.Encrypt(buf)
	}
	if err := wire.WriteFrame(f.conn, buf); err != nil {
		f.t.Fatalf("WriteFrame: %v", err)
	}
}

func (f *fakeGameClient) read() []byte {
	f.t.Helper()
	if !f.handshaken {
		f.t.Fatal("read called before ProtocolVersion/VersionCheck handshake")
	}
	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := wire.ReadFrame(f.conn)
	if err != nil {
		f.t.Fatalf("ReadFrame: %v", err)
	}
	if f.cipher != nil {
		f.cipher.Decrypt(payload)
	}
	return payload
}

func (f *fakeGameClient) expectNoFrame() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if payload, err := wire.ReadFrame(f.conn); err == nil {
		if f.cipher != nil {
			f.cipher.Decrypt(payload)
		}
		f.t.Fatalf("unexpected frame: %x", payload)
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		f.t.Fatalf("ReadFrame: %v", err)
	}
}

func readEnterWorldBurst(t *testing.T, c *fakeGameClient, wantDie bool) [][]byte {
	t.Helper()
	want := []byte{
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
	}
	if wantDie {
		want = append(want, serverpackets.OpcodeDie)
	}
	want = append(want, serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)

	frames := make([][]byte, 0, len(want))
	for i, opcode := range want {
		frame := c.read()
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
		if i == 0 {
			if second := wire.NewReader(frame[1:]).ReadUint16(); second != serverpackets.OpcodeExStorageMaxCount {
				t.Fatalf("EnterWorld first extended opcode = %#x, want ExStorageMaxCount (%#x)", second, serverpackets.OpcodeExStorageMaxCount)
			}
		}
		frames = append(frames, frame)
	}
	return frames
}

func (f *fakeGameClient) expectClosed() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if n, err := f.conn.Read(buf); n != 0 || err == nil {
		f.t.Fatalf("expected connection to close, got n=%d err=%v", n, err)
	}
}
