package clientpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func encodeAuthLoginPayload(loginName string, playKey2, playKey1, loginKey1, loginKey2 int32) []byte {
	var w wire.Writer
	w.WriteUint8(OpcodeAuthLogin)
	w.WriteString(loginName)
	w.WriteInt32(playKey2)
	w.WriteInt32(playKey1)
	w.WriteInt32(loginKey1)
	w.WriteInt32(loginKey2)
	return w.Bytes()
}

func TestDecodeAuthLogin(t *testing.T) {
	// Distinct values in every field catch a decoder that mixes up the
	// play/login pairs or their halves.
	payload := encodeAuthLoginPayload("Player1", 11, 22, 33, 44)

	got, err := DecodeAuthLogin(payload)
	if err != nil {
		t.Fatalf("DecodeAuthLogin: %v", err)
	}
	want := AuthLogin{LoginName: "player1", PlayKey1: 22, PlayKey2: 11, LoginKey1: 33, LoginKey2: 44}
	if got != want {
		t.Fatalf("DecodeAuthLogin() = %+v, want %+v", got, want)
	}
}

func TestDecodeAuthLoginLowerCasesAccountName(t *testing.T) {
	payload := encodeAuthLoginPayload("MiXeDCaSe", 1, 2, 3, 4)

	got, err := DecodeAuthLogin(payload)
	if err != nil {
		t.Fatalf("DecodeAuthLogin: %v", err)
	}
	if got.LoginName != "mixedcase" {
		t.Fatalf("LoginName = %q, want %q", got.LoginName, "mixedcase")
	}
}

func TestDecodeAuthLoginShortPayload(t *testing.T) {
	var w wire.Writer
	w.WriteUint8(OpcodeAuthLogin)
	w.WriteString("player1")
	w.WriteInt32(1) // only one of the four required ints

	if _, err := DecodeAuthLogin(w.Bytes()); err == nil {
		t.Fatal("DecodeAuthLogin: want error on short payload, got nil")
	}
}
