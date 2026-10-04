package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ownerQueuedSkill is a self-targeted skill the owner queues behind its
// swing.
const ownerQueuedSkill = 3

// bootWolfStrikerOwnerCaster is bootWolfStrikerWithBystander with an owner
// who also knows ownerQueuedSkill.
func bootWolfStrikerOwnerCaster(t *testing.T) (*petWorld, *summon.Actor, int32) {
	t.Helper()
	wolf := wolfTemplate()
	wolf.Skills = map[int]int{wolfStrikeSkill: 1}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		wolfStrike(),
		{
			ID: ownerQueuedSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "DUMMY",
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
		gameservertest.WithSkills(skills),
		gameservertest.WithAITask(),
	)
	if !srv.DrivesClock() {
		t.Skip("holding the owner's swing open needs the driven clock")
	}
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, ownerQueuedSkill, 1); err != nil {
		t.Fatalf("seed known skill %d: %v", ownerQueuedSkill, err)
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	petActor, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)

	bystanderID := srv.SeedCharacterFor(t, "player2", "Bystander", 5, 0).ID
	bystander := srv.DialClient(t, "player2", 1)
	startInWorld(t, bystander)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, bystander)
	return h, petActor, bystanderID
}

// TestSummonCtrlStrikeOnPlayerTheOwnerAttacksWithCastQueued commands the
// wolf's CTRL strike at an unflagged player the owner force-attacks, after
// the owner queued a skill behind its swing. The queued skill is only the
// next intention (PlayableAI.tryToCast, PlayableAI.java:312-318): the
// owner's current intention stays ATTACK on that player until the swing
// ends, so the player is still the owner's main target
// (Playable.java:405-412, 443) and the strike starts. The queued skill then
// starts once the swing is over.
func TestSummonCtrlStrikeOnPlayerTheOwnerAttacksWithCastQueued(t *testing.T) {
	t.Parallel()
	h, petActor, playerID := bootWolfStrikerOwnerCaster(t)
	h.targetPlayer(t, playerID)
	x, y, z := h.srv.PlayerPosition(t, playerID)
	h.client.Send(encodeAttackRequest(playerID, int32(x), int32(y), int32(z), false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeAttack, "owner's swing")
	if attacker := wire.NewReader(frames[len(frames)-1][1:]).ReadInt32(); attacker != h.ownerID {
		t.Fatalf("first Attack by %d, want the owner %d", attacker, h.ownerID)
	}

	h.client.Send(encodeRequestMagicSkillUse(ownerQueuedSkill))
	readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "queued skill ActionFailed")
	if h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("the owner's skill started mid-swing")
	}

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, true))
	requireSummonStrikeStarted(t, readUntilStrikeAnswer(t, h, petActor), petActor, playerID)

	h.srv.AdvanceUntil(t, "the owner's queued skill", func() bool { return h.srv.PlayerCastingNow(t, h.ownerID) })
}
