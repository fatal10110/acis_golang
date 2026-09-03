package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const corpsePetSkillID int32 = 2179

func corpsePetSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(corpsePetSkillID), Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpsePet, SkillType: "RESURRECT",
		CastRange: 400, HitTime: 500, ReuseDelay: 60_000,
		StaticHitTime: true, StaticReuse: true, Power: 100,
	}
}

func bootCorpsePetCaster(t *testing.T) *gameservertest.Server {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{corpsePetSkill()})),
	)
	seedKnownSkill(t, srv, srv.SoleObjectID(t), int(corpsePetSkillID), 1)
	return srv
}

// TestCorpsePetCastRejectsLivingAndDeadNonPet drives RequestMagicSkillUse
// for Blessed Scroll of Resurrection: Pet (skill 2179): a living target
// gets INVALID_TARGET, and a dead non-pet gets S1_CANNOT_BE_USED carrying
// that skill's name, then the actor's MoveToPawn rotation.
func TestCorpsePetCastRejectsLivingAndDeadNonPet(t *testing.T) {
	t.Run("living target", func(t *testing.T) {
		srv := bootCorpsePetCaster(t)
		c := srv.Client
		startInWorld(t, c)
		guard := srv.SpawnHostileNPC(t)
		drainUntilQuiet(t, c)
		targetHostile(t, c, guard.ObjectID())

		c.Send(encodeRequestMagicSkillUse(corpsePetSkillID, false, false))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToPawn, "living corpse-pet rotation")
	})

	t.Run("dead non-pet", func(t *testing.T) {
		srv := bootCorpsePetCaster(t)
		healer := srv.Client
		patientID := srv.SeedCharacterFor(t, "player2", "Patient", 5, 0).ID
		patient := srv.DialClient(t, "player2", 1)

		startInWorld(t, healer)
		startInWorldAmongPlayers(t, patient)
		srv.MarkPlayerDead(t, patientID)
		drainUntilQuiet(t, healer)
		drainUntilQuiet(t, patient)

		x, y, z := srv.PlayerPosition(t, patientID)
		healer.Send(encodeAction(patientID, int32(x), int32(y), int32(z), false))
		drainUntilQuiet(t, healer)
		drainUntilQuiet(t, patient)

		healer.Send(encodeRequestMagicSkillUse(corpsePetSkillID, false, false))
		assertSystemMessageSkillFrame(t, healer.Read(), serverpackets.SystemMessageS1CannotBeUsed, corpsePetSkillID, 1)
		assertFrameOpcode(t, healer.Read(), serverpackets.OpcodeMoveToPawn, "dead non-pet corpse-pet rotation")
	})
}
