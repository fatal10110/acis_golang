package link

import (
	"bytes"
	"testing"
)

func TestEncodeLoginServerFail(t *testing.T) {
	got := EncodeLoginServerFail(ReasonAlreadyLoggedIn)
	want := []byte{OpcodeLoginServerFail, byte(ReasonAlreadyLoggedIn)}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeLoginServerFail() = %x, want %x", got, want)
	}
}

func TestDecodeLoginServerFail(t *testing.T) {
	got, err := DecodeLoginServerFail(EncodeLoginServerFail(ReasonWrongHexID))
	if err != nil {
		t.Fatalf("DecodeLoginServerFail: %v", err)
	}
	if got != ReasonWrongHexID {
		t.Fatalf("DecodeLoginServerFail() = %v, want %v", got, ReasonWrongHexID)
	}
}

func TestDecodeLoginServerFailShort(t *testing.T) {
	if _, err := DecodeLoginServerFail([]byte{OpcodeLoginServerFail}); err == nil {
		t.Error("DecodeLoginServerFail: want error on short payload, got nil")
	}
}

func TestLoginServerFailReasonString(t *testing.T) {
	tests := map[LoginServerFailReason]string{
		ReasonIPBanned:        "ip banned",
		ReasonIPReserved:      "ip reserved",
		ReasonWrongHexID:      "wrong hexid",
		ReasonIDReserved:      "id reserved",
		ReasonNoFreeID:        "no free ID",
		ReasonNotAuthed:       "not authed",
		ReasonAlreadyLoggedIn: "already logged in",
	}
	for reason, want := range tests {
		if got := reason.String(); got != want {
			t.Errorf("%v.String() = %q, want %q", reason, got, want)
		}
	}
	if got := LoginServerFailReason(0).String(); got == "" {
		t.Error("LoginServerFailReason(0).String() = empty, want a fallback description")
	}
}
