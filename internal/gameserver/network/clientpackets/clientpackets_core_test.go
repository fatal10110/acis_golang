package clientpackets

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// ---- from acquire_skill_test.go ----
func TestDecodeRequestAcquireSkillInfo(t *testing.T) {
	payload := []byte{
		OpcodeRequestAcquireSkillInfo,
		0x03, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAcquireSkillInfo(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAcquireSkillInfo: %v", err)
	}
	want := RequestAcquireSkillInfo{SkillID: 3, Level: 1, SkillType: 0}
	if got != want {
		t.Fatalf("DecodeRequestAcquireSkillInfo = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestAcquireSkill(t *testing.T) {
	payload := []byte{
		OpcodeRequestAcquireSkill,
		0xf8, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAcquireSkill(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAcquireSkill: %v", err)
	}
	want := RequestAcquireSkill{SkillID: 248, Level: 2, SkillType: 0}
	if got != want {
		t.Fatalf("DecodeRequestAcquireSkill = %+v, want %+v", got, want)
	}
}

func TestDecodeAcquireSkillShort(t *testing.T) {
	if _, err := DecodeRequestAcquireSkillInfo([]byte{OpcodeRequestAcquireSkillInfo, 1}); err == nil {
		t.Fatal("DecodeRequestAcquireSkillInfo: want error on short payload")
	}
	if _, err := DecodeRequestAcquireSkill([]byte{OpcodeRequestAcquireSkill, 1}); err == nil {
		t.Fatal("DecodeRequestAcquireSkill: want error on short payload")
	}
}

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

// ---- from cannotmoveanymore_test.go ----
func TestDecodeCannotMoveAnymore(t *testing.T) {
	payload := []byte{
		OpcodeCannotMoveAnymore,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00, 0x80, 0x00, 0x00,
	}

	got, err := DecodeCannotMoveAnymore(payload)
	if err != nil {
		t.Fatalf("DecodeCannotMoveAnymore: %v", err)
	}
	want := CannotMoveAnymore{X: 46160, Y: 41237, Z: -3534, Heading: 32768}
	if got != want {
		t.Fatalf("DecodeCannotMoveAnymore = %+v, want %+v", got, want)
	}
}

func TestDecodeCannotMoveAnymoreShort(t *testing.T) {
	if _, err := DecodeCannotMoveAnymore([]byte{OpcodeCannotMoveAnymore, 1, 2}); err == nil {
		t.Fatal("DecodeCannotMoveAnymore: want error on short payload")
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

// ---- from item_ops_test.go ----
func TestDecodeRequestDropItem(t *testing.T) {
	payload := []byte{
		OpcodeRequestDropItem,
		0xf4, 0x01, 0x00, 0x00,
		0x28, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}

	got, err := DecodeRequestDropItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestDropItem: %v", err)
	}
	want := RequestDropItem{ObjectID: 500, Count: 40, X: 46160, Y: 41237, Z: -3534}
	if got != want {
		t.Fatalf("DecodeRequestDropItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestDestroyItem(t *testing.T) {
	payload := []byte{OpcodeRequestDestroyItem, 0xf5, 0x01, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}

	got, err := DecodeRequestDestroyItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestDestroyItem: %v", err)
	}
	want := RequestDestroyItem{ObjectID: 501, Count: 2}
	if got != want {
		t.Fatalf("DecodeRequestDestroyItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestCrystallizeItem(t *testing.T) {
	payload := []byte{OpcodeRequestCrystallizeItem, 0xf6, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}

	got, err := DecodeRequestCrystallizeItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestCrystallizeItem: %v", err)
	}
	want := RequestCrystallizeItem{ObjectID: 502, Count: 1}
	if got != want {
		t.Fatalf("DecodeRequestCrystallizeItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestEnchantItem(t *testing.T) {
	payload := []byte{OpcodeRequestEnchantItem, 0xf7, 0x01, 0x00, 0x00}

	got, err := DecodeRequestEnchantItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestEnchantItem: %v", err)
	}
	want := RequestEnchantItem{ObjectID: 503}
	if got != want {
		t.Fatalf("DecodeRequestEnchantItem = %+v, want %+v", got, want)
	}
}

func TestDecodePetItemRequests(t *testing.T) {
	use, err := DecodeRequestPetUseItem([]byte{OpcodeRequestPetUseItem, 0x21, 0x03, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestPetUseItem: %v", err)
	}
	if use != (RequestPetUseItem{ObjectID: 801}) {
		t.Fatalf("DecodeRequestPetUseItem = %+v, want ObjectID 801", use)
	}

	give, err := DecodeRequestGiveItemToPet([]byte{OpcodeRequestGiveItemToPet, 0x22, 0x03, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestGiveItemToPet: %v", err)
	}
	if give != (RequestGiveItemToPet{ObjectID: 802, Count: 5}) {
		t.Fatalf("DecodeRequestGiveItemToPet = %+v, want ObjectID 802 Count 5", give)
	}

	take, err := DecodeRequestGetItemFromPet([]byte{OpcodeRequestGetItemFromPet, 0x23, 0x03, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff})
	if err != nil {
		t.Fatalf("DecodeRequestGetItemFromPet: %v", err)
	}
	if take != (RequestGetItemFromPet{ObjectID: 803, Count: 6, Unknown: -1}) {
		t.Fatalf("DecodeRequestGetItemFromPet = %+v, want ObjectID 803 Count 6 Unknown -1", take)
	}

	pickup, err := DecodeRequestPetGetItem([]byte{OpcodeRequestPetGetItem, 0x24, 0x03, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestPetGetItem: %v", err)
	}
	if pickup != (RequestPetGetItem{ObjectID: 804}) {
		t.Fatalf("DecodeRequestPetGetItem = %+v, want ObjectID 804", pickup)
	}
}

func TestDecodeSendTimeCheck(t *testing.T) {
	payload := []byte{OpcodeSendTimeCheck, 0x11, 0x00, 0x00, 0x00, 0x22, 0x00, 0x00, 0x00}

	got, err := DecodeSendTimeCheck(payload)
	if err != nil {
		t.Fatalf("DecodeSendTimeCheck: %v", err)
	}
	want := SendTimeCheck{RequestID: 17, ResponseID: 34}
	if got != want {
		t.Fatalf("DecodeSendTimeCheck = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestAutoSoulShot(t *testing.T) {
	payload := []byte{
		OpcodeExtended,
		0x05, 0x00,
		0xb7, 0x05, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAutoSoulShot(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAutoSoulShot: %v", err)
	}
	want := RequestAutoSoulShot{ItemID: 1463, Type: 1}
	if got != want {
		t.Fatalf("DecodeRequestAutoSoulShot = %+v, want %+v", got, want)
	}
}

func TestDecodeItemOpsShort(t *testing.T) {
	if _, err := DecodeRequestDropItem([]byte{OpcodeRequestDropItem, 1}); err == nil {
		t.Fatal("DecodeRequestDropItem: want error on short payload")
	}
	if _, err := DecodeRequestDestroyItem([]byte{OpcodeRequestDestroyItem, 1}); err == nil {
		t.Fatal("DecodeRequestDestroyItem: want error on short payload")
	}
	if _, err := DecodeRequestCrystallizeItem([]byte{OpcodeRequestCrystallizeItem, 1}); err == nil {
		t.Fatal("DecodeRequestCrystallizeItem: want error on short payload")
	}
	if _, err := DecodeRequestEnchantItem([]byte{OpcodeRequestEnchantItem, 1}); err == nil {
		t.Fatal("DecodeRequestEnchantItem: want error on short payload")
	}
	if _, err := DecodeRequestPetUseItem([]byte{OpcodeRequestPetUseItem, 1}); err == nil {
		t.Fatal("DecodeRequestPetUseItem: want error on short payload")
	}
	if _, err := DecodeRequestGiveItemToPet([]byte{OpcodeRequestGiveItemToPet, 1}); err == nil {
		t.Fatal("DecodeRequestGiveItemToPet: want error on short payload")
	}
	if _, err := DecodeRequestGetItemFromPet([]byte{OpcodeRequestGetItemFromPet, 1}); err == nil {
		t.Fatal("DecodeRequestGetItemFromPet: want error on short payload")
	}
	if _, err := DecodeRequestPetGetItem([]byte{OpcodeRequestPetGetItem, 1}); err == nil {
		t.Fatal("DecodeRequestPetGetItem: want error on short payload")
	}
	if _, err := DecodeSendTimeCheck([]byte{OpcodeSendTimeCheck, 1}); err == nil {
		t.Fatal("DecodeSendTimeCheck: want error on short payload")
	}
	if _, err := DecodeRequestAutoSoulShot([]byte{OpcodeExtended, 0x05, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestAutoSoulShot: want error on short payload")
	}
	if _, err := DecodeRequestAutoSoulShot([]byte{OpcodeExtended, 0x08, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestAutoSoulShot: want error on wrong extended opcode")
	}
}

// ---- from magic_skill_use_ground_test.go ----
func TestDecodeRequestExMagicSkillUseGround(t *testing.T) {
	payload := []byte{
		OpcodeExtended,
		0x2f, 0x00,
		0x10, 0x00, 0x00, 0x00,
		0x20, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01,
	}

	got, err := DecodeRequestExMagicSkillUseGround(payload)
	if err != nil {
		t.Fatalf("DecodeRequestExMagicSkillUseGround: %v", err)
	}
	want := RequestExMagicSkillUseGround{X: 16, Y: 32, Z: 0, SkillID: 3, CtrlPressed: true, ShiftPressed: true}
	if got != want {
		t.Fatalf("DecodeRequestExMagicSkillUseGround = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestExMagicSkillUseGroundShort(t *testing.T) {
	if _, err := DecodeRequestExMagicSkillUseGround([]byte{OpcodeExtended, 0x2f, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestExMagicSkillUseGround: want error on short payload")
	}
}

func TestDecodeRequestExMagicSkillUseGroundWrongOpcode(t *testing.T) {
	payload := []byte{
		OpcodeExtended,
		0x10, 0x00,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
	if _, err := DecodeRequestExMagicSkillUseGround(payload); err == nil {
		t.Fatal("DecodeRequestExMagicSkillUseGround: want error on wrong extended opcode")
	}
}

// ---- from magic_skill_use_test.go ----
func TestDecodeRequestMagicSkillUse(t *testing.T) {
	payload := []byte{
		OpcodeRequestMagicSkillUse,
		0x03, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01,
	}

	got, err := DecodeRequestMagicSkillUse(payload)
	if err != nil {
		t.Fatalf("DecodeRequestMagicSkillUse: %v", err)
	}
	want := RequestMagicSkillUse{SkillID: 3, CtrlPressed: true, ShiftPressed: true}
	if got != want {
		t.Fatalf("DecodeRequestMagicSkillUse = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestMagicSkillUseShort(t *testing.T) {
	if _, err := DecodeRequestMagicSkillUse([]byte{OpcodeRequestMagicSkillUse, 1}); err == nil {
		t.Fatal("DecodeRequestMagicSkillUse: want error on short payload")
	}
}

// ---- from movebackwardtolocation_test.go ----
func TestDecodeMoveBackwardToLocation(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		want MoveBackwardToLocation
	}{
		{
			name: "with movement mode",
			hex:  "0150b4000015a1000032f2ffff25b400001fa1000034f2ffff01000000",
			want: MoveBackwardToLocation{
				TargetX:      46160,
				TargetY:      41237,
				TargetZ:      -3534,
				OriginX:      46117,
				OriginY:      41247,
				OriginZ:      -3532,
				MoveMovement: 1,
			},
		},
		{
			name: "without movement mode",
			hex:  "0150b4000015a1000032f2ffff25b400001fa1000034f2ffff",
			want: MoveBackwardToLocation{
				TargetX:      46160,
				TargetY:      41237,
				TargetZ:      -3534,
				OriginX:      46117,
				OriginY:      41247,
				OriginZ:      -3532,
				MoveMovement: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("decode test payload: %v", err)
			}

			got, err := DecodeMoveBackwardToLocation(payload)
			if err != nil {
				t.Fatalf("DecodeMoveBackwardToLocation: %v", err)
			}
			if got != tt.want {
				t.Fatalf("DecodeMoveBackwardToLocation = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeMoveBackwardToLocation_Short(t *testing.T) {
	if _, err := DecodeMoveBackwardToLocation([]byte{OpcodeMoveBackwardToLocation, 1, 2}); err == nil {
		t.Fatal("DecodeMoveBackwardToLocation: want error on short payload")
	}
}

// ---- from pet_name_test.go ----
func TestDecodeRequestChangePetName(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestChangePetName)
	w.WriteString("Rex")

	got, err := DecodeRequestChangePetName(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeRequestChangePetName: %v", err)
	}
	if got.Name != "Rex" {
		t.Fatalf("Name = %q, want Rex", got.Name)
	}
}

func TestDecodeRequestChangePetNameShort(t *testing.T) {
	if _, err := DecodeRequestChangePetName([]byte{OpcodeRequestChangePetName, 'x'}); err == nil {
		t.Fatal("DecodeRequestChangePetName: want error on unterminated string")
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

// ---- from requestactionuse_test.go ----
func TestDecodeRequestActionUse(t *testing.T) {
	payload := []byte{
		OpcodeRequestActionUse,
		0x34, 0x00, 0x00, 0x00, // action id 52
		0x01, 0x00, 0x00, 0x00, // ctrl
		0x01, // shift
	}

	got, err := DecodeRequestActionUse(payload)
	if err != nil {
		t.Fatalf("DecodeRequestActionUse: %v", err)
	}
	if got != (RequestActionUse{ActionID: 52, CtrlPressed: true, ShiftPressed: true}) {
		t.Fatalf("DecodeRequestActionUse = %+v", got)
	}
}

func TestDecodeRequestActionUse_Short(t *testing.T) {
	if _, err := DecodeRequestActionUse([]byte{OpcodeRequestActionUse, 1, 2}); err == nil {
		t.Fatal("DecodeRequestActionUse: want error on short payload")
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

// ---- from rotation_test.go ----
func TestDecodeStartRotating(t *testing.T) {
	payload := []byte{
		OpcodeStartRotating,
		0x00, 0x80, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeStartRotating(payload)
	if err != nil {
		t.Fatalf("DecodeStartRotating: %v", err)
	}
	if got != (StartRotating{Degree: 32768, Side: 1}) {
		t.Fatalf("DecodeStartRotating = %+v", got)
	}
}

func TestDecodeFinishRotating(t *testing.T) {
	payload := []byte{
		OpcodeFinishRotating,
		0x34, 0x12, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeFinishRotating(payload)
	if err != nil {
		t.Fatalf("DecodeFinishRotating: %v", err)
	}
	if got != (FinishRotating{Degree: 0x1234, Side: 1}) {
		t.Fatalf("DecodeFinishRotating = %+v", got)
	}
}

func TestDecodeRotatingShort(t *testing.T) {
	if _, err := DecodeStartRotating([]byte{OpcodeStartRotating, 1, 2}); err == nil {
		t.Fatal("DecodeStartRotating: want error on short payload")
	}
	if _, err := DecodeFinishRotating([]byte{OpcodeFinishRotating, 1, 2}); err == nil {
		t.Fatal("DecodeFinishRotating: want error on short payload")
	}
}

// ---- from shop_trade_test.go ----
func TestDecodeTradeRequest(t *testing.T) {
	payload := []byte{OpcodeTradeRequest, 0x04, 0x03, 0x02, 0x01}

	got, err := DecodeTradeRequest(payload)
	if err != nil {
		t.Fatalf("DecodeTradeRequest: %v", err)
	}
	want := TradeRequest{ObjectID: 0x01020304}
	if got != want {
		t.Fatalf("DecodeTradeRequest = %+v, want %+v", got, want)
	}
}

func TestDecodeAddTradeItem(t *testing.T) {
	payload := []byte{
		OpcodeAddTradeItem,
		0x01, 0x00, 0x00, 0x00,
		0x2c, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x00, 0x00,
	}

	got, err := DecodeAddTradeItem(payload)
	if err != nil {
		t.Fatalf("DecodeAddTradeItem: %v", err)
	}
	want := AddTradeItem{TradeID: 1, ObjectID: 300, Count: 5}
	if got != want {
		t.Fatalf("DecodeAddTradeItem = %+v, want %+v", got, want)
	}
}

func TestDecodeTradeDone(t *testing.T) {
	payload := []byte{OpcodeTradeDone, 0x01, 0x00, 0x00, 0x00}

	got, err := DecodeTradeDone(payload)
	if err != nil {
		t.Fatalf("DecodeTradeDone: %v", err)
	}
	want := TradeDone{Response: 1}
	if got != want {
		t.Fatalf("DecodeTradeDone = %+v, want %+v", got, want)
	}
}

func TestDecodeAnswerTradeRequest(t *testing.T) {
	payload := []byte{OpcodeAnswerTradeRequest, 0x00, 0x00, 0x00, 0x00}

	got, err := DecodeAnswerTradeRequest(payload)
	if err != nil {
		t.Fatalf("DecodeAnswerTradeRequest: %v", err)
	}
	want := AnswerTradeRequest{Response: 0}
	if got != want {
		t.Fatalf("DecodeAnswerTradeRequest = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestShortCutReg(t *testing.T) {
	payload := []byte{
		OpcodeRequestShortCutReg,
		0x02, 0x00, 0x00, 0x00, // skill
		0x0f, 0x00, 0x00, 0x00, // slot 3, page 1
		0xf8, 0x00, 0x00, 0x00, // skill id
		0x01, 0x00, 0x00, 0x00, // character type
	}

	got, err := DecodeRequestShortCutReg(payload)
	if err != nil {
		t.Fatalf("DecodeRequestShortCutReg: %v", err)
	}
	want := RequestShortCutReg{Type: 2, Slot: 3, Page: 1, ID: 248, CharacterType: 1}
	if got != want {
		t.Fatalf("DecodeRequestShortCutReg = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestShortCutDel(t *testing.T) {
	payload := []byte{OpcodeRequestShortCutDel, 0x0f, 0x00, 0x00, 0x00}

	got, err := DecodeRequestShortCutDel(payload)
	if err != nil {
		t.Fatalf("DecodeRequestShortCutDel: %v", err)
	}
	want := RequestShortCutDel{Slot: 3, Page: 1}
	if got != want {
		t.Fatalf("DecodeRequestShortCutDel = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestBuyItem(t *testing.T) {
	payload := []byte{
		OpcodeRequestBuyItem,
		0x65, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x39, 0x30, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0x57, 0x04, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestBuyItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestBuyItem: %v", err)
	}
	want := RequestBuyItem{ListID: 101, Items: []BuyItemRequest{
		{ItemID: 12345, Count: 3},
		{ItemID: 1111, Count: 1},
	}}
	if got.ListID != want.ListID || len(got.Items) != len(want.Items) || got.Items[0] != want.Items[0] || got.Items[1] != want.Items[1] {
		t.Fatalf("DecodeRequestBuyItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestSellItem(t *testing.T) {
	payload := []byte{
		OpcodeRequestSellItem,
		0xc8, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0xf4, 0x01, 0x00, 0x00,
		0x39, 0x30, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0xf5, 0x01, 0x00, 0x00,
		0x57, 0x04, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestSellItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestSellItem: %v", err)
	}
	want := RequestSellItem{ListID: 200, Items: []SellItemRequest{
		{ObjectID: 500, ItemID: 12345, Count: 3},
		{ObjectID: 501, ItemID: 1111, Count: 1},
	}}
	if got.ListID != want.ListID || len(got.Items) != len(want.Items) || got.Items[0] != want.Items[0] || got.Items[1] != want.Items[1] {
		t.Fatalf("DecodeRequestSellItem = %+v, want %+v", got, want)
	}
}

func TestDecodeShopTradeShort(t *testing.T) {
	if _, err := DecodeTradeRequest([]byte{OpcodeTradeRequest, 1}); err == nil {
		t.Fatal("DecodeTradeRequest: want error on short payload")
	}
	if _, err := DecodeAddTradeItem([]byte{OpcodeAddTradeItem, 1}); err == nil {
		t.Fatal("DecodeAddTradeItem: want error on short payload")
	}
	if _, err := DecodeTradeDone([]byte{OpcodeTradeDone, 1}); err == nil {
		t.Fatal("DecodeTradeDone: want error on short payload")
	}
	if _, err := DecodeAnswerTradeRequest([]byte{OpcodeAnswerTradeRequest, 1}); err == nil {
		t.Fatal("DecodeAnswerTradeRequest: want error on short payload")
	}
	if _, err := DecodeRequestShortCutReg([]byte{OpcodeRequestShortCutReg, 1}); err == nil {
		t.Fatal("DecodeRequestShortCutReg: want error on short payload")
	}
	if _, err := DecodeRequestShortCutDel([]byte{OpcodeRequestShortCutDel, 1}); err == nil {
		t.Fatal("DecodeRequestShortCutDel: want error on short payload")
	}
	if _, err := DecodeRequestBuyItem([]byte{OpcodeRequestBuyItem, 1}); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on short payload")
	}
	if _, err := DecodeRequestSellItem([]byte{OpcodeRequestSellItem, 1}); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on short payload")
	}
}

func TestDecodeShopTradeRejectsMalformedLists(t *testing.T) {
	if _, err := DecodeRequestBuyItem([]byte{
		OpcodeRequestBuyItem,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on zero item count")
	}
	if _, err := DecodeRequestSellItem([]byte{
		OpcodeRequestSellItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on mismatched row length")
	}
	if _, err := DecodeRequestBuyItem([]byte{
		OpcodeRequestBuyItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on zero item id")
	}
	if _, err := DecodeRequestSellItem([]byte{
		OpcodeRequestSellItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on zero count")
	}
}

// TestDecodeShopTradeRejectsOversizedCount proves RequestBuyItem/
// RequestSellItem reject a row count above Config.MAX_ITEM_IN_PACKET
// (RequestBuyItem.java:32, RequestSellItem.java:26), mirroring the cap
// warehouse.go already enforces for sibling packets.
func TestDecodeShopTradeRejectsOversizedCount(t *testing.T) {
	const oversized = maxShopItemInPacket + 1

	buyPayload := []byte{OpcodeRequestBuyItem, 0x01, 0x00, 0x00, 0x00}
	buyPayload = binary.LittleEndian.AppendUint32(buyPayload, uint32(oversized))
	buyPayload = append(buyPayload, make([]byte, oversized*shopBuyRowSize)...)
	if _, err := DecodeRequestBuyItem(buyPayload); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on count exceeding MAX_ITEM_IN_PACKET")
	}

	sellPayload := []byte{OpcodeRequestSellItem, 0x01, 0x00, 0x00, 0x00}
	sellPayload = binary.LittleEndian.AppendUint32(sellPayload, uint32(oversized))
	sellPayload = append(sellPayload, make([]byte, oversized*shopSellRowSize)...)
	if _, err := DecodeRequestSellItem(sellPayload); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on count exceeding MAX_ITEM_IN_PACKET")
	}
}

// ---- from skill_enchant_test.go ----
func TestDecodeSkillEnchantRequests(t *testing.T) {
	info, err := DecodeRequestExEnchantSkillInfo([]byte{
		OpcodeExtended,
		0x06, 0x00,
		0x7c, 0x00, 0x00, 0x00,
		0x65, 0x00, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestExEnchantSkillInfo: %v", err)
	}
	if info != (RequestExEnchantSkillInfo{SkillID: 124, SkillLevel: 101}) {
		t.Fatalf("DecodeRequestExEnchantSkillInfo = %+v, want skill 124 level 101", info)
	}

	enchant, err := DecodeRequestExEnchantSkill([]byte{
		OpcodeExtended,
		0x07, 0x00,
		0x7d, 0x00, 0x00, 0x00,
		0x66, 0x00, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestExEnchantSkill: %v", err)
	}
	if enchant != (RequestExEnchantSkill{SkillID: 125, SkillLevel: 102}) {
		t.Fatalf("DecodeRequestExEnchantSkill = %+v, want skill 125 level 102", enchant)
	}
}

func TestDecodeSkillEnchantRequestsShort(t *testing.T) {
	if _, err := DecodeRequestExEnchantSkillInfo([]byte{OpcodeExtended, 0x06, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestExEnchantSkillInfo: want error on short payload")
	}
	if _, err := DecodeRequestExEnchantSkill([]byte{OpcodeExtended, 0x07, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestExEnchantSkill: want error on short payload")
	}
}

func TestDecodeSkillEnchantRequestsWrongExtendedOpcode(t *testing.T) {
	if _, err := DecodeRequestExEnchantSkillInfo([]byte{OpcodeExtended, 0x07, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestExEnchantSkillInfo: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestExEnchantSkill([]byte{OpcodeExtended, 0x06, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestExEnchantSkill: want error on wrong extended opcode")
	}
}

// ---- from stance_social_test.go ----
func TestDecodeRequestChangeMoveType(t *testing.T) {
	payload := []byte{OpcodeRequestChangeMoveType, 0x01, 0x00, 0x00, 0x00}

	got, err := DecodeRequestChangeMoveType(payload)
	if err != nil {
		t.Fatalf("DecodeRequestChangeMoveType: %v", err)
	}
	if got != (RequestChangeMoveType{Run: true}) {
		t.Fatalf("DecodeRequestChangeMoveType = %+v", got)
	}
}

func TestDecodeRequestChangeWaitType(t *testing.T) {
	payload := []byte{OpcodeRequestChangeWaitType, 0x00, 0x00, 0x00, 0x00}

	got, err := DecodeRequestChangeWaitType(payload)
	if err != nil {
		t.Fatalf("DecodeRequestChangeWaitType: %v", err)
	}
	if got != (RequestChangeWaitType{Stand: false}) {
		t.Fatalf("DecodeRequestChangeWaitType = %+v", got)
	}
}

func TestDecodeRequestSocialAction(t *testing.T) {
	payload := []byte{OpcodeRequestSocialAction, 0x0d, 0x00, 0x00, 0x00}

	got, err := DecodeRequestSocialAction(payload)
	if err != nil {
		t.Fatalf("DecodeRequestSocialAction: %v", err)
	}
	if got != (RequestSocialAction{ActionID: 13}) {
		t.Fatalf("DecodeRequestSocialAction = %+v", got)
	}
}

func TestDecodeStanceAndSocialShort(t *testing.T) {
	if _, err := DecodeRequestChangeMoveType([]byte{OpcodeRequestChangeMoveType, 1}); err == nil {
		t.Fatal("DecodeRequestChangeMoveType: want error on short payload")
	}
	if _, err := DecodeRequestChangeWaitType([]byte{OpcodeRequestChangeWaitType, 1}); err == nil {
		t.Fatal("DecodeRequestChangeWaitType: want error on short payload")
	}
	if _, err := DecodeRequestSocialAction([]byte{OpcodeRequestSocialAction, 1}); err == nil {
		t.Fatal("DecodeRequestSocialAction: want error on short payload")
	}
}

// ---- from targetaction_test.go ----
func TestDecodeAction(t *testing.T) {
	payload := []byte{
		OpcodeAction,
		0x39, 0x30, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x01,
	}

	got, err := DecodeAction(payload)
	if err != nil {
		t.Fatalf("DecodeAction: %v", err)
	}
	want := Action{ObjectID: 12345, OriginX: 46160, OriginY: 41237, OriginZ: -3534, Shift: true}
	if got != want {
		t.Fatalf("DecodeAction = %+v, want %+v", got, want)
	}
}

func TestDecodeAttackRequest(t *testing.T) {
	payload := []byte{
		OpcodeAttackRequest,
		0x39, 0x30, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00,
	}

	got, err := DecodeAttackRequest(payload)
	if err != nil {
		t.Fatalf("DecodeAttackRequest: %v", err)
	}
	want := AttackRequest{ObjectID: 12345, OriginX: 46160, OriginY: 41237, OriginZ: -3534}
	if got != want {
		t.Fatalf("DecodeAttackRequest = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestTargetCancel(t *testing.T) {
	payload := []byte{OpcodeRequestTargetCancel, 0x01, 0x00}

	got, err := DecodeRequestTargetCancel(payload)
	if err != nil {
		t.Fatalf("DecodeRequestTargetCancel: %v", err)
	}
	if got != (RequestTargetCancel{Unselect: 1}) {
		t.Fatalf("DecodeRequestTargetCancel = %+v", got)
	}
}

func TestDecodeTargetActionShort(t *testing.T) {
	if _, err := DecodeAction([]byte{OpcodeAction, 1, 2}); err == nil {
		t.Fatal("DecodeAction: want error on short payload")
	}
	if _, err := DecodeAttackRequest([]byte{OpcodeAttackRequest, 1, 2}); err == nil {
		t.Fatal("DecodeAttackRequest: want error on short payload")
	}
	if _, err := DecodeRequestTargetCancel([]byte{OpcodeRequestTargetCancel}); err == nil {
		t.Fatal("DecodeRequestTargetCancel: want error on short payload")
	}
}

// ---- from validateposition_test.go ----
func TestDecodeValidatePosition(t *testing.T) {
	payload := []byte{
		OpcodeValidatePosition,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00, 0x80, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	got, err := DecodeValidatePosition(payload)
	if err != nil {
		t.Fatalf("DecodeValidatePosition: %v", err)
	}

	want := ValidatePosition{X: 46160, Y: 41237, Z: -3534, Heading: 32768}
	if got != want {
		t.Fatalf("DecodeValidatePosition = %+v, want %+v", got, want)
	}
}

func TestDecodeValidatePosition_Short(t *testing.T) {
	if _, err := DecodeValidatePosition([]byte{OpcodeValidatePosition, 1, 2}); err == nil {
		t.Fatal("DecodeValidatePosition: want error on short payload")
	}
}

// ---- from variation_test.go ----
func TestDecodeVariationRequests(t *testing.T) {
	target, err := DecodeRequestConfirmTargetItem([]byte{
		OpcodeExtended,
		0x29, 0x00,
		0xe8, 0x03, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmTargetItem: %v", err)
	}
	if target != (RequestConfirmTargetItem{ObjectID: 1000}) {
		t.Fatalf("DecodeRequestConfirmTargetItem = %+v, want ObjectID 1000", target)
	}

	refiner, err := DecodeRequestConfirmRefinerItem([]byte{
		OpcodeExtended,
		0x2a, 0x00,
		0xe8, 0x03, 0x00, 0x00,
		0xd0, 0x07, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmRefinerItem: %v", err)
	}
	if refiner != (RequestConfirmRefinerItem{TargetObjectID: 1000, RefinerObjectID: 2000}) {
		t.Fatalf("DecodeRequestConfirmRefinerItem = %+v, want target 1000 refiner 2000", refiner)
	}

	gemstone, err := DecodeRequestConfirmGemStone([]byte{
		OpcodeExtended,
		0x2b, 0x00,
		0xe8, 0x03, 0x00, 0x00,
		0xd0, 0x07, 0x00, 0x00,
		0xb8, 0x0b, 0x00, 0x00,
		0x24, 0x00, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmGemStone: %v", err)
	}
	wantGemstone := RequestConfirmGemStone{
		TargetObjectID:   1000,
		RefinerObjectID:  2000,
		GemstoneObjectID: 3000,
		GemstoneCount:    36,
	}
	if gemstone != wantGemstone {
		t.Fatalf("DecodeRequestConfirmGemStone = %+v, want %+v", gemstone, wantGemstone)
	}

	cancel, err := DecodeRequestConfirmCancelItem([]byte{
		OpcodeExtended,
		0x2d, 0x00,
		0xe8, 0x03, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmCancelItem: %v", err)
	}
	if cancel != (RequestConfirmCancelItem{ObjectID: 1000}) {
		t.Fatalf("DecodeRequestConfirmCancelItem = %+v, want ObjectID 1000", cancel)
	}
}

func TestDecodeVariationRequestsShort(t *testing.T) {
	if _, err := DecodeRequestConfirmTargetItem([]byte{OpcodeExtended, 0x29, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmTargetItem: want error on short payload")
	}
	if _, err := DecodeRequestConfirmRefinerItem([]byte{OpcodeExtended, 0x2a, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmRefinerItem: want error on short payload")
	}
	if _, err := DecodeRequestConfirmGemStone([]byte{OpcodeExtended, 0x2b, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmGemStone: want error on short payload")
	}
	if _, err := DecodeRequestConfirmCancelItem([]byte{OpcodeExtended, 0x2d, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmCancelItem: want error on short payload")
	}
}

func TestDecodeVariationRequestsWrongExtendedOpcode(t *testing.T) {
	if _, err := DecodeRequestConfirmTargetItem([]byte{OpcodeExtended, 0x2a, 0x00, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmTargetItem: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmRefinerItem([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmRefinerItem: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmGemStone([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmGemStone: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmCancelItem([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmCancelItem: want error on wrong extended opcode")
	}
}

// ---- from warehouse_test.go ----
func TestDecodeWarehouseItemBatchPackets(t *testing.T) {
	payload := []byte{
		OpcodeSendWarehouseDeposit,
		0x02, 0x00, 0x00, 0x00,
		0xf4, 0x01, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0xf5, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	deposit, err := DecodeSendWarehouseDepositList(payload)
	if err != nil {
		t.Fatalf("DecodeSendWarehouseDepositList: %v", err)
	}
	withdraw, err := DecodeSendWarehouseWithdrawList(append([]byte{OpcodeSendWarehouseWithdraw}, payload[1:]...))
	if err != nil {
		t.Fatalf("DecodeSendWarehouseWithdrawList: %v", err)
	}

	want := []ItemRequest{{ObjectID: 500, Count: 3}, {ObjectID: 501, Count: 1}}
	if !sameItemRequests(deposit.Items, want) {
		t.Fatalf("deposit items = %+v, want %+v", deposit.Items, want)
	}
	if !sameItemRequests(withdraw.Items, want) {
		t.Fatalf("withdraw items = %+v, want %+v", withdraw.Items, want)
	}
}

func TestDecodeWarehouseItemBatchRejectsMalformedPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"zero count", []byte{OpcodeSendWarehouseDeposit, 0, 0, 0, 0}},
		{"trailing byte", []byte{OpcodeSendWarehouseDeposit, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0}},
		{"bad object id", []byte{OpcodeSendWarehouseDeposit, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0}},
		{"negative count", []byte{OpcodeSendWarehouseDeposit, 1, 0, 0, 0, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeSendWarehouseDepositList(tt.payload); err == nil {
				t.Fatal("DecodeSendWarehouseDepositList: want error")
			}
		})
	}
}

func TestDecodeRequestPackageSendableItemList(t *testing.T) {
	payload := []byte{OpcodeRequestPackageItemList, 0x78, 0x56, 0x34, 0x12}

	got, err := DecodeRequestPackageSendableItemList(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPackageSendableItemList: %v", err)
	}
	if got.ObjectID != 0x12345678 {
		t.Fatalf("ObjectID = %#x, want 0x12345678", got.ObjectID)
	}
}

func TestDecodeRequestPackageSend(t *testing.T) {
	payload := []byte{
		OpcodeRequestPackageSend,
		0x78, 0x56, 0x34, 0x12,
		0x02, 0x00, 0x00, 0x00,
		0xf4, 0x01, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0xf5, 0x01, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestPackageSend(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPackageSend: %v", err)
	}
	want := RequestPackageSend{ObjectID: 0x12345678, Items: []ItemRequest{{ObjectID: 500, Count: 3}, {ObjectID: 501, Count: 1}}}
	if got.ObjectID != want.ObjectID || !sameItemRequests(got.Items, want.Items) {
		t.Fatalf("DecodeRequestPackageSend = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestPackageSendAllowsEmptyList(t *testing.T) {
	payload := []byte{OpcodeRequestPackageSend, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0}

	got, err := DecodeRequestPackageSend(payload)
	if err != nil {
		t.Fatalf("DecodeRequestPackageSend: %v", err)
	}
	if got.ObjectID != 0x12345678 || len(got.Items) != 0 {
		t.Fatalf("DecodeRequestPackageSend = %+v, want object id with no items", got)
	}
}

// TestDecodeRequestPackageSendCountAboveMaxIsNotShortPacket proves a
// count exceeding maxItemInPacket (mirroring Config.MAX_ITEM_IN_PACKET,
// whose readImpl() guard returns silently before any row read, so it can
// never throw BufferUnderflowException) is a plain validation error, not
// classified as a buffer-underflow-equivalent wire.ErrShortPacket -- even
// though its trailing byte count is also short for that count, matching
// the reference's guard order (count bound checked first, silently).
func TestDecodeRequestPackageSendCountAboveMaxIsNotShortPacket(t *testing.T) {
	payload := []byte{
		OpcodeRequestPackageSend,
		0x78, 0x56, 0x34, 0x12,
		0x65, 0x00, 0x00, 0x00, // count = 101, exceeds maxItemInPacket (100)
	}

	_, err := DecodeRequestPackageSend(payload)
	if err == nil {
		t.Fatal("DecodeRequestPackageSend: want error for count above max")
	}
	if errors.Is(err, wire.ErrShortPacket) {
		t.Fatalf("DecodeRequestPackageSend() error = %v, want a non-short-packet validation error", err)
	}
}

func TestDecodeRequestPackageSendRejectsMalformedPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"short header", []byte{OpcodeRequestPackageSend, 1}},
		{"negative count", []byte{OpcodeRequestPackageSend, 1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}},
		{"short item", []byte{OpcodeRequestPackageSend, 1, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeRequestPackageSend(tt.payload); err == nil {
				t.Fatal("DecodeRequestPackageSend: want error")
			}
		})
	}
}

func sameItemRequests(a, b []ItemRequest) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
