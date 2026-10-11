package combat

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// TestMakeAttackEvent drives the behavior base's makeAttackEvent
// (DefaultNpc lines 242-268) from an invocation of the recording behavior:
// the script's own attacked hook runs first, with no skill; with the party
// call asked for, the master (alive, not the NPC) and every live private of
// the party but the NPC are called, each through every script bound to its
// template. Nothing is called for a lone NPC or without the party call.
func TestMakeAttackEvent(t *testing.T) {
	t.Parallel()
	ids := []int32{fanoutRoles["M"], fanoutRoles["m1"], fanoutRoles["m2"]}
	log := newHookLog()
	srv := bootAttackedHooks(t, log, ids)
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)
	log.name(p.ObjectID(), "p")

	spawnParty := func(t *testing.T) map[string]*npc.Hostile {
		t.Helper()
		roles := map[string]*npc.Hostile{}
		for i, role := range []string{"M", "m1", "m2"} {
			h := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(fanoutRoles[role], nil), location.Location{X: hostileX + 40*i, Y: hostileY, Z: hostileZ})
			roles[role] = h
			log.track(role, h)
		}
		for _, role := range []string{"m1", "m2"} {
			roles[role].SetMaster(roles["M"])
			roles["M"].AddMinion(roles[role])
		}
		return roles
	}
	// raise runs makeAttackEvent on caller as the recorder and returns the
	// hook lines, the party-attacked ones sorted after the first line: the
	// privates of a party come in no particular order.
	raise := func(t *testing.T, caller *npc.Hostile, party bool) []string {
		t.Helper()
		if !srv.RunScript(t, p.ObjectID(), "Recorder", func(sc *script.Script, pl *script.Player) {
			defaultnpc.MakeAttackEvent(sc, script.NPCOf(caller), pl, 50, party)
		}) {
			t.Fatal("makeAttackEvent invocation panicked")
		}
		got, _ := log.take()
		if len(got) > 1 {
			slices.Sort(got[1:])
		}
		return got
	}
	check := func(t *testing.T, got []string, want ...string) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Fatalf("hook calls =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
	}

	t.Run("private", func(t *testing.T) {
		roles := spawnParty(t)
		check(t, raise(t, roles["m1"], true),
			"ATTACKED npc=m1 attacker=p damage=50 skill=none",
			"PARTY_ATTACKED caller=m1 called=M target=p damage=50",
			"PARTY_ATTACKED caller=m1 called=m2 target=p damage=50",
		)
	})
	t.Run("master", func(t *testing.T) {
		roles := spawnParty(t)
		check(t, raise(t, roles["M"], true),
			"ATTACKED npc=M attacker=p damage=50 skill=none",
			"PARTY_ATTACKED caller=M called=m1 target=p damage=50",
			"PARTY_ATTACKED caller=M called=m2 target=p damage=50",
		)
	})
	t.Run("dead members skipped", func(t *testing.T) {
		roles := spawnParty(t)
		roles["M"].MarkDead()
		roles["m2"].MarkDead()
		check(t, raise(t, roles["m1"], true), "ATTACKED npc=m1 attacker=p damage=50 skill=none")
	})
	t.Run("no party call", func(t *testing.T) {
		roles := spawnParty(t)
		check(t, raise(t, roles["m1"], false), "ATTACKED npc=m1 attacker=p damage=50 skill=none")
	})
	t.Run("lone", func(t *testing.T) {
		h := srv.SpawnHostileNPCTemplateAt(t, roleTemplate(fanoutRoles["M"], nil), location.Location{X: hostileX, Y: hostileY + 200, Z: hostileZ})
		log.track("M", h)
		check(t, raise(t, h, true), "ATTACKED npc=M attacker=p damage=50 skill=none")
		if h.IsMaster() {
			t.Fatal("a lone NPC became a master")
		}
	})
}

// TestNPCMinions pins Npc.getMinions on the NPC handle: a master lists its
// privates, a private its master's (itself among them), and a lone NPC
// lists none and is a master from then on.
func TestNPCMinions(t *testing.T) {
	t.Parallel()
	log := newHookLog()
	srv := bootAttackedHooks(t, log, nil)
	startInWorld(t, srv.Client)
	p := liveCombatant(t, srv)

	spawn := func(role string, i int) *npc.Hostile {
		return srv.SpawnHostileNPCTemplateAt(t, roleTemplate(fanoutRoles[role], nil), location.Location{X: hostileX + 40*i, Y: hostileY, Z: hostileZ})
	}
	master, m1, m2, lone := spawn("M", 0), spawn("m1", 1), spawn("m2", 2), spawn("M", 3)
	for _, m := range []*npc.Hostile{m1, m2} {
		m.SetMaster(master)
		master.AddMinion(m)
	}
	ids := func(ns []*script.NPC) []int32 {
		var out []int32
		for _, n := range ns {
			out = append(out, n.ObjectID())
		}
		slices.Sort(out)
		return out
	}
	want := []int32{m1.ObjectID(), m2.ObjectID()}
	slices.Sort(want)
	if !srv.RunScript(t, p.ObjectID(), "Recorder", func(*script.Script, *script.Player) {
		if got := ids(script.NPCOf(master).Minions()); !slices.Equal(got, want) {
			t.Errorf("master's Minions() = %v, want %v", got, want)
		}
		if got := ids(script.NPCOf(m1).Minions()); !slices.Equal(got, want) {
			t.Errorf("private's Minions() = %v, want its master's %v", got, want)
		}
		n := script.NPCOf(lone)
		if n.IsMaster() {
			t.Error("lone NPC is a master before Minions()")
		}
		if got := n.Minions(); len(got) != 0 {
			t.Errorf("lone Minions() = %v, want none", ids(got))
		}
		if !n.IsMaster() {
			t.Error("lone NPC is no master after Minions()")
		}
	}) {
		t.Fatal("invocation panicked")
	}
}
