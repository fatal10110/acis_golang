package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Rainbow Springs NPCs of the hot springs game, and the NPC the gourd
// test listens to its script events through.
const (
	gourdID           = int32(35588) // AzitWateringGroud
	mimicMagicalID    = int32(35594) // AzitWateringMimicMagical
	hallTreasureBoxID = int32(35595) // AzitWateringTreasureBox
	recycleCanID      = int32(35600) // AzitWateringRecycleCan
	hallListenerID    = int32(29981) // bound by the test
)

// The ground items the sweeper is shown: a letter of the hot springs game
// and an item of no game.
const (
	hallLetterID = int32(8040)
	hallPotionID = int32(1060)
)

// hallListener is a behavior that says each script event its NPC receives,
// with the event's two arguments.
func hallListener() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{hallListenerID}, Hooks: script.Hooks{
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			e.NPC.Say(fmt.Sprintf("event %d %d %d", e.EventID, e.Arg1, e.Arg2))
		},
	}}
}

// TestGourdTellsItsNeighboursWhoBrokeIt ports AzitWateringGroud's dying
// hook: 3 s after the character kills a gourd, every NPC within 500 of it
// receives script event 5 with 40000 and the character's object id; one
// 600 away receives nothing.
func TestGourdTellsItsNeighboursWhoBrokeIt(t *testing.T) {
	t.Parallel()
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":          defaultnpc.New,
		"script.ai.siegablehall.AzitWateringGroud": hall.Gourd,
		"test.HallListener":                        hallListener,
	}
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.siegablehall.AzitWateringGroud"}, {Path: "test.HallListener"}}
	kinds := map[int32]script.NPCKind{gourdID: script.KindFolk, hallListenerID: script.KindHostile}
	scripts := raidBoot{opts: []gameservertest.Option{gameservertest.WithNPCScripts(kinds, list, catalog)}}
	w := bootHalls(t, 0, 0, location.Location{}, scripts, gameservertest.FolkTemplate("Folk", int(gourdID)), agitTemplate(hallListenerID))
	gourd := w.spawnFolk(t, gourdID, 100, 0)
	near := w.spawn(t, hallListenerID, 500, 0)
	far := w.spawn(t, hallListenerID, -500, 0)

	w.onFolkQueue(t, gourd, func() { gourd.Kill(w.player) })
	var frames [][]byte
	w.srv.AdvanceUntil(t, "the gourd's dying hook", func() bool {
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return len(says(frames, near.ObjectID())) > 0
	})
	want := []string{fmt.Sprintf("event 5 40000 %d", w.player.ObjectID())}
	if got := says(frames, near.ObjectID()); !slices.Equal(got, want) {
		t.Fatalf("listener within 500 heard %q, want %q", got, want)
	}
	w.srv.Advance(t, time.Second)
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	if got := says(frames, far.ObjectID()); len(got) != 0 {
		t.Fatalf("listener 600 away heard %q, want nothing", got)
	}
}

// TestMagicalMimicVanishesOnceItsCastLands ports AzitWateringMimicMagical:
// a hit on a draw of 0 makes it want to cast its DD magic at the
// character, weight 1000000, walking to it; once the cast lands it leaves
// the world.
func TestMagicalMimicVanishesOnceItsCastLands(t *testing.T) {
	t.Parallel()
	w := bootHalls(t, 0, 0, location.Location{}, raidBoot{},
		agitCasterTemplate(mimicMagicalID, map[npc.SkillType]modelskill.ID{npc.SkillTypeDDMagic: hallDDMagic}))
	mimic := w.spawn(t, mimicMagicalID, 150, 0)

	w.hit(t, mimic, siegeTwentieth)
	wantCast(t, "mimic", mimic, hallDDMagic, w.player, 1000000, true)
	if !w.inWorld(mimic) {
		t.Fatal("the mimic left the world before its cast")
	}
	var frames [][]byte
	w.srv.AdvanceUntil(t, "the mimic's cast and departure", func() bool {
		frames = append(frames, cycle(t, w.srv)...)
		return !w.inWorld(mimic)
	})
	if got := castsOf(frames, mimic, hallDDMagic); len(got) != 1 || got[0] != w.player.ObjectID() {
		t.Fatalf("mimic casts of its DD magic = %v, want one on the character", got)
	}
}

// TestTreasureBoxTakesOnlyBlowsItsScriptCounts ports
// AzitWateringTreasureBox's created and attacked hooks: the box is
// invulnerable as it appears, so the character's offensive skill takes none
// of its hit points (the script ignores a skill), while a barehanded hit
// of 22 takes 22 through the script, which refreshes the health bar of the
// character targeting the box.
func TestTreasureBoxTakesOnlyBlowsItsScriptCounts(t *testing.T) {
	t.Parallel()
	w := bootHalls(t, 0, 0, location.Location{}, siegeSkillBoot(t), agitTemplate(hallTreasureBoxID))
	box := w.spawn(t, hallTreasureBoxID, 150, 0)
	if !box.Invul() {
		t.Fatal("the box is not invulnerable as it appears")
	}
	full := box.HP()

	w.selectTarget(t, box)
	w.cast(t, siegeBoltID)
	if got := box.HP(); got != full || box.Dead() {
		t.Fatalf("box HP after the bolt = %v (dead %v), want %v", got, box.Dead(), full)
	}

	w.hit(t, box, siegeTwentieth)
	if got, want := box.HP(), full-float64(siegeTwentieth); got != want {
		t.Fatalf("box HP after a barehanded hit = %v, want %v", got, want)
	}
	if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeStatusUpdate, box.ObjectID()); len(got) != 1 {
		t.Fatalf("box StatusUpdate frames after the hit = %d, want 1", len(got))
	}
}

// hallGroundItems is the fixture item catalog with the sweeper's letter
// and potion.
func hallGroundItems() *item.Table {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{hallLetterID, hallPotionID} {
		templates = append(templates, &item.Template{
			ID: id, Name: fmt.Sprint("Item ", id), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Dropable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return item.NewTable(templates)
}

// TestRecycleCanSweepsThenLeaves ports AzitWateringRecycleCan's created,
// see-item and timer hooks: appearing beside a letter of the hot springs
// game and a potion, the sweeper wants to pick up each, the n-th item it
// is told about at 10000 - n, the letter at twice that. It leaves the
// world 5 minutes after it appeared, not before.
func TestRecycleCanSweepsThenLeaves(t *testing.T) {
	t.Parallel()
	items := raidBoot{opts: []gameservertest.Option{gameservertest.WithItemTemplates(hallGroundItems())}}
	w := bootHalls(t, 0, 0, location.Location{}, items, gameservertest.FolkTemplate("Folk", int(recycleCanID)))
	w.srv.SeedGroundItem(t, 0, hallLetterID, 1, w.at.X+100, w.at.Y, w.at.Z)
	w.srv.SeedGroundItem(t, 0, hallPotionID, 1, w.at.X+100, w.at.Y+50, w.at.Z)
	can := w.spawnFolk(t, recycleCanID, 40, 0)

	weights := map[int32]float64{}
	for _, d := range can.Desires().Snapshot() {
		if d.Kind != actorai.IntentionPickUp {
			continue
		}
		obj, ok := w.srv.State.Object(d.ItemObjectID)
		g, isGround := obj.(*grounditem.Item)
		if !ok || !isGround {
			t.Fatalf("pick-up desire on %d, no ground item", d.ItemObjectID)
		}
		weights[g.ItemID()] = d.Weight
	}
	letter, potion := weights[hallLetterID], weights[hallPotionID]
	if !(letter == 20000 && potion == 9999) && !(letter == 19998 && potion == 10000) {
		t.Fatalf("pick-up weights = %v, want the letter at 2 * (10000 - n) and the potion at 10000 - n", weights)
	}

	inWorld := func() bool { _, ok := w.srv.State.Object(can.ObjectID()); return ok }
	w.srv.Advance(t, 5*time.Minute-time.Second)
	w.srv.Settle(t)
	if !inWorld() {
		t.Fatal("the sweeper left the world before 5 minutes")
	}
	w.srv.Advance(t, time.Second)
	w.srv.Settle(t)
	if inWorld() {
		t.Fatal("the sweeper is still in the world 5 minutes after it appeared")
	}
}
