package clientpackets

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func encodeUTF16Z(s string) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}

func TestDecodeRequestCharacterCreate(t *testing.T) {
	var payload []byte
	payload = append(payload, OpcodeRequestCharacterCreate)
	payload = append(payload, encodeUTF16Z("Newbie")...)
	payload = binary.LittleEndian.AppendUint32(payload, 0) // race
	payload = binary.LittleEndian.AppendUint32(payload, 1) // sex
	payload = binary.LittleEndian.AppendUint32(payload, 0) // classId
	for i := 0; i < 6; i++ {
		payload = binary.LittleEndian.AppendUint32(payload, 999) // ignored stat fields
	}
	payload = binary.LittleEndian.AppendUint32(payload, 2) // hairStyle
	payload = binary.LittleEndian.AppendUint32(payload, 3) // hairColor
	payload = binary.LittleEndian.AppendUint32(payload, 1) // face

	got, err := DecodeRequestCharacterCreate(payload)
	if err != nil {
		t.Fatalf("DecodeRequestCharacterCreate: %v", err)
	}
	want := RequestCharacterCreate{
		Name: "Newbie", Race: 0, Sex: 1, ClassID: 0,
		HairStyle: 2, HairColor: 3, Face: 1,
	}
	if got != want {
		t.Errorf("DecodeRequestCharacterCreate = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestCharacterCreate_Short(t *testing.T) {
	if _, err := DecodeRequestCharacterCreate([]byte{OpcodeRequestCharacterCreate}); err == nil {
		t.Error("DecodeRequestCharacterCreate: want error on short payload, got nil")
	}
}
