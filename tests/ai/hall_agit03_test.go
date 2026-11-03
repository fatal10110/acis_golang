package ai

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The Fortress of the Dead scenarios play beside Lidia's family: the
// character stands at fortressSpot and Lidia appears at lidiaSpot.
var (
	fortressSpot = location.Location{X: 58000, Y: -27400, Z: 600}
	lidiaSpot    = location.Location{X: 58100, Y: -27400, Z: 600}
)

// vonHellmannTemplates are the Fortress of the Dead templates: Lidia, who
// lists a DD magic, a self buff and a self range DD magic, Alfred, who
// lists a DD magic, Giselle, who lists a DD magic and a self range buff,
// and the vampires.
func vonHellmannTemplates() []*npc.Template {
	return []*npc.Template{
		agitCasterTemplate(lidiaID, map[npc.SkillType]modelskill.ID{
			npc.SkillTypeDDMagic: hallDDMagic, npc.SkillTypeSelfBuff: hallSelfBuff, npc.SkillTypeSelfRangeDDMagic: hallSelfRangeDD,
		}),
		agitCasterTemplate(alfredID, map[npc.SkillType]modelskill.ID{npc.SkillTypeDDMagic: hallDDMagic}),
		agitCasterTemplate(giselleID, map[npc.SkillType]modelskill.ID{npc.SkillTypeDDMagic: hallDDMagic, npc.SkillTypeSelfRangeBuffA: hallSelfRangeBuff}),
		agitTemplate(vampireID),
	}
}

// bootFortress boots the character at fortressSpot beside
// vonHellmannTemplates, every script draw draw, and raises Lidia at
// lidiaSpot. It returns the world, Lidia, Giselle, Alfred and the frames
// Lidia's appearance sent.
func bootFortress(t *testing.T, draw int) (w *warriorWorld, lidia, giselle, alfred *npc.Hostile, frames [][]byte) {
	t.Helper()
	w = bootHalls(t, 0, draw, fortressSpot, raidBoot{}, vonHellmannTemplates()...)
	lidia, frames = w.spawnSeen(t, lidiaID, lidiaSpot)
	return w, lidia, privateOf(t, lidia, giselleID), privateOf(t, lidia, alfredID), frames
}

// wantDesireWeight fails unless h's cast desire of skill id on target
// weighs weight.
func wantDesireWeight(t *testing.T, name string, h *npc.Hostile, id modelskill.ID, weight float64) {
	t.Helper()
	if d, ok := castDesireOf(h, id); !ok || d.Weight != weight {
		t.Fatalf("%s cast desire of %d = %+v, %v, want weight %v", name, id, d, ok, weight)
	}
}

// TestLidiaRaisesHerFamilyAsSheAppears ports the created hooks of
// Agit03Lidia, Agit03Giselle and Agit03Alfred: Lidia shouts NpcString
// 1010624, raises Giselle, Alfred and 15 vampires where the reference
// places them and facing as it says, and wants to cast her self buff on
// herself, weight 1000000. As they appear Giselle shouts 1010637 and
// Alfred 1010636.
func TestLidiaRaisesHerFamilyAsSheAppears(t *testing.T) {
	t.Parallel()
	_, lidia, giselle, alfred, frames := bootFortress(t, 2)

	for _, c := range []struct {
		name string
		h    *npc.Hostile
		line int32
	}{{"Lidia", lidia, 1010624}, {"Giselle", giselle, 1010637}, {"Alfred", alfred, 1010636}} {
		if got := says(frames, c.h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.line)}) {
			t.Errorf("%s lines = %q, want NpcString %d", c.name, got, c.line)
		}
	}
	wantCast(t, "Lidia", lidia, hallSelfBuff, lidia, 1000000, true)

	want := []string{"35631 56619,-27866,569 54000", "35630 59282,-26496,569 48000", "35647 57905,-27648,608 33540", "35647 57905,-27712,608 33540"}
	for y := -27182; y >= -27782; y -= 50 {
		want = append(want, fmt.Sprintf("35647 58233,%d,608 33540", y))
	}
	var got []string
	for _, m := range lidia.Minions() {
		h := m.(*npc.Hostile)
		at := positionOf(h)
		got = append(got, fmt.Sprintf("%d %d,%d,%d %d", h.NpcID(), at.X, at.Y, at.Z, script.NPCOf(h).SpawnLocation().Heading))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("Lidia's privates:\n got %q\nwant %q", got, want)
	}
}

// TestLidiaBuffsHerselfUntilHerFamilyFalls ports Agit03Lidia's 1001 timer
// and party-died hook: every 30 s she asks her self buff again (the
// unspent requests add up), until Giselle and Alfred have both died: each
// death counts in her int slot 1, and the check that finds it at 2 asks
// once more and is the last.
func TestLidiaBuffsHerselfUntilHerFamilyFalls(t *testing.T) {
	t.Parallel()
	w, lidia, giselle, alfred, _ := bootFortress(t, 2)
	step := func(weight float64) {
		t.Helper()
		w.srv.Advance(t, 30000*time.Millisecond)
		w.srv.Settle(t)
		wantDesireWeight(t, "Lidia", lidia, hallSelfBuff, weight)
	}

	step(2000000)
	w.kill(t, giselle)
	if got := lidia.Scratch().Int(npc.IntAI1); got != 1 {
		t.Fatalf("Lidia int slot 1 after Giselle's death = %d, want 1", got)
	}
	step(3000000)
	w.kill(t, alfred)
	if got := lidia.Scratch().Int(npc.IntAI1); got != 2 {
		t.Fatalf("Lidia int slot 1 after Alfred's death = %d, want 2", got)
	}
	step(4000000)
	step(4000000)
}

// TestLidiaFightsInHerTerritoryAndGoesHome ports Agit03Lidia's attacked
// hook on draws of 0: a hit taking a twentieth of her hit points makes her
// want to attack the character by 1000, and her clan call to herself,
// which comes after, by 1000 more; she also wants to cast her self range DD
// magic at it, weight 1000000, and within 300 no DD magic. Displaced out
// of her territory, 1000 away, a hit adds no hate of its own but asks the
// DD magic and the self range DD magic, and she jumps home dropping her
// attacks and hate: her clan call to herself then leaves her wanting to
// attack the character by its 1000 alone.
func TestLidiaFightsInHerTerritoryAndGoesHome(t *testing.T) {
	t.Parallel()
	w, lidia, _, _, _ := bootFortress(t, 0)
	home := positionOf(lidia)

	w.hit(t, lidia, siegeTwentieth)
	w.wantHate(t, "Lidia", lidia, 2000)
	wantCast(t, "Lidia", lidia, hallSelfRangeDD, w.player, 1000000, true)
	if d, ok := castDesireOf(lidia, hallDDMagic); ok {
		t.Fatalf("Lidia wants her DD magic within 300: %+v", d)
	}

	w.teleport(t, lidia, 0, 1000)
	w.hit(t, lidia, siegeTwentieth)
	if got := positionOf(lidia); got != home {
		t.Fatalf("Lidia at %v after a hit out of her territory, want back at %v", got, home)
	}
	if d, ok := desire(lidia, actorai.IntentionAttack); !ok || d.Weight != 1000 {
		t.Fatalf("Lidia attack desire after a hit out of her territory = %+v, %v, want the clan call's 1000 alone", d, ok)
	}
	w.wantHate(t, "Lidia", lidia, 1000)
	wantDesireWeight(t, "Lidia", lidia, hallSelfRangeDD, 2000000)
	wantCast(t, "Lidia", lidia, hallDDMagic, w.player, 1000000, true)
}

// TestLidiaAnswersHerClan ports Agit03Lidia's clan-attacked hook: the
// character's hit on a vampire near her makes her want to attack the
// character by 1000 for a twentieth of her hit points.
func TestLidiaAnswersHerClan(t *testing.T) {
	t.Parallel()
	w, lidia, _, _, _ := bootFortress(t, 2)
	var near *npc.Hostile
	for _, m := range lidia.Minions() {
		h := m.(*npc.Hostile)
		at := positionOf(h)
		if h.NpcID() == int(vampireID) && math.Hypot(float64(at.X-lidiaSpot.X), float64(at.Y-lidiaSpot.Y)) < 200 {
			near = h
			break
		}
	}
	if near == nil {
		t.Fatal("no vampire within 200 of Lidia")
	}
	w.hit(t, near, siegeTwentieth)
	w.wantHate(t, "Lidia", lidia, 1000)
}

// TestVonHellmannsFightBack ports the attacked hooks of Agit03Alfred and
// Agit03Giselle: a hit taking a twentieth of the NPC's hit points makes it
// want to attack the character by 1000, and, from more than 300 away, on a
// draw of 0, to cast its DD magic at it, weight 1000000; from 100 it wants
// none.
func TestVonHellmannsFightBack(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{alfredID, giselleID} {
		for _, dx := range []int{100, 400} {
			t.Run(fmt.Sprintf("%d from %d", id, dx), func(t *testing.T) {
				t.Parallel()
				w := bootHalls(t, 0, 0, location.Location{}, raidBoot{}, vonHellmannTemplates()...)
				h := w.spawn(t, id, dx, 0)
				w.hit(t, h, siegeTwentieth)
				w.wantHate(t, "the von Hellmann", h, 1000)
				d, ok := castDesireOf(h, hallDDMagic)
				if ok != (dx > 300) {
					t.Fatalf("DD magic desire from %d = %+v, %v", dx, d, ok)
				}
				if ok {
					wantCast(t, "the von Hellmann", h, hallDDMagic, w.player, 1000000, true)
				}
			})
		}
	}
}

// TestGiselleBuffsHerselfEveryFiveMinutes ports Agit03Giselle's 1003
// timer: 1 s after she appears she asks her self range buff on herself,
// weight 1000000, and asks it again every 300 s (the unspent requests add
// up).
func TestGiselleBuffsHerselfEveryFiveMinutes(t *testing.T) {
	t.Parallel()
	w := bootHalls(t, 0, 2, location.Location{}, raidBoot{}, vonHellmannTemplates()...)
	giselle := w.spawn(t, giselleID, 100, 0)
	if _, ok := castDesireOf(giselle, hallSelfRangeBuff); ok {
		t.Fatal("Giselle asks her buff as she appears")
	}
	w.srv.Advance(t, hallTimerStep)
	w.srv.Settle(t)
	wantCast(t, "Giselle", giselle, hallSelfRangeBuff, giselle, 1000000, true)
	w.srv.Advance(t, 299000*time.Millisecond)
	w.srv.Settle(t)
	wantDesireWeight(t, "Giselle", giselle, hallSelfRangeBuff, 1000000)
	w.srv.Advance(t, 1000*time.Millisecond)
	w.srv.Settle(t)
	wantDesireWeight(t, "Giselle", giselle, hallSelfRangeBuff, 2000000)
}
