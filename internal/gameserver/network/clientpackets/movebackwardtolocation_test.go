package clientpackets

import (
	"encoding/hex"
	"testing"
)

func TestDecodeMoveBackwardToLocation(t *testing.T) {
	payload, err := hex.DecodeString("0150b4000015a1000032f2ffff25b400001fa1000034f2ffff01000000")
	if err != nil {
		t.Fatalf("decode test payload: %v", err)
	}

	got, err := DecodeMoveBackwardToLocation(payload)
	if err != nil {
		t.Fatalf("DecodeMoveBackwardToLocation: %v", err)
	}

	want := MoveBackwardToLocation{
		TargetX:      46160,
		TargetY:      41237,
		TargetZ:      -3534,
		OriginX:      46117,
		OriginY:      41247,
		OriginZ:      -3532,
		MoveMovement: 1,
	}
	if got != want {
		t.Fatalf("DecodeMoveBackwardToLocation = %+v, want %+v", got, want)
	}
}

func TestDecodeMoveBackwardToLocation_Short(t *testing.T) {
	if _, err := DecodeMoveBackwardToLocation([]byte{OpcodeMoveBackwardToLocation, 1, 2}); err == nil {
		t.Fatal("DecodeMoveBackwardToLocation: want error on short payload")
	}
}
