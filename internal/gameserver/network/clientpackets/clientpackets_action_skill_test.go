package clientpackets

import (
	"encoding/hex"
	"testing"
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
