package quest

import (
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The desire, hate and AI-state operations of the NPC handle (#3748), as
// the boss, raid and faction behaviors call them: Npc.removeAllDesire,
// AggroList.cleanAllHate and remove, HateList.cleanAllHate, stopHate and
// addDefaultHateInfo, the desire queue's clear of one kind,
// Npc.disableCoreAi and Npc.isAISleeping.

// hateOf returns a's attack hate and hate-list hate of b, and how many
// creatures each table lists.
func hateOf(a, b *npc.Hostile) (threat, hate float64, threats, hates int) {
	return a.AI().Threats().Hate(b), a.AI().Hates().Hate(b), len(a.AI().Threats().Snapshot()), a.AI().Hates().Len()
}

func TestNPCHateAndDesireOps(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	x, y, z := srv.PlayerPosition(t, objID)
	a := srv.SpawnHostileNPCAt(t, location.Location{X: x + 40, Y: y, Z: z})
	b := srv.SpawnHostileNPCAt(t, location.Location{X: x - 40, Y: y, Z: z})
	far := srv.SpawnHostileNPCAt(t, location.Location{X: x + 30000, Y: y, Z: z})
	na, nb := script.NPCOf(a), script.NPCOf(b)
	desires := a.AI().Desires()

	// seed gives a an attack desire on b with attack hate, hate-list hate of
	// 50, a cast desire on b and a wander desire.
	seed := func() {
		na.AddAttackDesire(nb, 100)
		na.HateList().Add(nb, 50)
		desires.AddOrUpdate(&ai.Desire{Kind: ai.IntentionCast, FinalTarget: b, Weight: 10})
		na.AddWanderDesire(5, 20)
	}

	run(t, srv, objID, func(_ *script.Script, _ *script.Player) {
		seed()
		if threat, hate, _, _ := hateOf(a, b); threat <= 0 || hate != 50 {
			t.Errorf("seeded hate: attack %v, list %v; want > 0 and 50", threat, hate)
		}

		// AggroList.cleanAllHate: attack hate zeroed, b still listed, the
		// attack desires gone; the hate list and the other desires stay.
		na.CleanAllHate()
		if threat, hate, threats, _ := hateOf(a, b); threat != 0 || threats != 1 || hate != 50 {
			t.Errorf("after CleanAllHate: attack %v (%d listed), list %v; want 0 (1 listed), 50", threat, threats, hate)
		}
		if desires.HasKind(ai.IntentionAttack) || !desires.HasKind(ai.IntentionCast) || !desires.HasKind(ai.IntentionWander) {
			t.Errorf("after CleanAllHate: attack %v cast %v wander %v; want false true true",
				desires.HasKind(ai.IntentionAttack), desires.HasKind(ai.IntentionCast), desires.HasKind(ai.IntentionWander))
		}

		// HateList.cleanAllHate: list hate zeroed with b still listed, the
		// cast desire on b gone.
		na.HateList().CleanAll()
		if _, hate, _, hates := hateOf(a, b); hate != 0 || hates != 1 {
			t.Errorf("after CleanAll: list %v (%d listed); want 0 (1 listed)", hate, hates)
		}
		if desires.HasKind(ai.IntentionCast) || !desires.HasKind(ai.IntentionWander) {
			t.Errorf("after CleanAll: cast %v wander %v; want false true", desires.HasKind(ai.IntentionCast), desires.HasKind(ai.IntentionWander))
		}

		// The desire queue's removeIf on one kind.
		na.RemoveDesires(script.IntentionWander)
		if desires.Len() != 0 {
			t.Errorf("after RemoveDesires(wander): %d desires left, want 0", desires.Len())
		}

		// AggroList.remove and HateList.stopHate drop b's entries.
		na.RemoveAggro(nb)
		na.HateList().StopHate(nb)
		if _, _, threats, hates := hateOf(a, b); threats != 0 || hates != 0 {
			t.Errorf("after RemoveAggro and StopHate: %d attack and %d list entries, want none", threats, hates)
		}

		// addDefaultHateInfo: 300 for the first creature of an NPC in its
		// territory, 100 after that.
		if !a.InTerritory() {
			t.Errorf("an NPC at its spawn point is out of its territory")
		}
		na.HateList().AddDefault(nb)
		first := a.AI().Hates().Hate(b)
		na.HateList().AddDefault(nb)
		if second := a.AI().Hates().Hate(b); first != 300 || second != 400 {
			t.Errorf("default hate: %v then %v, want 300 then 400", first, second)
		}

		// Npc.removeAllDesire: every desire gone, both hates zeroed with b
		// still listed.
		seed()
		na.RemoveAllDesire()
		if threat, hate, threats, hates := hateOf(a, b); threat != 0 || hate != 0 || threats != 1 || hates != 1 {
			t.Errorf("after RemoveAllDesire: attack %v list %v (%d and %d listed); want 0 0 (1 and 1)", threat, hate, threats, hates)
		}
		if desires.Len() != 0 {
			t.Errorf("after RemoveAllDesire: %d desires left, want 0", desires.Len())
		}

		// Npc.disableCoreAi.
		na.DisableCoreAI(true)
		disabled := a.AttackDisabled()
		na.DisableCoreAI(false)
		if !disabled || a.AttackDisabled() {
			t.Errorf("attack disabled with the core AI off %v, back on %v; want true, false", disabled, a.AttackDisabled())
		}

		// Npc.isAISleeping: the player keeps a's region awake; nobody is
		// around far.
		if na.AISleeping() || !script.NPCOf(far).AISleeping() {
			t.Errorf("AI sleeping: near %v, far %v; want false, true", na.AISleeping(), script.NPCOf(far).AISleeping())
		}
	})

	// Every one of these is a hostile NPC's: a civilian NPC panics.
	folk := script.NPCOf(srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 32109), location.Location{X: x, Y: y + 40, Z: z}))
	for name, op := range map[string]func(){
		"RemoveAllDesire":   folk.RemoveAllDesire,
		"CleanAllHate":      folk.CleanAllHate,
		"RemoveAggro":       func() { folk.RemoveAggro(nb) },
		"RemoveDesires":     func() { folk.RemoveDesires(script.IntentionWander) },
		"DisableCoreAI":     func() { folk.DisableCoreAI(true) },
		"BroadcastOnScreen": func() { folk.BroadcastOnScreen(1000, "x") },
	} {
		if srv.RunScript(t, objID, q001, func(*script.Script, *script.Player) { op() }) {
			t.Errorf("%s on a civilian NPC returned, want a panic", name)
		}
	}
}

// screenMessages returns the text and duration of each ExShowScreenMessage
// among frames.
func screenMessages(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		if len(f) < 3 || f[0] != serverpackets.OpcodeExtended || binary.LittleEndian.Uint16(f[1:3]) != serverpackets.OpcodeExShowScreenMessage {
			continue
		}
		r := wire.NewReader(f[3:])
		for range 8 {
			r.ReadInt32()
		}
		duration := r.ReadInt32()
		r.ReadInt32()
		out = append(out, r.ReadString()+"@"+strconv.Itoa(int(duration)))
	}
	return out
}

// Npc.broadcastOnScreen: every player around the NPC sees the text for the
// duration given; a player who does not know the NPC sees nothing.
func TestNPCBroadcastOnScreen(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	x, y, z := srv.PlayerPosition(t, objID)
	near := script.NPCOf(srv.SpawnHostileNPCAt(t, location.Location{X: x + 40, Y: y, Z: z}))
	far := script.NPCOf(srv.SpawnHostileNPCAt(t, location.Location{X: x + 30000, Y: y, Z: z}))
	srv.ReadQueued(t, srv.Client)

	run(t, srv, objID, func(_ *script.Script, _ *script.Player) {
		near.BroadcastOnScreen(3000, script.NpcString(1000519))
		far.BroadcastOnScreen(3000, "far away")
	})
	got := screenMessages(srv.ReadQueued(t, srv.Client))
	want := script.NpcString(1000519) + "@3000"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("screen messages %q, want [%q]", got, want)
	}
}

// Npc.getNeighborHero: a hero within the radius is found; a player who is
// no hero, or a hero out of the radius, is not.
func TestScriptNeighborHero(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	x, y, z := srv.PlayerPosition(t, objID)
	n := script.NPCOf(srv.SpawnHostileNPCAt(t, location.Location{X: x + 400, Y: y, Z: z}))

	var noHero, hero, outOfRange *script.Player
	srv.RunQuest(t, objID, q001, func(_ *script.Quests, c *player.Character, sc *script.Script) {
		noHero = sc.NeighborHero(n, 4000)
		c.SetHero(true)
		hero = sc.NeighborHero(n, 4000)
		outOfRange = sc.NeighborHero(n, 100)
		c.SetHero(false)
	})
	if noHero != nil || outOfRange != nil {
		t.Fatalf("NeighborHero found %v (no hero) and %v (out of range), want none", noHero, outOfRange)
	}
	if hero == nil || hero.ObjectID() != objID {
		t.Fatalf("NeighborHero = %v, want the player %d", hero, objID)
	}
}
