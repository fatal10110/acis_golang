package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const corpsePlayerSkillID int32 = 1016

func corpsePlayerSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(corpsePlayerSkillID), Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT",
		CastRange: 400, HitTime: 500, ReuseDelay: 60_000,
		StaticHitTime: true, StaticReuse: true, Power: 100,
	}
}

func bootCorpsePlayerCaster(t *testing.T, def modelskill.Definition) *gameservertest.Server {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	seedKnownSkill(t, srv, srv.SoleObjectID(t), int(corpsePlayerSkillID), 1)
	return srv
}

func TestCorpsePlayerCastRejections(t *testing.T) {
	t.Parallel()
	t.Run("living target", func(t *testing.T) {
		srv := bootCorpsePlayerCaster(t, corpsePlayerSkill())
		caster := srv.Client
		patientID := srv.SeedCharacterFor(t, "player2", "Patient", 5, 0).ID
		patient := srv.DialClient(t, "player2", 1)

		startInWorld(t, caster)
		startInWorldAmongPlayers(t, patient)
		drainUntilQuiet(t, caster)
		drainUntilQuiet(t, patient)

		x, y, z := srv.PlayerPosition(t, patientID)
		caster.Send(encodeAction(patientID, int32(x), int32(y), int32(z), false))
		drainUntilQuiet(t, caster)
		drainUntilQuiet(t, patient)
		headingBefore := playerHeading(t, srv, srv.SoleObjectID(t))

		// A player's failed target condition answers with its reason alone:
		// no heading toward the target and no MoveToPawn to anyone.
		caster.Send(encodeRequestMagicSkillUse(corpsePlayerSkillID, false, false))
		assertStaticSystemMessage(t, caster.Read(), serverpackets.SystemMessageInvalidTarget)
		if extra := caster.ReadWithTimeout(300 * time.Millisecond); extra != nil {
			t.Fatalf("living corpse-player rejection extra caster frame = %#x, want none", extra[0])
		}
		if extra := patient.ReadWithTimeout(300 * time.Millisecond); extra != nil {
			t.Fatalf("living corpse-player rejection observer frame = %#x, want none", extra[0])
		}
		if got := playerHeading(t, srv, srv.SoleObjectID(t)); got != headingBefore {
			t.Fatalf("caster heading after rejected cast = %d, want unchanged %d", got, headingBefore)
		}
	})

	t.Run("low MP wins over living target", func(t *testing.T) {
		def := corpsePlayerSkill()
		def.MPInitialConsume = 12
		def.MPConsume = 99_999
		srv := bootCorpsePlayerCaster(t, def)
		caster := srv.Client
		patientID := srv.SeedCharacterFor(t, "player2", "Patient", 5, 0).ID
		patient := srv.DialClient(t, "player2", 1)

		startInWorld(t, caster)
		startInWorldAmongPlayers(t, patient)
		drainUntilQuiet(t, caster)
		drainUntilQuiet(t, patient)

		x, y, z := srv.PlayerPosition(t, patientID)
		caster.Send(encodeAction(patientID, int32(x), int32(y), int32(z), false))
		drainUntilQuiet(t, caster)
		drainUntilQuiet(t, patient)

		caster.Send(encodeRequestMagicSkillUse(corpsePlayerSkillID, false, false))
		assertStaticSystemMessage(t, caster.Read(), serverpackets.SystemMessageNotEnoughMP)
		if extra := caster.ReadWithTimeout(300 * time.Millisecond); extra != nil {
			t.Fatalf("low MP corpse-player rejection extra frame = %#x, want no ActionFailed or MoveToPawn", extra[0])
		}
		if extra := patient.ReadWithTimeout(300 * time.Millisecond); extra != nil {
			t.Fatalf("low MP corpse-player rejection observer frame = %#x, want none", extra[0])
		}
	})

	t.Run("dead non-playable", func(t *testing.T) {
		srv := bootCorpsePlayerCaster(t, corpsePlayerSkill())
		c := srv.Client
		startInWorld(t, c)
		guard := srv.SpawnHostileNPC(t)
		drainUntilQuiet(t, c)
		targetHostile(t, c, guard.ObjectID())
		guard.MarkDead()
		drainUntilQuiet(t, c)

		c.Send(encodeRequestMagicSkillUse(corpsePlayerSkillID, false, false))
		assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, corpsePlayerSkillID, 1)
		if extra := c.ReadWithTimeout(300 * time.Millisecond); extra != nil {
			t.Fatalf("dead non-playable corpse-player rejection extra frame = %#x, want no ActionFailed or MoveToPawn", extra[0])
		}
	})

	t.Run("dead playable starts cast", func(t *testing.T) {
		srv := bootCorpsePlayerCaster(t, corpsePlayerSkill())
		caster := srv.Client
		patientID := srv.SeedCharacterFor(t, "player2", "Patient", 5, 0).ID
		patient := srv.DialClient(t, "player2", 1)

		startInWorld(t, caster)
		startInWorldAmongPlayers(t, patient)
		srv.MarkPlayerDead(t, patientID)
		drainUntilQuiet(t, caster)
		drainUntilQuiet(t, patient)

		x, y, z := srv.PlayerPosition(t, patientID)
		caster.Send(encodeAction(patientID, int32(x), int32(y), int32(z), false))
		drainUntilQuiet(t, caster)
		drainUntilQuiet(t, patient)

		caster.Send(encodeRequestMagicSkillUse(corpsePlayerSkillID, false, false))
		readCastStartFrames(t, caster, srv.SoleObjectID(t), corpsePlayerSkillID, 1, 500, 60_000, patientID)
	})
}
