package character

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func TestValidatePositionFallDamage(t *testing.T) {
	srv, character, objID := bootInZones(t, zone.NewIndex())
	x, y, z := srv.PlayerPosition(t, objID)
	hp := character.HP()
	if hp != 35 {
		t.Fatalf("fixture player HP = %v, want 35", hp)
	}
	if maxHP := character.MaxHPValue(); maxHP < 36.799 || maxHP > 36.801 {
		t.Fatalf("fixture maximum HP = %v, want 36.8", maxHP)
	}

	// The male human fighter's safe fall height is 250. The reference
	// comparator is strict: equality does not start a fall.
	for _, drop := range []int{249, 250} {
		if frames := validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - drop}); len(frames) != 0 {
			t.Fatalf("drop %d replies = %x, want none", drop, frames)
		}
		if got := character.HP(); got != hp {
			t.Fatalf("drop %d HP = %v, want %v", drop, got, hp)
		}
	}

	// Java getMaxHp truncates 36.8 to 36 before calcFallDam:
	// (int)(251 * 36 / 1000) = 9.
	frames := validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - 251})
	if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeStatusUpdate || frames[1][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("fall replies = %x, want StatusUpdate then SystemMessage", frames)
	}
	wantMessage := []byte{
		serverpackets.OpcodeSystemMessage,
		0x28, 0x01, 0, 0, // FALL_DAMAGE_S1 (296)
		1, 0, 0, 0, // one parameter
		1, 0, 0, 0, // number
		9, 0, 0, 0,
	}
	if countOpcode(frames, serverpackets.OpcodeSystemMessage) != 1 ||
		!bytes.Equal(frames[firstOpcode(frames, serverpackets.OpcodeSystemMessage)], wantMessage) {
		t.Fatalf("fall replies = %x, want one damage message %x", frames, wantMessage)
	}
	if countOpcode(frames, serverpackets.OpcodeValidateLocation) != 0 {
		t.Fatalf("fall replies = %x, want no position correction", frames)
	}
	if got := character.HP(); got != hp-9 {
		t.Fatalf("HP after fall = %v, want %v", got, hp-9)
	}

	// During the 10-second fall window, even horizontal desync is ignored.
	frames = validatePositionReplies(t, srv.Client, location.Location{X: x + 1_000, Y: y, Z: z - 2_000})
	if len(frames) != 0 || character.HP() != hp-9 {
		t.Fatalf("fall-window replies = %x, HP = %v", frames, character.HP())
	}
}

func TestValidatePositionFemaleSafeFallHeight(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacterSex("Newbie", 5, 0, player.SexFemale),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zone.NewIndex()),
	)
	enterWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, objID)
	hp := srv.PlayerCurrentHP(t, objID)

	// The human fighter's female threshold is 270, versus 250 for males.
	for _, drop := range []int{251, 270} {
		if frames := validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - drop}); len(frames) != 0 {
			t.Fatalf("female drop %d replies = %x, want none", drop, frames)
		}
		if got := srv.PlayerCurrentHP(t, objID); got != hp {
			t.Fatalf("female drop %d HP = %d, want %d", drop, got, hp)
		}
	}

	frames := validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - 271})
	if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeStatusUpdate || frames[1][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("female drop 271 replies = %x, want status then fall message", frames)
	}
	if got := srv.PlayerCurrentHP(t, objID); got != hp-9 {
		t.Fatalf("female drop 271 HP = %d, want %d", got, hp-9)
	}
}

func TestValidatePositionFallCannotKillAndDeadPlayerDoesNotFall(t *testing.T) {
	srv, character, objID := bootInZones(t, zone.NewIndex())
	x, y, z := srv.PlayerPosition(t, objID)
	frames := validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - 10_000})
	wantMessage := []byte{
		serverpackets.OpcodeSystemMessage,
		0x28, 0x01, 0, 0, // FALL_DAMAGE_S1 (296)
		1, 0, 0, 0,
		1, 0, 0, 0,
		0x68, 0x01, 0, 0, // 360 damage, before the HP floor
	}
	if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeStatusUpdate || !bytes.Equal(frames[1], wantMessage) {
		t.Fatalf("large fall replies = %x, want status then %x", frames, wantMessage)
	}
	if got := character.HP(); got != 1 || character.Dead() {
		t.Fatalf("large fall HP = %v, dead = %v; want 1 HP alive", got, character.Dead())
	}

	character.MarkDead()
	frames = validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - 10_000})
	if len(frames) != 0 || character.HP() != 0 {
		t.Fatalf("dead-player replies = %x, HP = %v", frames, character.HP())
	}
}

func TestValidatePositionFallDamageDisabledStillOpensWindow(t *testing.T) {
	srv, character, objID := bootInZones(t, zone.NewIndex(), gameservertest.WithFallingDamage(false))
	x, y, z := srv.PlayerPosition(t, objID)
	hp := character.HP()
	if frames := validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - 251}); len(frames) != 0 {
		t.Fatalf("disabled fall replies = %x, want none", frames)
	}
	if frames := validatePositionReplies(t, srv.Client, location.Location{X: x + 1_000, Y: y, Z: z - 251}); len(frames) != 0 {
		t.Fatalf("disabled fall-window replies = %x, want none", frames)
	}
	if got := character.HP(); got != hp {
		t.Fatalf("disabled fall HP = %v, want %v", got, hp)
	}
}

func TestValidatePositionFallDamageIntegerOverflow(t *testing.T) {
	srv, character, objID := bootInZones(t, zone.NewIndex())
	x, y, z := srv.PlayerPosition(t, objID)
	hp := character.HP()
	// Java's 32-bit product wraps at this height: 59,652,324 * 36
	// becomes -2,147,483,632 before division, so no damage is sent.
	frames := validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - 59_652_324})
	if len(frames) != 0 || character.HP() != hp {
		t.Fatalf("overflow fall replies = %x, HP = %v, want no damage", frames, character.HP())
	}
}
