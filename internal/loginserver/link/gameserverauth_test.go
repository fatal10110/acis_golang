package link

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDecodeGameServerAuth(t *testing.T) {
	hexID := []byte{0xde, 0xad, 0xbe, 0xef}

	payload := []byte{OpcodeGameServerAuth, 3, 1, 0}
	payload = appendString(payload, "gs.example.com")
	payload = binary.LittleEndian.AppendUint16(payload, 7777)
	payload = binary.LittleEndian.AppendUint32(payload, 500)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(hexID)))
	payload = append(payload, hexID...)

	got, err := DecodeGameServerAuth(payload)
	if err != nil {
		t.Fatalf("DecodeGameServerAuth: %v", err)
	}
	want := GameServerAuth{
		DesiredID:         3,
		AcceptAlternateID: true,
		HostReserved:      false,
		HostName:          "gs.example.com",
		Port:              7777,
		MaxPlayers:        500,
		HexID:             hexID,
	}
	if got.DesiredID != want.DesiredID || got.AcceptAlternateID != want.AcceptAlternateID ||
		got.HostReserved != want.HostReserved || got.HostName != want.HostName ||
		got.Port != want.Port || got.MaxPlayers != want.MaxPlayers || !bytes.Equal(got.HexID, want.HexID) {
		t.Fatalf("DecodeGameServerAuth() = %+v, want %+v", got, want)
	}
}

func TestDecodeGameServerAuthShort(t *testing.T) {
	if _, err := DecodeGameServerAuth([]byte{OpcodeGameServerAuth, 1}); err == nil {
		t.Error("DecodeGameServerAuth: want error on short payload, got nil")
	}
}

func TestEncodeGameServerAuthRoundTrip(t *testing.T) {
	want := GameServerAuth{
		DesiredID:         3,
		AcceptAlternateID: true,
		HostReserved:      false,
		HostName:          "gs.example.com",
		Port:              7777,
		MaxPlayers:        500,
		HexID:             []byte{0xde, 0xad, 0xbe, 0xef},
	}

	got, err := DecodeGameServerAuth(EncodeGameServerAuth(want))
	if err != nil {
		t.Fatalf("DecodeGameServerAuth(EncodeGameServerAuth()): %v", err)
	}
	if got.DesiredID != want.DesiredID || got.AcceptAlternateID != want.AcceptAlternateID ||
		got.HostReserved != want.HostReserved || got.HostName != want.HostName ||
		got.Port != want.Port || got.MaxPlayers != want.MaxPlayers || !bytes.Equal(got.HexID, want.HexID) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}
