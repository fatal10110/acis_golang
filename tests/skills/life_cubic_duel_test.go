package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLifeCubicInOneOnOneDuelHealsOnlyOwner: Cubic.pickFriendlyTarget
// (Cubic.java:214) treats an owner in a duel that is not a party duel as
// partyless, so the Life Cubic heals its wounded owner and passes over a
// party member hurt worse, whom it would pick outside the duel.
func TestLifeCubicInOneOnOneDuelHealsOnlyOwner(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Owner", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: summonLifeCubicSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Life),
				CubicActivationTime: lifeCubicInterval, SummonTotalLifeTime: 900_000,
				StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
			},
			{ID: lifeCubicHealSkill, Level: 1, Power: lifeCubicHealPower},
		})),
	)
	c, ownerID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, ownerID, summonLifeCubicSkill, 1)
	startInWorld(t, c)
	memberID := srv.SeedCharacterFor(t, "player2", "Member", 20, 0).ID
	member := srv.DialClient(t, "player2", 1)
	startInWorldAmongPlayers(t, member)
	srv.SeedCharacterFor(t, "player3", "Rival", 20, 0)
	rival := srv.DialClient(t, "player3", 1)
	startInWorldAmongPlayers(t, rival)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, member)

	c.Send(encodePartyInvite("Member"))
	drainUntilQuiet(t, c)
	member.Send(encodePartyAnswer(1))
	drainUntilQuiet(t, member)
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(ownerID)
	if !ok {
		t.Fatalf("world player %d missing", ownerID)
	}
	owner, ok := obj.(interface {
		SetRollSource(func(int) int)
		InDuel() bool
		DuelState() duel.State
	})
	if !ok {
		t.Fatalf("world player %d = %T, want a roll source and a duel standing", ownerID, obj)
	}
	owner.SetRollSource(func(int) int { return 0 })

	c.Send(encodeDuelChallenge("Rival"))
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, rival)
	rival.Send(encodeDuelAccept())
	srv.AdvanceUntil(t, "the duel starting", func() bool { return owner.DuelState() == duel.Duelling })
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, member)
	drainUntilQuiet(t, rival)

	ownerBefore, _ := damageToHealHeadroom(t, srv, ownerID, 2*lifeCubicHealPower)
	memberBefore, _ := damageToHealHeadroom(t, srv, memberID, 3*lifeCubicHealPower)

	c.Send(encodeRequestMagicSkillUse(summonLifeCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)

	want := ownerBefore + lifeCubicHealPower
	srv.AdvanceUntil(t, "the Life Cubic healing its owner", func() bool { return srv.PlayerCurrentHP(t, ownerID) == want })
	if hp := srv.PlayerCurrentHP(t, memberID); hp != memberBefore {
		t.Fatalf("party member HP = %d, want untouched %d: the cubic healed past a one-on-one duel", hp, memberBefore)
	}
	if !owner.InDuel() {
		t.Fatal("the owner left the duel before the heal; the gate was not exercised")
	}
}

func encodeDuelChallenge(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestDuelStart)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeDuelAccept() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestDuelAnswerStart)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(1)
	return w.Bytes()
}
