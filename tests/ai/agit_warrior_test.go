package ai

import (
	"database/sql"
	"math"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/agitwarrior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The clan hall siege beasts the tests spawn, and the NPCs they see.
const (
	agitBerserkerID = int32(35428) // AgitWarrior, Monster
	agitCougarID    = int32(35618) // AgitWarrior, Folk
	agitBuffaloID   = int32(35619) // AgitWarriorFlee, Folk
	agitWolfID      = int32(20001) // no behavior, Monster
	agitClerkID     = int32(30001) // no behavior, Folk

	// agitClanID is the clan the character belongs to when a test seeds
	// one.
	agitClanID = int32(7101)
)

// bootAgitWarriors boots one character beside the templates, the
// AgitWarrior chain registered in scripts.xml order, every draw 0, with
// opts.
func bootAgitWarriors(t *testing.T, opts []gameservertest.Option, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.AgitWarrior.AgitWarrior",
		"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitWarriorFlee",
	}
	catalog := script.Catalog{paths[0]: defaultnpc.New, paths[1]: agitwarrior.Warrior, paths[2]: agitwarrior.WarriorFlee}
	list := make([]script.Listing, 0, len(paths))
	for _, path := range paths {
		list = append(list, script.Listing{Path: path})
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
		if tmpl.Type == "Folk" {
			kinds[int32(tmpl.ID)] = script.KindFolk
		}
	}
	return bootRaid(t, raidBoot{opts: append(opts, gameservertest.WithNPCScripts(kinds, list, catalog))}, templates...)
}

// agitClanSeed makes the character a member of agitClanID.
func agitClanSeed(t *testing.T) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		for _, q := range []string{
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) SELECT 7101, 'Beasts', 1, 0, obj_Id FROM characters WHERE char_name = 'Hunter'`,
			`UPDATE characters SET clanid = 7101 WHERE char_name = 'Hunter'`,
		} {
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	})
}

// folkWeight returns the weight of f's queued attack desire on the
// character, 0 for none.
func (w *warriorWorld) folkWeight(f *npc.Folk) float64 {
	for _, d := range f.Desires().Snapshot() {
		if d.Kind == actorai.IntentionAttack && d.FinalTarget != nil && d.FinalTarget.ObjectID() == w.player.ObjectID() {
			return d.Weight
		}
	}
	return 0
}

// TestAgitWarriorCivilianWeighsHitsByItsMaxHP runs AgitWarrior's and
// AgitWarriorFlee's attacked hooks on the frenzied beasts, which are
// civilians: a hit of a tenth and a half of a beast's maximum hit points
// makes it want to attack the character by 100 per twentieth (300), the
// fleeing kind's weight cut to a whole number; seeing the character as it
// appeared, each also wants to attack it by 200.
func TestAgitWarriorCivilianWeighsHitsByItsMaxHP(t *testing.T) {
	t.Parallel()
	cougar, buffalo := folkFighterTemplate(agitCougarID), folkFighterTemplate(agitBuffaloID)
	cougar.HPMax, buffalo.HPMax = 1000, 1000
	w := bootAgitWarriors(t, nil, cougar, buffalo)
	for _, id := range []int32{agitCougarID, agitBuffaloID} {
		f := w.spawnFolk(t, id, 100, 0)
		w.ticks(t, 1)
		seen := w.folkWeight(f)
		if seen != 200 {
			t.Fatalf("npc %d: attack desire once it saw the character = %v, want 200", id, seen)
		}
		damage := f.MaxHP()*3/20 + 1
		w.onFolkQueue(t, f, func() { f.TakeDamage(damage, w.player) })
		want := float64(damage) / float64(f.MaxHP()) / 0.05 * 100
		if id == agitBuffaloID {
			want = math.Trunc(want)
		}
		if got := w.folkWeight(f) - seen; got != want {
			t.Fatalf("npc %d (max HP %d): attack weight of a hit of %d = %v, want %v", id, f.MaxHP(), damage, got, want)
		}
	}
}

// TestAgitWarriorSparesItsOwnClan runs AgitWarrior's clan test: a
// berserker spawned for the character's clan neither wants to attack the
// character it sees nor answers its hits, while one of no clan attacks it,
// as it would a clanless character.
func TestAgitWarriorSparesItsOwnClan(t *testing.T) {
	t.Parallel()
	w := bootAgitWarriors(t, []gameservertest.Option{agitClanSeed(t)}, warriorTemplate(agitBerserkerID, nil))
	live, err := w.srv.NpcSpawns.SpawnForClan(agitBerserkerID, w.at.X+100, w.at.Y, w.at.Z, 0, agitClanID)
	if err != nil {
		t.Fatalf("spawn for the clan: %v", err)
	}
	own := live.(*npc.Hostile)
	other := w.spawn(t, agitBerserkerID, -100, 0)
	w.ticks(t, 1)
	w.hit(t, own, 50)
	w.hit(t, other, 50)
	if d, ok := desire(own, actorai.IntentionAttack); ok {
		t.Fatalf("berserker of the character's clan wants %+v", d)
	}
	if d, ok := desire(own, actorai.IntentionCast); ok {
		t.Fatalf("berserker of the character's clan wants %+v", d)
	}
	if d, ok := desire(other, actorai.IntentionAttack); !ok || d.Weight != 200+50.0/440/0.05*100 {
		t.Fatalf("berserker of no clan attack desire = %+v (%v), want 200 for the sight and its hit's weight", d, ok)
	}
}

// TestAgitWarriorLooksAroundAfterAWalk runs AgitWarrior's move-finished
// and see-creature hooks: a berserker far from the character that ends a
// walk looks around and wants to attack the monster it sees, adding 200
// to its hate, but neither the civilian nor the character, out of its
// sight.
func TestAgitWarriorLooksAroundAfterAWalk(t *testing.T) {
	t.Parallel()
	w := bootAgitWarriors(t, nil, warriorTemplate(agitBerserkerID, nil), warriorTemplate(agitWolfID, nil),
		gameservertest.FolkTemplate("Folk", int(agitClerkID)))
	h := w.spawn(t, agitBerserkerID, 1500, 0)
	wolf := w.spawn(t, agitWolfID, 1800, 0)
	clerk := w.spawnFolk(t, agitClerkID, 1500, 300)
	// The berserker looked around as it appeared, before the wolf came:
	// it looks again no sooner than 30 s later.
	w.srv.Advance(t, 30*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if d, ok := desire(h, actorai.IntentionAttack); ok || h.AggroHate(wolf) != 0 {
		t.Fatalf("setup: before its walk the berserker wants %+v (%v), hates the wolf by %v", d, ok, h.AggroHate(wolf))
	}
	to := positionOf(h)
	to.X += 100
	w.onQueue(t, h, func() { script.NPCOf(h).AddMoveToDesire(script.Loc{X: to.X, Y: to.Y, Z: to.Z}, 100) })
	// A wander walk may be under way: the berserker walks there first.
	for i := 0; ; i++ {
		if _, ok := desire(h, actorai.IntentionMoveTo); !ok {
			break
		}
		if i >= 400 {
			t.Fatal("the berserker never arrived")
		}
		w.ticks(t, 1)
		for j := 0; h.IsMoving() && j < 200; j++ {
			w.srv.TickPositions()
		}
		w.srv.Settle(t)
	}
	// The attack desire on the wolf may be acted on and dropped by now;
	// the hate it added stays, decaying.
	if got := h.AggroHate(wolf); got <= 0 || got > 200 {
		t.Fatalf("berserker hate of the wolf after its walk = %v, want up to 200", got)
	}
	if got := h.AggroHate(clerk); got != 0 {
		t.Fatalf("berserker hate of the civilian = %v, want none", got)
	}
	if got := h.AggroHate(w.player); got != 0 {
		t.Fatalf("berserker hate of the character out of its sight = %v, want none", got)
	}
}
