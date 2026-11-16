package lifecycle

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestRepeatedProtocolVersionRekeys sends ProtocolVersion three times on one
// connection. The first VersionCheck crosses in cleartext. Each later one
// carries a fresh key and is already encrypted with it, both rolling keys
// restarting from it: the next ProtocolVersion, encrypted with the new key
// from its start, is read as a supported revision and answered again.
func TestRepeatedProtocolVersionRekeys(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	conn := testsupport.Dial(t, srv.Addr()).Conn()

	sendProtocolVersion(t, conn, nil)
	first := readFrame(t, conn)
	key := versionCheckKey(t, first)
	assertVersionCheck(t, first, key)

	for range 2 {
		out := armedCipher(t, key)
		sendProtocolVersion(t, conn, out)
		raw := readFrame(t, conn)
		next := recoverVersionCheckKey(raw)
		if bytes.Equal(next, key) {
			t.Fatalf("repeated ProtocolVersion kept key %x", key)
		}
		armedCipher(t, next).Decrypt(raw)
		assertVersionCheck(t, raw, next)
		key = next
	}
}

// sendProtocolVersion writes ProtocolVersion, encrypted with out once the
// client holds a key.
func sendProtocolVersion(t *testing.T, conn net.Conn, out *gamecipher.Cipher) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
	w.WriteInt32(protocolRevision)
	payload := w.Bytes()
	if out != nil {
		out.Encrypt(payload)
	}
	if err := wire.WriteFrame(conn, payload); err != nil {
		t.Fatalf("write ProtocolVersion: %v", err)
	}
}

// readFrame reads the next frame's payload as it crossed the wire.
func readFrame(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := wire.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read VersionCheck: %v", err)
	}
	return payload
}

// armedCipher is the client's cipher for key, armed as VersionCheck arms it.
func armedCipher(t *testing.T, key []byte) *gamecipher.Cipher {
	t.Helper()
	c, err := gamecipher.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	c.Encrypt(nil)
	return c
}

// versionCheckKey is the full cipher key a cleartext VersionCheck hands the
// client: its random half, then the static half.
func versionCheckKey(t *testing.T, payload []byte) []byte {
	t.Helper()
	if len(payload) < 10 {
		t.Fatalf("VersionCheck %x too short", payload)
	}
	return append(append([]byte{}, payload[2:10]...), gamecipher.StaticKey[:]...)
}

// recoverVersionCheckKey reads the key off a VersionCheck encrypted with that
// same key. Each ciphertext byte is the plaintext byte XOR the key byte XOR
// the previous ciphertext byte, and the plaintext opens with the opcode, 0x01
// and the key's random half, so the half unrolls byte by byte; the
// decryption the caller checks then confirms it.
func recoverVersionCheckKey(c []byte) []byte {
	key := make([]byte, gamecipher.KeySize)
	if len(c) < 8 {
		return key
	}
	key[0] = c[0] ^ serverpackets.OpcodeVersionCheck
	key[1] = c[1] ^ c[0] ^ 0x01
	for i := 2; i < 8; i++ {
		key[i] = c[i] ^ c[i-1] ^ key[i-2]
	}
	copy(key[8:], gamecipher.StaticKey[:])
	return key
}

// assertVersionCheck requires payload to be the VersionCheck handing out
// key with the cipher on.
func assertVersionCheck(t *testing.T, payload, key []byte) {
	t.Helper()
	want := []byte{serverpackets.OpcodeVersionCheck, 0x01}
	want = append(want, key[:8]...)
	want = append(want, 1, 0, 0, 0, 1, 0, 0, 0)
	if !bytes.Equal(payload, want) {
		t.Fatalf("VersionCheck = %x, want %x", payload, want)
	}
}
