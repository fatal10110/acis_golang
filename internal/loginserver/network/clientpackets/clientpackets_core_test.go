package clientpackets

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"math/big"
	"testing"
)

// ---- from authgameguard_test.go ----
func TestDecodeAuthGameGuard(t *testing.T) {
	payload := make([]byte, 1+authGameGuardSize)
	payload[0] = OpcodeAuthGameGuard
	binary.LittleEndian.PutUint32(payload[1:], 0x11223344)
	binary.LittleEndian.PutUint32(payload[5:], 1)
	binary.LittleEndian.PutUint32(payload[9:], 2)
	binary.LittleEndian.PutUint32(payload[13:], 3)
	binary.LittleEndian.PutUint32(payload[17:], 4)

	got, err := DecodeAuthGameGuard(payload)
	if err != nil {
		t.Fatalf("DecodeAuthGameGuard: %v", err)
	}
	want := AuthGameGuard{SessionID: 0x11223344, Data1: 1, Data2: 2, Data3: 3, Data4: 4}
	if got != want {
		t.Errorf("DecodeAuthGameGuard = %+v, want %+v", got, want)
	}
}

func TestDecodeAuthGameGuardShort(t *testing.T) {
	if _, err := DecodeAuthGameGuard(make([]byte, 10)); err == nil {
		t.Error("DecodeAuthGameGuard: want error on short payload, got nil")
	}
}

// ---- from oracle_test.go ----
// These plaintext packets are fixed vectors from the Java readers in
// aCis_gameserver/java/net/sf/l2j/loginserver/network/clientpackets.
func TestJavaOracleClientPacketVectors(t *testing.T) {
	guard, err := DecodeAuthGameGuard([]byte{
		OpcodeAuthGameGuard, 0x44, 0x33, 0x22, 0x11, 0x88, 0x77, 0x66, 0x55,
		0xcc, 0xbb, 0xaa, 0x99, 0x00, 0xff, 0xee, 0xdd, 0x78, 0x56, 0x34, 0x12,
	})
	if err != nil || guard != (AuthGameGuard{0x11223344, 0x55667788, -0x66554434, -0x22110100, 0x12345678}) {
		t.Fatalf("DecodeAuthGameGuard() = %#v, %v", guard, err)
	}

	list, err := DecodeRequestServerList([]byte{OpcodeRequestServerList, 0x44, 0x33, 0x22, 0x11, 0xcc, 0xbb, 0xaa, 0x99})
	if err != nil || list != (RequestServerList{0x11223344, -0x66554434}) {
		t.Fatalf("DecodeRequestServerList() = %#v, %v", list, err)
	}

	login, err := DecodeRequestServerLogin([]byte{OpcodeRequestServerLogin, 0x44, 0x33, 0x22, 0x11, 0xcc, 0xbb, 0xaa, 0x99, 0x07})
	if err != nil || login != (RequestServerLogin{0x11223344, -0x66554434, 7}) {
		t.Fatalf("DecodeRequestServerLogin() = %#v, %v", login, err)
	}
}

// ---- from requestauthlogin_test.go ----
func TestDecodeRequestAuthLogin(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, credentialBlockSize*8)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	var block [credentialBlockSize]byte
	copy(block[usernameOffset:], "TestUser\x00\x00\x00\x00\x00\x00")
	copy(block[passwordOffset:], "s3cr3t   \x00\x00\x00\x00\x00\x00\x00")

	payload := append([]byte{OpcodeRequestAuthLogin}, encryptBlock(t, &key.PublicKey, block[:])...)

	got, err := DecodeRequestAuthLogin(payload, key)
	if err != nil {
		t.Fatalf("DecodeRequestAuthLogin: %v", err)
	}
	want := RequestAuthLogin{Username: "testuser", Password: "s3cr3t"}
	if got != want {
		t.Errorf("DecodeRequestAuthLogin = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestAuthLoginShort(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, credentialBlockSize*8)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if _, err := DecodeRequestAuthLogin(make([]byte, 10), key); err == nil {
		t.Error("DecodeRequestAuthLogin: want error on short payload, got nil")
	}
}

func TestTrimControlBytes(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"no padding", []byte("hello"), "hello"},
		{"null padded", []byte("hello\x00\x00\x00"), "hello"},
		{"space padded both ends", []byte("  hello  "), "hello"},
		{"all blank", []byte("\x00\x00 \x00"), ""},
		{"empty", []byte{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimControlBytes(tt.in); got != tt.want {
				t.Errorf("trimControlBytes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// encryptBlock RSA-encrypts a full-size block with no padding scheme,
// mirroring how the client encrypts the credential block: c = m^e mod n.
func encryptBlock(t *testing.T, pub *rsa.PublicKey, plaintext []byte) []byte {
	t.Helper()
	m := new(big.Int).SetBytes(plaintext)
	c := new(big.Int).Exp(m, big.NewInt(int64(pub.E)), pub.N)
	out := make([]byte, credentialBlockSize)
	c.FillBytes(out)
	return out
}

// ---- from requestserverlist_test.go ----
func TestDecodeRequestServerList(t *testing.T) {
	payload := make([]byte, 1+requestServerListSize)
	payload[0] = OpcodeRequestServerList
	binary.LittleEndian.PutUint32(payload[1:], 111)
	binary.LittleEndian.PutUint32(payload[5:], 222)

	got, err := DecodeRequestServerList(payload)
	if err != nil {
		t.Fatalf("DecodeRequestServerList: %v", err)
	}
	want := RequestServerList{SessionKey1: 111, SessionKey2: 222}
	if got != want {
		t.Errorf("DecodeRequestServerList = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestServerListShort(t *testing.T) {
	if _, err := DecodeRequestServerList(make([]byte, 4)); err == nil {
		t.Error("DecodeRequestServerList: want error on short payload, got nil")
	}
}

// ---- from requestserverlogin_test.go ----
func TestDecodeRequestServerLogin(t *testing.T) {
	payload := make([]byte, 1+requestServerLoginSize)
	payload[0] = OpcodeRequestServerLogin
	binary.LittleEndian.PutUint32(payload[1:], 111)
	binary.LittleEndian.PutUint32(payload[5:], 222)
	payload[9] = 3

	got, err := DecodeRequestServerLogin(payload)
	if err != nil {
		t.Fatalf("DecodeRequestServerLogin: %v", err)
	}
	want := RequestServerLogin{SessionKey1: 111, SessionKey2: 222, ServerID: 3}
	if got != want {
		t.Errorf("DecodeRequestServerLogin = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestServerLoginShort(t *testing.T) {
	if _, err := DecodeRequestServerLogin(make([]byte, 4)); err == nil {
		t.Error("DecodeRequestServerLogin: want error on short payload, got nil")
	}
}
