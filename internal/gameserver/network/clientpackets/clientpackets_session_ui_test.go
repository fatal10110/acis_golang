package clientpackets

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// ---- from appearing_test.go ----
func TestDecodeAppearing(t *testing.T) {
	if _, err := DecodeAppearing([]byte{OpcodeAppearing}); err != nil {
		t.Fatalf("DecodeAppearing: %v", err)
	}
}

func TestDecodeAppearingShort(t *testing.T) {
	if _, err := DecodeAppearing(nil); err == nil {
		t.Fatal("DecodeAppearing: want error on short payload")
	}
}

// ---- from authlogin_test.go ----
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

// ---- from characterrestore_test.go ----
func TestDecodeCharacterRestore(t *testing.T) {
	payload := make([]byte, 1+characterRestoreSize)
	payload[0] = OpcodeCharacterRestore
	binary.LittleEndian.PutUint32(payload[1:], 4)

	got, err := DecodeCharacterRestore(payload)
	if err != nil {
		t.Fatalf("DecodeCharacterRestore: %v", err)
	}
	if want := (CharacterRestore{Slot: 4}); got != want {
		t.Errorf("DecodeCharacterRestore = %+v, want %+v", got, want)
	}
}

func TestDecodeCharacterRestore_Short(t *testing.T) {
	if _, err := DecodeCharacterRestore([]byte{OpcodeCharacterRestore}); err == nil {
		t.Error("DecodeCharacterRestore: want error on short payload, got nil")
	}
}

// ---- from crest_test.go ----
func TestDecodeRequestPledgeCrest(t *testing.T) {
	payload := []byte{
		OpcodeRequestPledgeCrest,
		0x65, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestPledgeCrest(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPledgeCrest: %v", err)
	}
	if got.CrestID != 101 {
		t.Fatalf("CrestID = %d, want 101", got.CrestID)
	}
}

func TestDecodeRequestAllyCrest(t *testing.T) {
	payload := []byte{
		OpcodeRequestAllyCrest,
		0x67, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAllyCrest(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAllyCrest: %v", err)
	}
	if got.CrestID != 103 {
		t.Fatalf("CrestID = %d, want 103", got.CrestID)
	}
}

func TestDecodeRequestExPledgeCrestLarge(t *testing.T) {
	payload := []byte{
		OpcodeExtended,
		0x10, 0x00,
		0x69, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestExPledgeCrestLarge(payload)
	if err != nil {
		t.Fatalf("DecodeRequestExPledgeCrestLarge: %v", err)
	}
	if got.CrestID != 105 {
		t.Fatalf("CrestID = %d, want 105", got.CrestID)
	}
}

func TestDecodeRequestPledgeCrestShort(t *testing.T) {
	if _, err := DecodeRequestPledgeCrest([]byte{OpcodeRequestPledgeCrest, 1}); err == nil {
		t.Fatal("DecodeRequestPledgeCrest: want error on short payload")
	}
}

func TestDecodeRequestAllyCrestShort(t *testing.T) {
	if _, err := DecodeRequestAllyCrest([]byte{OpcodeRequestAllyCrest, 1}); err == nil {
		t.Fatal("DecodeRequestAllyCrest: want error on short payload")
	}
}

func TestDecodeRequestExPledgeCrestLargeShort(t *testing.T) {
	if _, err := DecodeRequestExPledgeCrestLarge([]byte{OpcodeExtended, 0x10, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestExPledgeCrestLarge: want error on short payload")
	}
	if _, err := DecodeRequestExPledgeCrestLarge([]byte{OpcodeExtended, 0x11, 0x00, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestExPledgeCrestLarge: want error on wrong extended opcode")
	}
}

// ---- from dlganswer_test.go ----
func TestDecodeDlgAnswer(t *testing.T) {
	payload := []byte{
		OpcodeDlgAnswer,
		0x32, 0x07, 0x00, 0x00, // messageId 1842
		0x01, 0x00, 0x00, 0x00, // answer accept
		0x39, 0x30, 0x00, 0x00, // requesterId 12345
	}
	got, err := DecodeDlgAnswer(payload)
	if err != nil {
		t.Fatalf("DecodeDlgAnswer: %v", err)
	}
	if got.MessageID != 1842 || got.Answer != 1 || got.RequesterID != 12345 {
		t.Fatalf("DecodeDlgAnswer() = %+v, want {MessageID:1842 Answer:1 RequesterID:12345}", got)
	}
}

func TestDecodeDlgAnswerShort(t *testing.T) {
	if _, err := DecodeDlgAnswer([]byte{OpcodeDlgAnswer, 0x01}); err == nil {
		t.Fatal("DecodeDlgAnswer() error = nil, want short-payload error")
	}
}

// ---- from html_test.go ----
func TestDecodeRequestLinkHTML(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestLinkHtml)
	w.WriteString("help/tutorial.htm")

	got, err := DecodeRequestLinkHTML(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeRequestLinkHTML: %v", err)
	}
	if got.Link != "help/tutorial.htm" {
		t.Fatalf("Link = %q, want help/tutorial.htm", got.Link)
	}
}

func TestDecodeRequestLinkHTMLShort(t *testing.T) {
	if _, err := DecodeRequestLinkHTML([]byte{OpcodeRequestLinkHtml, 'x'}); err == nil {
		t.Fatal("DecodeRequestLinkHTML: want error on unterminated string")
	}
}

func TestDecodeRequestBypassToServer(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestBypassToServer)
	w.WriteString("player_help tutorial.htm")

	got, err := DecodeRequestBypassToServer(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeRequestBypassToServer: %v", err)
	}
	if got.Command != "player_help tutorial.htm" {
		t.Fatalf("Command = %q, want player_help tutorial.htm", got.Command)
	}
}

func TestDecodeRequestBypassToServerShort(t *testing.T) {
	if _, err := DecodeRequestBypassToServer([]byte{OpcodeRequestBypassToServer, 'x'}); err == nil {
		t.Fatal("DecodeRequestBypassToServer: want error on unterminated string")
	}
}

// ---- from protocolversion_test.go ----
func TestDecodeProtocolVersion(t *testing.T) {
	payload := []byte{OpcodeProtocolVersion, 0x21, 0xc6, 0x00, 0x00} // 0xc621, Interlude revision
	got, err := DecodeProtocolVersion(payload)
	if err != nil {
		t.Fatalf("DecodeProtocolVersion: %v", err)
	}
	if got.Revision != 0xc621 {
		t.Errorf("Revision = %#x, want %#x", got.Revision, 0xc621)
	}
}

func TestDecodeProtocolVersionShort(t *testing.T) {
	if _, err := DecodeProtocolVersion([]byte{OpcodeProtocolVersion, 0x01}); err == nil {
		t.Fatal("DecodeProtocolVersion() error = nil, want short-payload error")
	}
}

// ---- from requestcharactercreate_test.go ----
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

// ---- from requestcharacterdelete_test.go ----
func TestDecodeRequestCharacterDelete(t *testing.T) {
	payload := make([]byte, 1+requestCharacterDeleteSize)
	payload[0] = OpcodeRequestCharacterDelete
	binary.LittleEndian.PutUint32(payload[1:], 2)

	got, err := DecodeRequestCharacterDelete(payload)
	if err != nil {
		t.Fatalf("DecodeRequestCharacterDelete: %v", err)
	}
	if want := (RequestCharacterDelete{Slot: 2}); got != want {
		t.Errorf("DecodeRequestCharacterDelete = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestCharacterDelete_Short(t *testing.T) {
	if _, err := DecodeRequestCharacterDelete([]byte{OpcodeRequestCharacterDelete, 0, 1}); err == nil {
		t.Error("DecodeRequestCharacterDelete: want error on short payload, got nil")
	}
}

// ---- from requestgamestart_test.go ----
func TestDecodeRequestGameStart(t *testing.T) {
	var payload []byte
	payload = append(payload, OpcodeRequestGameStart)
	payload = binary.LittleEndian.AppendUint32(payload, 2) // slot
	payload = binary.LittleEndian.AppendUint16(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored

	got, err := DecodeRequestGameStart(payload)
	if err != nil {
		t.Fatalf("DecodeRequestGameStart: %v", err)
	}
	if want := (RequestGameStart{Slot: 2}); got != want {
		t.Errorf("DecodeRequestGameStart = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestGameStart_Short(t *testing.T) {
	if _, err := DecodeRequestGameStart([]byte{OpcodeRequestGameStart, 0, 1}); err == nil {
		t.Error("DecodeRequestGameStart: want error on short payload, got nil")
	}
}

// ---- from requestrestartpoint_test.go ----
func TestDecodeRequestRestartPoint(t *testing.T) {
	payload := make([]byte, 1+requestRestartPointSize)
	payload[0] = OpcodeRequestRestartPoint
	binary.LittleEndian.PutUint32(payload[1:], 27)

	got, err := DecodeRequestRestartPoint(payload)
	if err != nil {
		t.Fatalf("DecodeRequestRestartPoint: %v", err)
	}
	if want := (RequestRestartPoint{RequestType: 27}); got != want {
		t.Errorf("DecodeRequestRestartPoint = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestRestartPoint_Short(t *testing.T) {
	if _, err := DecodeRequestRestartPoint([]byte{OpcodeRequestRestartPoint}); err == nil {
		t.Error("DecodeRequestRestartPoint: want error on short payload, got nil")
	}
}
