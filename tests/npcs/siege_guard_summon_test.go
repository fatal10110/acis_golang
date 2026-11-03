package npcs

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The servitor both siege players summon.
const (
	siegeServitorSkillID = 1111
	siegeServitorNPCID   = 12077
)

// siegeServitorOptions are the servitor template and the summon skill that
// calls it up.
func siegeServitorOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := modelskill.NewTable([]modelskill.Definition{{
		ID: siegeServitorSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: siegeServitorNPCID, SummonTotalLifeTime: 1_200_000,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}})
	return []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
			ID: siegeServitorNPCID, TemplateID: siegeServitorNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
			HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, BaseAttackRange: 40,
			CollisionRadius: 8, CollisionHeight: 20, CorpseTime: 7,
		}})),
	}
}

// summonServitor has c, playing ownerID, cast the servitor summon and
// returns the servitor once it is in the world.
func summonServitor(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, ownerID int32) *summon.Actor {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(siegeServitorSkillID))
	var servitor *summon.Actor
	srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := srv.State.Summon(ownerID)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	return servitor
}

// TestSiegeGuardAndSummonsDuringItsSiege pins a siege guard against the
// summons of both siege sides while its castle's siege runs: the guard is
// attacked by whoever acts for the summon (SiegeGuard.isAttackableBy reads
// attacker.getActingPlayer(), SiegeGuard.java:22-38), so the attacker's
// servitor may hit it and the defender's may not; and the guard may hit a
// summon unless the summon's owner's clan owns or defends the siege running
// where the summon stands (Playable.isAttackableBy, Playable.java:457-466),
// so it may hit the attacker's servitor and not the defender's.
func TestSiegeGuardAndSummonsDuringItsSiege(t *testing.T) {
	t.Parallel()
	w := bootBesiegedWorldPrepared(t, true, 0, func(srv *gameservertest.Server, talkerID int32) {
		for _, id := range []int32{talkerID, besiegedKeeperID} {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), id, 0, siegeServitorSkillID, 1); err != nil {
				t.Fatalf("seed known skill: %v", err)
			}
		}
	}, siegeServitorOptions(t)...)
	attackerServitor := summonServitor(t, w.srv, w.talker, w.talkerID)
	defenderServitor := summonServitor(t, w.srv, w.keeper, w.keeperID)
	guard := w.srv.SpawnHostileNPCTemplateAt(t, sorcererTemplate(), location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.talker)
	drainUntilQuiet(t, w.keeper)

	if !guard.AttackableBy(attackerServitor) {
		t.Fatal("guard not attackable by the attacker's servitor")
	}
	if !guard.AttackableWithoutForceBy(attackerServitor) {
		t.Fatal("guard needs a forced attack from the attacker's servitor")
	}
	if guard.AttackableBy(defenderServitor) {
		t.Fatal("guard attackable by the defender's servitor")
	}
	if !attackerServitor.AttackableBy(guard) {
		t.Fatal("guard may not attack the attacker's servitor")
	}
	if defenderServitor.AttackableBy(guard) {
		t.Fatal("guard may attack the defender's servitor")
	}
}
