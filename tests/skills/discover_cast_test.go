package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestEnteringPlayerSeesCastInFlight pins what a player coming to know a
// casting player is shown: CharInfo, then right after it the cast's
// MagicSkillUse from the caster where it stands to its target, carrying the
// skill's own hit time and reuse delay rather than the ones the cast was
// timed with, so the observer sees the cast animation the caster is in.
func TestEnteringPlayerSeesCastInFlight(t *testing.T) {
	t.Parallel()
	const skillID = 1204
	// A long hit time keeps the cast in flight through the observer's entry
	// on any clock; neither timing is static, so the cast's own plan scales
	// them by the caster's speed and reuse rate.
	def := modelskill.Definition{
		ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 20_000, ReuseDelay: 45_000, SkillType: "DUMMY",
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Caster", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	caster, casterID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, casterID, skillID, 1)
	startInWorld(t, caster)

	caster.Send(encodeRequestMagicSkillUse(skillID, false, false))
	assertFrameOpcode(t, caster.Read(), serverpackets.OpcodeMagicSkillUse, "caster cast start")
	drainUntilQuiet(t, caster)

	srv.SeedCharacterFor(t, "player2", "Arriving", 5, 0)
	observer := srv.DialClient(t, "player2", 1)
	observer.Send(encodeRequestGameStart(0))
	for frame := observer.Read(); frame[0] != serverpackets.OpcodeCharSelected; frame = observer.Read() {
	}
	observer.Send(encodeEnterWorld())
	var frames [][]byte
	for frame := observer.ReadWithTimeout(500 * time.Millisecond); frame != nil; frame = observer.ReadWithTimeout(500 * time.Millisecond) {
		frames = append(frames, frame)
	}

	var next []byte
	for i, frame := range frames {
		if frame[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		r := wireReader(frame[1:])
		r.ReadInt32() // x
		r.ReadInt32() // y
		r.ReadInt32() // z
		r.ReadInt32() // heading
		if r.ReadInt32() == casterID && i+1 < len(frames) {
			next = frames[i+1]
		}
	}
	if next == nil {
		t.Fatal("caster's CharInfo not followed by any frame, want MagicSkillUse")
	}
	assertFrameOpcode(t, next, serverpackets.OpcodeMagicSkillUse, "frame after caster CharInfo")
	r := wireReader(next[1:])
	gotCaster, gotTarget, gotSkill, gotLevel := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	gotHit, gotReuse := r.ReadInt32(), r.ReadInt32()
	cx, cy, cz := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	crit := r.ReadInt32()
	tx, ty, tz := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if gotCaster != casterID || gotTarget != casterID || gotSkill != skillID || gotLevel != 1 {
		t.Fatalf("MagicSkillUse ids = caster %d target %d skill %d level %d, want %d/%d/%d/1",
			gotCaster, gotTarget, gotSkill, gotLevel, casterID, casterID, skillID)
	}
	if gotHit != 20_000 || gotReuse != 45_000 {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want the skill's own 20000/45000", gotHit, gotReuse)
	}
	x, y, z := srv.PlayerPosition(t, casterID)
	if int(cx) != x || int(cy) != y || int(cz) != z || tx != cx || ty != cy || tz != cz {
		t.Fatalf("MagicSkillUse caster at (%d,%d,%d) target at (%d,%d,%d), want both at the caster's (%d,%d,%d)",
			cx, cy, cz, tx, ty, tz, x, y, z)
	}
	if crit != 0 {
		t.Fatalf("MagicSkillUse critical flag = %d, want 0", crit)
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("MagicSkillUse layout: err %v, %d trailing bytes", err, r.Remaining())
	}
}
