package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The expected gains below are the reference's party kill arithmetic
// (the party branch of Monster.calculateRewards, Party.distributeXpAndSp
// with the stock "level" cutoff of 20 and x1 party rates), produced by the
// same Java probe as the model package's party reward vectors.

// partyRewardMonster is a level-40 monster worth 5000 exp and 250 SP, so a
// level-40 party takes it without the level falloff.
func partyRewardMonster() *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 40, HPMax: 2000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		RewardExp: 5000, RewardSp: 250,
	}
}

// rewardParty boots a party of Leader 40, Member 30 and Novice 19, plus
// the partyless Outsider 40, all beside a fresh reward monster.
func rewardParty(t *testing.T, opts ...gameservertest.Option) (*group, *npc.Hostile) {
	t.Helper()
	g := bootGroup(t, []seat{{"Leader", 40}, {"Member", 30}, {"Novice", 19}, {"Outsider", 40}}, opts...)
	g.invite(t, 0, 1, 0)
	g.invite(t, 0, 2, 0)
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	monster := g.srv.SpawnHostileNPCTemplateAt(t, partyRewardMonster(), location.Location{X: x + 60, Y: y + 20, Z: z})
	g.quiet(t)
	return g, monster
}

func (g *group) combatant(t *testing.T, i int) attackable.Combatant {
	t.Helper()
	obj, ok := g.srv.State.Player(g.players[i].id)
	if !ok {
		t.Fatalf("%s missing from the world", g.players[i].name)
	}
	c, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("%s is %T, not a combatant", g.players[i].name, obj)
	}
	return c
}

// earned returns each player's one exp/SP gain message, or ok false when
// it got none.
func (g *group) earned(t *testing.T) []gain {
	t.Helper()
	out := make([]gain, len(g.players))
	for i, p := range g.players {
		for _, f := range drainFrames(t, p.c) {
			if f[0] != serverpackets.OpcodeSystemMessage || int(wire.NewReader(f[1:]).ReadInt32()) != serverpackets.SystemMessageYouEarnedS1ExpAndS2SP {
				continue
			}
			if out[i].ok {
				t.Fatalf("%s got two exp/SP gain messages", p.name)
			}
			r := wire.NewReader(f[5:])
			r.ReadInt32() // params
			r.ReadInt32()
			out[i].exp = r.ReadInt32()
			r.ReadInt32()
			out[i].sp = r.ReadInt32()
			out[i].ok = true
		}
	}
	return out
}

type gain struct {
	exp, sp int32
	ok      bool
}

// TestPartyKillSharesExpAcrossMembers pins a party kill: the leader's
// damage pays every living member in party range by squared level, with
// the two-member party bonus; Novice, 21 levels under the party's level,
// still gets a zero gain; the partyless Outsider, who never fought, gets
// nothing.
func TestPartyKillSharesExpAcrossMembers(t *testing.T) {
	g, monster := rewardParty(t)
	if !monster.TakeDamage(1000, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}
	want := []gain{{4160, 208, true}, {2340, 117, true}, {0, 0, true}, {}}
	if got := g.earned(t); !equalGains(got, want) {
		t.Fatalf("gains = %+v, want %+v", got, want)
	}
}

// TestPartyKillSharedWithOutsider pays a partyless attacker its own share
// while the party splits the rest: the party's 60% is taken off the
// reward twice before the bonus, and a member that did not fight shares
// it as much as the one that did.
func TestPartyKillSharedWithOutsider(t *testing.T) {
	g, monster := rewardParty(t)
	monster.TakeDamage(400, g.combatant(t, 3))
	if !monster.TakeDamage(600, g.combatant(t, 1)) {
		t.Fatal("member's hit did not kill the monster")
	}
	want := []gain{{1498, 74, true}, {842, 42, true}, {0, 0, true}, {2000, 100, true}}
	if got := g.earned(t); !equalGains(got, want) {
		t.Fatalf("gains = %+v, want %+v", got, want)
	}
}

// TestKillRewardsApplyServerRates pins RateXp and RateSp on the same kill:
// the monster's 5000 exp and 250 SP become 500000 and 312 (312.5 narrowed
// to an int) before the partyless Outsider takes its 40% and the party
// splits its 60% pool. Expected gains follow the reference's
// Npc.getExpReward/getSpReward feeding Monster.calculateExpAndSp, with the
// party arithmetic of TestPartyKillSharedWithOutsider; 312.5 un-narrowed
// would pay the Outsider 125 SP instead of 124.
func TestKillRewardsApplyServerRates(t *testing.T) {
	g, monster := rewardParty(t, gameservertest.WithRateXpSp(100, 1.25))
	monster.TakeDamage(400, g.combatant(t, 3))
	if !monster.TakeDamage(600, g.combatant(t, 1)) {
		t.Fatal("member's hit did not kill the monster")
	}
	want := []gain{{149760, 92, true}, {84240, 52, true}, {0, 0, true}, {200000, 124, true}}
	if got := g.earned(t); !equalGains(got, want) {
		t.Fatalf("gains = %+v, want %+v", got, want)
	}
}

// TestPartyKillOverhitAddsToPool adds the overhitting member's bonus to the
// pool the party splits; only that member is told of the overhit.
func TestPartyKillOverhitAddsToPool(t *testing.T) {
	g, monster := rewardParty(t)
	monster.EnableOverhit()
	if !monster.TakeDamage(int(monster.CurrentHP())*2, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}
	frames := make([][][]byte, len(g.players))
	for i, p := range g.players {
		frames[i] = drainFrames(t, p.c)
	}
	for i, f := range frames {
		got := hasStaticMessage(f, serverpackets.SystemMessageOverHit)
		if got != (i == 0) {
			t.Fatalf("%s OVER_HIT = %v, want %v", g.players[i].name, got, i == 0)
		}
	}
	// (5000 + 1250) * 1.30 split 1600:900; SP has no overhit.
	if exp, sp := gainIn(t, frames[0]); exp != 5200 || sp != 208 {
		t.Fatalf("leader gained %d/%d, want 5200/208", exp, sp)
	}
	if exp, sp := gainIn(t, frames[1]); exp != 2925 || sp != 117 {
		t.Fatalf("member gained %d/%d, want 2925/117", exp, sp)
	}
}

// TestPartyKillSkipsMemberOutOfRange leaves a member past the party range
// out of the split altogether: no gain, and no part in the bonus or the
// squared-level sum, though it is the party's highest level.
func TestPartyKillSkipsMemberOutOfRange(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 40}, {"Member", 30}, {"Far", 45}})
	g.invite(t, 0, 1, 0)
	g.invite(t, 0, 2, 0)
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	obj, _ := g.srv.State.Player(g.players[2].id)
	far, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("Far is %T", obj)
	}
	far.TeleportTo(x+3000, y, z, 0)
	monster := g.srv.SpawnHostileNPCTemplateAt(t, partyRewardMonster(), location.Location{X: x + 60, Y: y + 20, Z: z})
	g.quiet(t)

	if !monster.TakeDamage(1000, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}
	want := []gain{{4160, 208, true}, {2340, 117, true}, {}}
	if got := g.earned(t); !equalGains(got, want) {
		t.Fatalf("gains = %+v, want %+v", got, want)
	}
}

func equalGains(got, want []gain) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasStaticMessage(frames [][]byte, id int) bool {
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && int(wire.NewReader(f[1:]).ReadInt32()) == id {
			return true
		}
	}
	return false
}

func gainIn(t *testing.T, frames [][]byte) (exp, sp int32) {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage || int(wire.NewReader(f[1:]).ReadInt32()) != serverpackets.SystemMessageYouEarnedS1ExpAndS2SP {
			continue
		}
		r := wire.NewReader(f[5:])
		r.ReadInt32()
		r.ReadInt32()
		exp = r.ReadInt32()
		r.ReadInt32()
		return exp, r.ReadInt32()
	}
	t.Fatal("no exp/SP gain message")
	return 0, 0
}
