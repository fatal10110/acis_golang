package ai

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootWarriorBaseWith is bootWarriorBaseOnly with extra's options and
// ready step added to the boot.
func bootWarriorBaseWith(t *testing.T, extra raidBoot, templates ...*npc.Template) (*warriorWorld, *ind1Draws) {
	t.Helper()
	list, catalog := warriorScripts()
	const path = "test.WarriorBaseWith"
	catalog[path] = func() script.Script {
		s := warrior.Base()
		s.NPCs = []int32{warriorBaseOnlyID}
		return s
	}
	list = append(list, script.Listing{Path: path})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra.opts...)
	return bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...), draws
}

// settleSight spawns a WarriorBase NPC of params dx, dy from the character,
// lets it look at the character on its first cycles with every draw 99 (so
// no chance-gated branch fires), and returns it.
func settleSight(t *testing.T, w *warriorWorld, draws *ind1Draws, dx, dy int) *npc.Hostile {
	t.Helper()
	draws.set(99)
	h := w.spawn(t, warriorBaseOnlyID, dx, dy)
	w.ticks(t, 2)
	return h
}

// threaten enters other in h's threat table with hate, which leaves its
// intention as it is, on h's queue.
func (w *warriorWorld) threaten(t *testing.T, h, other *npc.Hostile, hate float64) {
	t.Helper()
	w.onQueue(t, h, func() { h.AI().Threats().AddDamage(other, 0, hate) })
}

// forgetEveryone drops every attack desire and hate h holds, on its queue.
func (w *warriorWorld) forgetEveryone(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, script.NPCOf(h).RemoveAllAttackDesire)
}

// fightOther has h fight other, so its intention is an attack.
func (w *warriorWorld) fightOther(t *testing.T, h, other *npc.Hostile) {
	t.Helper()
	w.hateOn(t, h, other, 500)
	w.ticks(t, 1)
	if got := script.NPCOf(h).Intention(); got != script.IntentionAttack {
		t.Fatalf("setup: the NPC's intention = %v, want an attack", got)
	}
}

// TestWarriorBaseRandomAggressiveAttacksAPlayerOnAChance ports the
// RandomAggressive branch of WarriorBase's see-creature hook. An NPC that is
// not fighting and sees a player attacks it when a draw of 100 is under the
// parameter, otherwise drops the hate it holds; one that is fighting keeps its
// hate and takes none. Attacking, it keeps its hate of the others.
func TestWarriorBaseRandomAggressiveAttacksAPlayerOnAChance(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		draw     int
		fighting bool
		attack   bool // the NPC wants the character afterwards
		kept     bool // the NPC still hates the other
	}{
		{"idle, draw under the chance", 29, false, true, true},
		{"idle, draw at the chance", 30, false, false, false},
		{"fighting, draw under the chance", 29, true, false, true},
		{"fighting, draw at the chance", 30, true, false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w, draws := bootWarriorBaseWith(t, raidBoot{},
				warriorTemplate(warriorBaseOnlyID, npc.AIParams{"RandomAggressive": "30"}), warriorTemplate(warriorFarID, nil))
			h := settleSight(t, w, draws, 150, 0)
			other := w.spawn(t, warriorFarID, -150, 0)
			if row.fighting {
				w.fightOther(t, h, other)
			} else {
				w.threaten(t, h, other, 500)
			}
			if h.AggroHate(other) == 0 {
				t.Fatal("setup: the NPC hates nothing about the other")
			}
			draws.set(row.draw)

			w.sightCharacterAgain(t)
			if d, ok := attackOn(h, w.player); ok != row.attack || (ok && d.Weight != 200) {
				t.Fatalf("attack desire on the character = %+v (%v), want weight 200: %v", d, ok, row.attack)
			}
			if got := h.AggroHate(other) > 0; got != row.kept {
				t.Fatalf("hate of the other = %v, want kept: %v", h.AggroHate(other), row.kept)
			}
		})
	}
}

// TestWarriorBaseHalfAggressiveAttacksByTheGameHour ports the HalfAggressive
// branch of WarriorBase's see-creature hook: with 1 an idle NPC attacks a
// player it sees from game hour 5 on, with 2 before it; neither attacks
// while fighting, and the branch never takes a draw.
func TestWarriorBaseHalfAggressiveAttacksByTheGameHour(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		param    string
		hour     int
		fighting bool
		attack   bool
	}{
		{"1 at hour 5", "1", 5, false, true},
		{"1 at hour 4", "1", 4, false, false},
		{"1 at hour 5, fighting", "1", 5, true, false},
		{"2 at hour 4", "2", 4, false, true},
		{"2 at hour 5", "2", 5, false, false},
		{"2 at hour 4, fighting", "2", 4, true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			// The in-game clock reads half past the hour, an in-game hour
			// being 600 real seconds, so the cycles never carry it over.
			now := time.Now()
			midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			clock := gameservertest.WithGameClockNow(func() time.Time { return midnight.Add(time.Duration(row.hour)*600*time.Second + 300*time.Second) })
			w, draws := bootWarriorBaseWith(t, raidBoot{opts: []gameservertest.Option{clock}},
				warriorTemplate(warriorBaseOnlyID, npc.AIParams{"HalfAggressive": row.param}), warriorTemplate(warriorFarID, nil))
			h := settleSight(t, w, draws, 150, 0)
			other := w.spawn(t, warriorFarID, -150, 0)
			// It looked at the character as it appeared; look again, this
			// time with nothing else on its mind or only the other.
			if row.fighting {
				w.forgetEveryone(t, h)
				w.fightOther(t, h, other)
			}

			w.sightCharacterAgain(t)
			if d, ok := attackOn(h, w.player); ok != row.attack || (ok && d.Weight != 200) {
				t.Fatalf("attack desire on the character = %+v (%v), want weight 200: %v", d, ok, row.attack)
			}
		})
	}
}

// TestWarriorBaseDaggerBackAttackStrikesFromBehind ports the DaggerBackAttack
// branch of WarriorBase's see-creature hook: an idle NPC with the parameter 1
// that stands behind a player closer than 100 attacks it, on a draw of 100 under
// 50, saying the line a second draw of 4 picks; the first two lines name the
// player. A draw of 50, a player facing the NPC or one 100 away,
// another parameter, or a fighting NPC, do nothing.
func TestWarriorBaseDaggerBackAttackStrikesFromBehind(t *testing.T) {
	t.Parallel()
	const name = "Hunter"
	lines := []string{
		script.NpcString(1000286, name), script.NpcString(1000381, name), script.NpcString(1000382), script.NpcString(1000383),
	}
	for _, row := range []struct {
		name     string
		param    string
		dx       int // the NPC's offset from the character; the character faces east
		draw     int
		line     int
		fighting bool
		attack   bool
	}{
		{"behind, line 0", "1", -80, 49, 0, false, true},
		{"behind, line 1", "1", -99, 0, 1, false, true},
		{"behind, line 2", "1", -80, 10, 2, false, true},
		{"behind, line 3", "1", -80, 10, 3, false, true},
		{"behind, draw 50", "1", -80, 50, 0, false, false},
		{"in front", "1", 80, 10, 0, false, false},
		{"behind at 100", "1", -100, 10, 0, false, false},
		{"parameter 2", "2", -80, 10, 0, false, false},
		{"fighting", "1", -80, 10, 0, true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w, draws := bootWarriorBaseWith(t, raidBoot{},
				warriorTemplate(warriorBaseOnlyID, npc.AIParams{"DaggerBackAttack": row.param}), warriorTemplate(warriorFarID, nil))
			if h := w.player.Heading(); h != 0 {
				t.Fatalf("setup: the character's heading = %d, want 0 (east)", h)
			}
			h := settleSight(t, w, draws, row.dx, 0)
			w.forgetEveryone(t, h)
			if row.fighting {
				w.fightOther(t, h, w.spawn(t, warriorFarID, 0, 150))
			}
			if d, ok := attackOn(h, w.player); ok {
				t.Fatalf("setup: the NPC already wants the character: %+v", d)
			}
			draws.set(99, row.draw, row.line)

			w.sightCharacterAgain(t)
			got := sayLines(w.srv.ReadQueued(t, w.srv.Client), h)
			if d, ok := attackOn(h, w.player); ok != row.attack || (ok && d.Weight != 200) {
				t.Fatalf("attack desire on the character = %+v (%v), want weight 200: %v", d, ok, row.attack)
			}
			if row.attack {
				if want := []string{lines[row.line]}; !slices.Equal(got, want) {
					t.Fatalf("lines = %q, want %q", got, want)
				}
			} else if len(got) != 0 {
				t.Fatalf("lines = %q, want none", got)
			}
		})
	}
}
