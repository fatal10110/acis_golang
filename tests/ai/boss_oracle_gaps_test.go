package ai

import (
	"context"
	"slices"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAntharasBehemothCallIsRearmedAfterFourHours ports the re-arm of the
// Behemoth Dragon call (phase 4): the call sets Antharas's flag, and 240
// minutes later its timer 2002 clears it, so the 2-minute check that
// follows calls a second Behemoth Dragon. Until then no other is called.
func TestAntharasBehemothCallIsRearmedAfterFourHours(t *testing.T) {
	w := bootAntharas(t, antharasLiveRow(3), "1hour")
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	w.onQueue(t, boss, func() { boss.SetCurrentHP(boss.MaxHP() * 4 / 10) })

	w.advance(t, 2*time.Minute)
	soleBaiumNPC(t, w, antharasBehemothID)

	// Timer 2002 was armed at 2 minutes for 240: it clears the flag at 242
	// minutes, where the 2-minute check that follows may run in the same
	// instant, so the second call is asserted at 244 minutes.
	w.advance(t, 240*time.Minute-time.Millisecond)
	if got := script.NPCOf(boss).Int(script.SlotIntAI0); got != 1 {
		t.Fatalf("Antharas's call flag just before 4 hours = %d, want 1", got)
	}
	if n := len(baiumNPCs(w, antharasBehemothID)); n != 1 {
		t.Fatalf("%d Behemoth Dragons just before 4 hours, want 1", n)
	}
	w.advance(t, 2*time.Minute+time.Millisecond)
	if n := len(baiumNPCs(w, antharasBehemothID)); n != 2 {
		t.Fatalf("%d Behemoth Dragons after the check that follows the re-arm, want 2", n)
	}
	if got := script.NPCOf(boss).Int(script.SlotIntAI0); got != 1 {
		t.Fatalf("Antharas's call flag after the second call = %d, want 1", got)
	}
}

// baiumBoltID is a damaging spell the character knows when Baium's brackets
// are tested.
const baiumBoltID = modelskill.ID(1177)

// baiumKeepsPoints is every script draw 0 but the 70-in-100 one with which
// Baium's cast drops its target's points to 500, which it fails: so the
// points a hit or a sight gave stay to be read.
func baiumKeepsPoints(n int) int {
	if n == 100 {
		return 99
	}
	return 0
}

// baiumPoints returns what the character stands at in Baium's three target
// slots, -1 for a slot it does not hold.
func baiumPoints(w *baiumWorld, h *script.NPC) [3]int32 {
	out := [3]int32{-1, -1, -1}
	for i, who := range []script.CreatureSlot{npc.CreatureQuest0, npc.CreatureQuest1, npc.CreatureQuest2} {
		if got := h.Slot(who); got != nil && got.ObjectID() == w.player.ObjectID() {
			out[i] = h.Int([]script.IntSlot{npc.IntQuest0, npc.IntQuest1, npc.IntQuest2}[i])
		}
	}
	return out
}

// TestBaiumRanksSpellDamageByHPBracket ports the points a spell earns the
// character from Baium: the damage over 150 at 3/4 of its HP or more, over
// 100 under 3/4, over 50 under 1/2 and over 30 under 1/4, times 1000, kept
// only when that plus 1000 beats what the character stands at. The
// character holds one slot from seeing Baium, so the spell moves its
// points only.
func TestBaiumRanksSpellDamageByHPBracket(t *testing.T) {
	bolt := modelskill.Definition{
		ID: baiumBoltID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, Magic: true, SkillType: "MDAM",
		CastRange: 1500, HitTime: 500, StaticHitTime: true, StaticReuse: true, Power: 20,
	}
	db := sqltest.SharedDB(t)
	table := modelskill.NewTable([]modelskill.Definition{bolt})
	w := bootBaiumWith(t, "116033;17447;10107;0", 0, func(srv *gameservertest.Server, objID int32) {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(baiumBoltID), 1); err != nil {
			t.Fatalf("seed the spell: %v", err)
		}
	},
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithScriptSkills(bolt),
		gameservertest.WithScriptRand(baiumKeepsPoints),
		gameservertest.WithMagicFailures(false),
	)
	w.advance(t, 5*time.Second)
	boss := soleBaiumNPC(t, w, baiumID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	for i := 0; boss.AI().CurrentIntention() != actorai.IntentionWander; i++ {
		if i == 10 {
			t.Fatalf("Baium's intention after 10 AI ticks = %v, want wander", boss.AI().CurrentIntention())
		}
		w.ticks(t, 1)
	}
	for range 2 {
		w.srv.Client.Send(encodeFolkAction(boss.ObjectID(), w.at))
		w.srv.Settle(t)
	}
	w.srv.ReadQueued(t, w.srv.Client)

	// Each bracket, from the highest HP down, so each rank beats the one
	// before and is kept.
	for _, row := range []struct {
		name  string
		hp    float64
		ratio int
	}{
		{"3/4 or more", 1.0, 150},
		{"under 3/4", 0.7, 100},
		{"under 1/2", 0.45, 50},
		{"under 1/4", 0.2, 30},
	} {
		w.onQueue(t, boss, func() { boss.SetCurrentHP(int(row.hp * float64(boss.MaxHP()))) })
		before := baiumPoints(w, handle)
		hp := boss.CurrentHP()
		w.castFrames(t, baiumBoltID)
		d := hp - boss.CurrentHP()
		if d <= 0 {
			t.Fatalf("%s: the spell did %d damage, want some", row.name, d)
		}
		got := baiumPoints(w, handle)
		held := -1
		for i := range got {
			if got[i] >= 0 {
				held = i
			}
		}
		if held < 0 {
			t.Fatalf("%s: Baium ranks no slot for the character", row.name)
		}
		want := before[held]
		if exact := int32(d/row.ratio) * 1000; exact+1000 > before[held] {
			want = exact
		}
		if got[held] != want {
			t.Fatalf("%s: the character's points after a spell of %d, from %d = %d, want %d", row.name, d, before[held], got[held], want)
		}
		if want == before[held] {
			t.Fatalf("%s: the spell of %d did not move the character's points off %d: the test cannot tell the brackets apart", row.name, d, want)
		}
	}
}

// TestBaiumRanksAPriestBySightByHPTier ports the points a priest earns when
// Baium sees it: 2000 at 3/4 of its HP or more, 3000 under 3/4, 6000 under
// 1/2 and 10000 under 1/4, against 1000 for anyone else.
func TestBaiumRanksAPriestBySightByHPTier(t *testing.T) {
	w := bootBaium(t, "116033;17447;10107;0", gameservertest.WithScriptRand(baiumKeepsPoints))
	w.advance(t, 5*time.Second)
	boss := soleBaiumNPC(t, w, baiumID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	for i := 0; boss.AI().CurrentIntention() != actorai.IntentionWander; i++ {
		if i == 10 {
			t.Fatalf("Baium's intention after 10 AI ticks = %v, want wander", boss.AI().CurrentIntention())
		}
		w.ticks(t, 1)
	}
	pts := baiumPoints(w, handle)
	if got := slices.Max(pts[:]); got != 1000 {
		t.Fatalf("the character's points after the first sight = %d, want 1000", got)
	}
	const cleric = 15
	w.onBaiumCharacter(t, func(c *player.Character) { c.SetClassID(cleric) })

	for _, row := range []struct {
		name string
		hp   float64
		want int32
	}{
		{"3/4 or more", 1.0, 2000},
		{"under 3/4", 0.6, 3000},
		{"under 1/2", 0.4, 6000},
		{"under 1/4", 0.2, 10000},
	} {
		w.onQueue(t, boss, func() { boss.SetCurrentHP(int(row.hp * float64(boss.MaxHP()))) })
		w.sightAgain(t)
		var best int32 = -1
		for _, p := range baiumPoints(w, handle) {
			best = max(best, p)
		}
		if best != row.want {
			t.Fatalf("%s: the priest's points after Baium sees it = %d, want %d", row.name, best, row.want)
		}
	}
}

// sightAgain tells Baium about the character once more, as a jump or a
// teleport of the character does.
func (w *baiumWorld) sightAgain(t *testing.T) {
	t.Helper()
	w.onBaiumCharacter(t, func(*player.Character) { npc.ForceSeeCreature(w.srv.State, w.player) })
}
