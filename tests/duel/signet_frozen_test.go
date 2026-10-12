package duel

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFrozenDuellistsSignetPointRefusesActions pins the duel check of a
// click on a signet effect point, which acts for the player who placed it
// (EffectPoint.getActingPlayer): once that player lost its duel, the click
// is refused as the other party frozen, like a click on the player itself.
func TestFrozenDuellistsSignetPointRefusesActions(t *testing.T) {
	t.Parallel()
	def := modelskill.Definition{
		ID: 454, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "SIGNET", EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 600}},
	}
	db := sqltest.SharedDB(t)
	opts := []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
			ID: 13018, TemplateID: 13018, Type: "EffectPoint", CollisionRadius: 5, RunSpeed: 20, WalkSpeed: 50,
		}})),
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{def}), gamesql.NewCharacterSkillStore(db))),
	}
	a := bootArenaWith(t, opts, func(a *arena) {
		if err := a.srv.KnownSkills.SetKnownSkill(context.Background(), a.players[1].id, 0, 454, 1); err != nil {
			t.Fatalf("seed signet: %v", err)
		}
	}, "Challenger", "Rival", "Bystander")
	if !a.srv.DrivesClock() {
		t.Skip("holding the duel between the defeat and its end needs the driven clock")
	}
	a.challenge(t, 0, 1)
	a.players[1].c.Send(encodeCastSignet())
	var point *npc.EffectPoint
	a.srv.AdvanceUntil(t, "signet effect point", func() bool {
		for _, obj := range a.srv.State.Objects() {
			if ep, ok := obj.(*npc.EffectPoint); ok {
				point = ep
			}
		}
		return point != nil
	})
	a.quiet(t)

	a.players[1].c.Send(encodeDuelSurrender())
	a.srv.Settle(t)
	if got := a.standing(t, 1).DuelState(); got != duel.Dead {
		t.Fatalf("rival state after the surrender = %d, want Dead", got)
	}
	a.players[2].c.Send(encodeAction(point.ObjectID()))
	frames := a.srv.ReadQueued(t, a.players[2].c)
	at := requireMessage(t, frames, 0, serverpackets.SystemMessageOtherPartyIsFrozen)
	if indexOf(frames, at, serverpackets.OpcodeActionFailed) < 0 {
		t.Fatal("frozen duellist's signet point refusal sent no ActionFailed")
	}
}

// encodeCastSignet is a RequestMagicSkillUse of the signet 454.
func encodeCastSignet() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(454)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}
