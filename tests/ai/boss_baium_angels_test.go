package ai

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// enterVortex opens the Angelic Vortex's page, asks it for the way up and
// returns the frames of the answer.
func (w *baiumWorld) enterVortex(t *testing.T) [][]byte {
	t.Helper()
	w.openVortex(t)
	return w.askVortexEntry(t)
}

// openVortex talks to the Angelic Vortex, which answers its first page.
func (w *baiumWorld) openVortex(t *testing.T) {
	t.Helper()
	got := baiumPageNames(w.talkTo(t, soleBaiumNPC(t, w, baiumVortexID)))
	if len(got) == 0 || slices.ContainsFunc(got, func(p string) bool { return p != "31862-00.htm" }) {
		t.Fatalf("the vortex's pages = %q, want its first page", got)
	}
}

// askVortexEntry follows the first page's entry link and returns the
// frames of the answer.
func (w *baiumWorld) askVortexEntry(t *testing.T) [][]byte {
	t.Helper()
	w.srv.Client.Send(encodeManagerBypass("Quest BaiumAngelicVortex enter"))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// onBaiumCharacter runs fn on the character's queue.
func (w *baiumWorld) onBaiumCharacter(t *testing.T, fn func(c *player.Character)) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		fn(w.player.(player.CharacterHolder).PlayerCharacter())
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// fabricsHeld returns how many Blooded Fabrics the character's saved items
// hold.
func (w *baiumWorld) fabricsHeld(t *testing.T) int {
	t.Helper()
	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.player.ObjectID())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, row := range rows {
		if row.TemplateID == bloodedFabric {
			n += row.Count
		}
	}
	return n
}

// characterTeleports returns where each TeleportToLocation of the
// character among frames sends it.
func (w *baiumWorld) characterTeleports(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		out = append(out, baiumFrames(t, [][]byte{f}, map[int32]string{w.player.ObjectID(): "player"})...)
	}
	return out
}

// TestBaiumVortexFollowsBaiumsState ports the Angelic Vortex's entry gate
// (phase 5): while Baium sleeps at db value 0 a character with no Blooded
// Fabric is turned away with 31862-03.htm; once the stone woke Baium (db
// value 1) the entry answers 31862-01.htm, and on a wyvern 31862-05.htm,
// which goes first. The vortex's first page, its story and its answer with
// no Baium in the world are the quest scenario
// baium_angelic_vortex.scenario.
func TestBaiumVortexFollowsBaiumsState(t *testing.T) {
	w := bootBaium(t, "-105200;-253104;-15264;0")
	if got := baiumPageNames(w.enterVortex(t)); !slices.Equal(got, []string{"31862-03.htm"}) {
		t.Fatalf("the entry without a fabric = %q, want 31862-03.htm", got)
	}

	wakeBaium(t, w)
	if got := baiumPageNames(w.enterVortex(t)); !slices.Equal(got, []string{"31862-01.htm"}) {
		t.Fatalf("the entry with Baium awake = %q, want 31862-01.htm", got)
	}

	w.openVortex(t)
	w.onBaiumCharacter(t, func(c *player.Character) {
		if !c.Mount(player.WyvernNPCID, 0) {
			t.Error("setup: the character cannot mount the wyvern")
		}
	})
	if got := baiumPageNames(w.askVortexEntry(t)); !slices.Equal(got, []string{"31862-05.htm"}) {
		t.Fatalf("the entry on a wyvern = %q, want 31862-05.htm", got)
	}
}

// TestBaiumVortexLetsAFabricHolderUp ports the vortex's way up (phase 5):
// while Baium sleeps at db value 0, the entry takes one of the character's
// two Blooded Fabrics, sends it to 114077,15882,10078 and answers no page;
// the count left, 1, is saved.
func TestBaiumVortexLetsAFabricHolderUp(t *testing.T) {
	w := bootBaiumHolding(t, "-105200;-253104;-15264;0", 2)

	frames := w.enterVortex(t)
	if got := baiumPageNames(frames); len(got) != 0 {
		t.Fatalf("the entry's pages = %q, want none", got)
	}
	if got, want := w.characterTeleports(t, frames), []string{"S TeleportToLocation obj=player loc=114077,15882,10078 fast=0"}; !slices.Equal(got, want) {
		t.Fatalf("the entry's teleports = %q, want %q", got, want)
	}
	if got := w.fabricsHeld(t); got != 1 {
		t.Fatalf("saved fabrics = %d, want 1", got)
	}
}

// TestBaiumStoneStaysWithoutBaium ports the stone's read of global memo
// key 2 (phase 2): with Baium dead the key holds nothing, so the talk's
// hook aborts on its read, after the stone marks itself woken: the stone
// stays in the world, and a second talk does nothing either.
func TestBaiumStoneStaysWithoutBaium(t *testing.T) {
	w := bootBaium(t, "-105200;-253104;-15264;0")
	boss := soleBaiumNPC(t, w, baiumID).(*npc.Hostile)
	stone := soleBaiumNPC(t, w, baiumStoneID)
	w.onQueue(t, boss, func() { script.NPCOf(boss).DoDie(script.PlayerOf(w.player)) })
	w.srv.ReadQueued(t, w.srv.Client)
	w.advance(t, 3*time.Second)
	if n := len(baiumNPCs(w, baiumStoneID)); n != 1 {
		t.Fatalf("%d stones after Baium's death, want its one stone", n)
	}

	for i := range 2 {
		w.talkToStone(t, stone)
		if got := baiumNPCs(w, baiumStoneID); len(got) != 1 || got[0].ObjectID() != stone.ObjectID() {
			t.Fatalf("stones after talk %d = %d, want the one stone kept", i+1, len(got))
		}
		if got := script.NPCOf(stone).Int(script.SlotIntAI0); got != 1 {
			t.Fatalf("the stone's woken mark after talk %d = %d, want 1", i+1, got)
		}
	}
}

// archangelsByIndex returns Baium's live archangels by their index, the
// first spawn parameter Baium gave each.
func archangelsByIndex(t *testing.T, w *baiumWorld) map[int32]*npc.Hostile {
	t.Helper()
	out := map[int32]*npc.Hostile{}
	for _, c := range baiumNPCs(w, baiumArchangelID) {
		out[script.NPCOf(c).Int(script.SlotIntAI0)] = c.(*npc.Hostile)
	}
	if len(out) != 5 {
		t.Fatalf("archangels by index = %d, want 5", len(out))
	}
	return out
}

// desiresOn renders h's queued desires on target as kind/skill=weight,
// sorted.
func desiresOn(h *npc.Hostile, target attackable.Combatant) []string {
	var out []string
	for _, d := range h.AI().Desires().Snapshot() {
		if d.FinalTarget == nil || d.FinalTarget.ObjectID() != target.ObjectID() {
			continue
		}
		switch d.Kind {
		case actorai.IntentionAttack:
			out = append(out, fmt.Sprintf("attack=%g", d.Weight))
		case actorai.IntentionCast:
			out = append(out, fmt.Sprintf("cast %d=%g", d.Skill.ID, d.Weight))
		}
	}
	slices.Sort(out)
	return out
}

// TestBaiumArchangelsTakeTheirSides ports the archangels' created hook and
// their first checks (phase 3), every draw 0: each appears with its index
// as Baium gave it, turned on Baium (1) and its last damage at 100. 5 s
// later, at their first checks, Baium's flag 1 keeps archangels 1 and 2
// turned on Baium, each wanting to attack it with 10000 from each of its
// two checks, and frees the other three.
func TestBaiumArchangelsTakeTheirSides(t *testing.T) {
	w := bootBaium(t, "116033;17447;10107;0")
	w.advance(t, 5*time.Second)
	boss := soleBaiumNPC(t, w, baiumID)
	angels := archangelsByIndex(t, w)
	for i := int32(1); i <= 5; i++ {
		h := script.NPCOf(angels[i])
		if on, dmg, idle := h.Int(script.SlotIntAI1), h.Int(script.SlotIntAI2), h.Int(script.SlotIntAI3); on != 1 || dmg != 100 || idle != 0 {
			t.Fatalf("archangel %d at its creation: on Baium %d, last damage %d, idle %d; want 1, 100, 0", i, on, dmg, idle)
		}
	}

	w.advance(t, 5*time.Second)
	for i := int32(1); i <= 5; i++ {
		want := int32(0)
		if i <= 2 {
			want = 1
		}
		if got := script.NPCOf(angels[i]).Int(script.SlotIntAI1); got != want {
			t.Errorf("archangel %d on Baium = %d, want %d", i, got, want)
		}
		if i > 2 {
			continue
		}
		if got, want := desiresOn(angels[i], boss), []string{"attack=20000"}; !slices.Equal(got, want) {
			t.Errorf("archangel %d's desires on Baium = %q, want %q", i, got, want)
		}
	}
}

// TestBaiumArchangelAnswersAHit ports an archangel's attacked hook (phase
// 3), every draw 0: just created, turned on Baium, a hit of 7 from the
// character makes the character its last attacker with damage 7 and wants
// its skill 4132 cast at Baium with 1000000, not at the character; its HP
// above half, it wants no 4133.
func TestBaiumArchangelAnswersAHit(t *testing.T) {
	w := bootBaium(t, "116033;17447;10107;0")
	w.advance(t, 5*time.Second)
	boss := soleBaiumNPC(t, w, baiumID)
	angel := archangelsByIndex(t, w)[1]

	w.hit(t, angel, 7)
	h := script.NPCOf(angel)
	if last := h.Slot(script.SlotCreature2); last == nil || last.ObjectID() != w.player.ObjectID() {
		t.Fatalf("the archangel's last attacker = %v, want the character", last)
	}
	if got := h.Int(script.SlotIntAI2); got != 7 {
		t.Fatalf("the archangel's last damage = %d, want 7", got)
	}
	if got, want := desiresOn(angel, boss), []string{"cast 4132=1e+06"}; !slices.Equal(got, want) {
		t.Fatalf("the archangel's desires on Baium = %q, want %q", got, want)
	}
	if got := desiresOn(angel, w.player); len(got) != 0 {
		t.Fatalf("the archangel's desires on the character = %q, want none", got)
	}
}

// TestBaiumArchangelsLeaveWithBaium ports the archangels' check every 5 s
// (phase 4): once Baium's flag is 6, as its reset leaves it, each
// archangel leaves the world at its next check.
func TestBaiumArchangelsLeaveWithBaium(t *testing.T) {
	w := bootBaium(t, "116033;17447;10107;0")
	w.advance(t, 5*time.Second)
	boss := soleBaiumNPC(t, w, baiumID).(*npc.Hostile)
	archangelsByIndex(t, w)

	w.onQueue(t, boss, func() { script.NPCOf(boss).SetInt(script.SlotFlag, 6) })
	w.advance(t, 4900*time.Millisecond)
	if n := len(baiumNPCs(w, baiumArchangelID)); n != 5 {
		t.Fatalf("%d archangels before their check, want 5", n)
	}
	w.advance(t, 100*time.Millisecond)
	if n := len(baiumNPCs(w, baiumArchangelID)); n != 0 {
		t.Fatalf("%d archangels after their check, want none", n)
	}
}
