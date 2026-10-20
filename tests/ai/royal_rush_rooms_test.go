package ai

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/royalrush"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Royal Rush room NPCs the tests spawn: the charms, the key box, the
// trigger boxes, the NPC every room npcmaker holds, and the charms' status
// effect skill and the key box's key.
const (
	rrCharmID         = int32(18196)
	rrCharm2ID        = int32(18197)
	rrCharm3ID        = int32(18198)
	rrCharm4ID        = int32(18199)
	rrKeyBoxID        = int32(31455)
	rrTrigger1ID      = int32(31468)
	rrTrigger1NoneID  = int32(31473) // a second TriggerBox1 id
	rrTrigger1PlainID = int32(31478) // a third
	rrTrigger2ID      = int32(31469)
	rrTrigger3ID      = int32(31470)
	rrTrigger4ID      = int32(31471)
	rrTrigger5ID      = int32(31472)
	rrRoomNPCID       = int32(29811)
	rrStatusSkillID   = modelskill.ID(4628)
	rrSepulcherKey    = int32(7260)
)

// rrRoomMakers are the room npcmakers of the trigger tests, each holding
// one room NPC at x = -3900 + 100 * its index, y = -3900: three a box's AI
// parameters name, then one per name the boxes fall back on.
var rrRoomMakers = []string{
	"rr_room_1", "rr_room_2", "rr_room_3",
	"1rd_type1_a", "1rd_type2_a", "1rd_type3_a",
	"1rd_type1_b1", "1rd_type2_b1",
	"1rd_type1_c", "1rd_type2_c",
	"1rd_type1_d", "1rd_type2_d", "1rd_type3_d",
	"1rd_type1_e", "1rd_type2_e",
}

// rrRooms is the spawnlist of rrRoomMakers, default makers that fill on
// start.
func rrRooms() string {
	out := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rr_rooms" minZ="-1000" maxZ="1000"><node x="-4000" y="-4000"/><node x="-2000" y="-4000"/><node x="-2000" y="-3000"/><node x="-4000" y="-3000"/></territory>
`
	for i, name := range rrRoomMakers {
		out += fmt.Sprintf(`	<npcmaker name=%q territory="rr_rooms" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="%d" total="1" pos="%d;-3900;0;0" respawn="0sec"/>
	</npcmaker>
`, name, rrRoomNPCID, -3900+100*i)
	}
	return out + "</list>"
}

// bootRoyalRushRooms boots one character beside the templates and the room
// npcmakers, with the Royal Rush chain registered in scripts.xml order, the
// charms casting their status effect skill (reused after 60 s), the key
// box's key in the item catalog, a director script to read the clock
// through, and every draw 0 until a test queues others.
func bootRoyalRushRooms(t *testing.T, templates ...*npc.Template) *ind8World {
	t.Helper()
	chain := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAfflict", royalrush.Afflict},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAreaController", royalrush.AreaController},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushDefaultNpc", royalrush.DefaultNpc},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushKeyBox", royalrush.KeyBox},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTrasureBox", royalrush.TreasureBox},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox1", royalrush.TriggerBox1},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox2", royalrush.TriggerBox2},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox3", royalrush.TriggerBox3},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox4", royalrush.TriggerBox4},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBox5", royalrush.TriggerBox5},
		{"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTriggerBoxBase.RoyalRushTriggerBoxBase", royalrush.TriggerBoxBase},
	}
	catalog := script.Catalog{}
	list := make([]script.Listing, 0, len(chain)+1)
	for _, c := range chain {
		catalog[c.path] = c.ctor
		list = append(list, script.Listing{Path: c.path})
	}
	// Both worlds' clock reads go through a director of their own name.
	for _, director := range []string{"test.Ind1Director", "test." + ind8DirectorName} {
		catalog[director] = func() script.Script { return script.Script{} }
		list = append(list, script.Listing{Path: director})
	}

	templates = append(templates, warriorTemplate(rrRoomNPCID, nil))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
		if tmpl.Type == "Folk" {
			kinds[int32(tmpl.ID)] = script.KindFolk
		}
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rooms.xml"), []byte(rrRooms()), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	rooms, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	status := ind1Skill(rrStatusSkillID, 0, true)
	status.ReuseDelay = 60000
	items := append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: rrSepulcherKey, Name: "Sepulcher Key", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})

	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable([]modelskill.Definition{status})),
		gameservertest.WithNpcSpawns(rooms),
		gameservertest.WithItemTemplates(item.NewTable(items)),
	}}, templates...)
	if !w.srv.DrivesClock() {
		t.Fatal("setup: the test does not drive the clock")
	}
	return &ind8World{&ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}}
}

// rrCharm is a fixture charm of id whose "type" AI parameter is typ,
// listing its status effect skill.
func rrCharm(id int32, typ int) *npc.Template {
	tmpl := warriorTemplate(id, npc.AIParams{"type": fmt.Sprint(typ)})
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypeStatusEffect: {ID: rrStatusSkillID, Level: 1}}
	return tmpl
}

// rrStatusWeight returns the weight of h's queued desire to cast its
// status effect skill on itself, 0 for none.
func rrStatusWeight(t *testing.T, h *npc.Hostile) float64 {
	t.Helper()
	d, ok := ind1Casts(h)[rrStatusSkillID]
	if !ok {
		return 0
	}
	if d.FinalTarget.ObjectID() != h.ObjectID() {
		t.Fatalf("status effect cast aimed at %d, want the charm itself (%d)", d.FinalTarget.ObjectID(), h.ObjectID())
	}
	return d.Weight
}

// rrRoom returns the object id of the NPC rrRoomMakers[i] holds, 0 for
// none.
func (w *ind8World) rrRoom(i int) int32 {
	for _, h := range w.liveNPCs(rrRoomNPCID) {
		if at := ind8At(h); at.X == -3900+100*i && at.Y == -3900 {
			return h.ObjectID()
		}
	}
	return 0
}

// rrRoomsNow returns the object ids of every room's NPC.
func (w *ind8World) rrRoomsNow() []int32 {
	out := make([]int32, len(rrRoomMakers))
	for i := range rrRoomMakers {
		out[i] = w.rrRoom(i)
	}
	return out
}

// rrDied kills h on its own queue and returns the frames sent until its
// dying hooks have run, 3 s on.
func (w *ind8World) rrDied(t *testing.T, h *npc.Hostile) [][]byte {
	t.Helper()
	w.onQueue(t, h, func() { script.NPCOf(h).DoDie(nil) })
	frames := w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Advance(t, 3*time.Second)
	return append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
}

// TestRoyalRushAreaControllerCastsItsStatusEffect ports
// RoyalRushAreaController's created and timer hooks for each "type": types
// 0, 1 and 3 shout their opening line as they appear and want nothing; 2,
// 1 and 3 minutes on, timer 3001 makes them want their status effect skill
// on themselves (weight 1000000) and shout their second line. Type 2 wants
// the skill and shouts at once. From then on timer 3002, every 30 s, wants
// the skill again, the weight folding into the queued desire.
func TestRoyalRushAreaControllerCastsItsStatusEffect(t *testing.T) {
	t.Parallel()
	w := bootRoyalRushRooms(t, rrCharm(rrCharmID, 0), rrCharm(rrCharm2ID, 1), rrCharm(rrCharm3ID, 2), rrCharm(rrCharm4ID, 3))
	w.toMinute(t, 10, 0)

	type charm struct {
		h            *npc.Hostile
		delay        time.Duration
		open, second int32
	}
	var charms []charm
	for i, c := range []struct {
		id           int32
		delay        time.Duration
		open, second int32
	}{
		{rrCharm2ID, time.Minute, 1010473, 1010476},
		{rrCharmID, 2 * time.Minute, 1010474, 1010477},
		{rrCharm3ID, 0, 1010472, 0},
		{rrCharm4ID, 3 * time.Minute, 1010475, 1010478},
	} {
		obj, frames := w.place(t, c.id, 150, 100*i)
		h := obj.(*npc.Hostile)
		if got := ind8Shouts(frames, h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.open)}) {
			t.Fatalf("charm %d shouts as it appears = %q, want the %d line", c.id, got, c.open)
		}
		want := 0.0
		if c.delay == 0 {
			want = 1000000
		}
		if got := rrStatusWeight(t, h); got != want {
			t.Fatalf("charm %d status effect weight as it appears = %v, want %v", c.id, got, want)
		}
		charms = append(charms, charm{h, c.delay, c.open, c.second})
	}

	// The charms appeared at the same instant; each step below advances to
	// the next charm's timer 3001, the earliest first.
	start := w.now(t)
	for _, c := range charms {
		if c.delay == 0 {
			continue
		}
		w.srv.Advance(t, start.Add(c.delay-time.Millisecond).Sub(w.now(t)))
		w.srv.ReadQueued(t, w.srv.Client)
		if got := rrStatusWeight(t, c.h); got != 0 {
			t.Fatalf("charm %d status effect weight before timer 3001 = %v, want 0", c.h.NpcID(), got)
		}
		w.srv.Advance(t, time.Millisecond)
		frames := w.srv.ReadQueued(t, w.srv.Client)
		if got := rrStatusWeight(t, c.h); got != 1000000 {
			t.Fatalf("charm %d status effect weight after timer 3001 = %v, want 1000000", c.h.NpcID(), got)
		}
		if got := ind8Shouts(frames, c.h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.second)}) {
			t.Fatalf("charm %d shouts on timer 3001 = %q, want the %d line", c.h.NpcID(), got, c.second)
		}
	}

	// Type 2 has had timer 3002 since it appeared, 3 minutes ago: six
	// rounds on top of its created want. The others, 3002 since their 3001.
	for _, c := range charms {
		rounds := (3*time.Minute - c.delay) / (30 * time.Second)
		if got, want := rrStatusWeight(t, c.h), float64(1+rounds)*1000000; got != want {
			t.Fatalf("charm %d status effect weight 3 minutes on = %v, want %v (%d rounds of timer 3002)", c.h.NpcID(), got, want, rounds)
		}
	}
}

// TestRoyalRushAreaControllerRecastsAndShoutsAsItDies ports
// RoyalRushAreaController's skill finished and dying hooks: once a cast
// ends, a charm wants its status effect skill again (weight 1000000) even
// though it is in reuse, where timer 3002's checked want is refused; its
// dying hook, 3 s after its death, makes each type shout its last line.
func TestRoyalRushAreaControllerRecastsAndShoutsAsItDies(t *testing.T) {
	t.Parallel()
	w := bootRoyalRushRooms(t, rrCharm(rrCharmID, 0), rrCharm(rrCharm2ID, 1), rrCharm(rrCharm3ID, 2), rrCharm(rrCharm4ID, 3))
	w.toMinute(t, 10, 0)
	obj, _ := w.place(t, rrCharm3ID, 150, 0)
	h := obj.(*npc.Hostile)

	frames := w.ticks(t, 2)
	frames = append(frames, w.advanceFrames(t, 2*time.Second)...)
	if got := skillUses(frames, h.ObjectID()); len(got) != 1 || got[0] != rrStatusSkillID {
		t.Fatalf("charm casts = %v, want its status effect skill once", got)
	}
	if got := rrStatusWeight(t, h); got != 1000000 {
		t.Fatalf("status effect weight after the cast = %v, want 1000000: the skill finished want skips the reuse", got)
	}
	w.srv.Advance(t, 30*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := rrStatusWeight(t, h); got != 1000000 {
		t.Fatalf("status effect weight after timer 3002 in reuse = %v, want 1000000: the checked want is refused", got)
	}

	for i, c := range []struct {
		id   int32
		last int32
	}{{rrCharmID, 1010481}, {rrCharm2ID, 1010480}, {rrCharm3ID, 1010479}, {rrCharm4ID, 1010482}} {
		obj, _ := w.place(t, c.id, -150, 100*i)
		victim := obj.(*npc.Hostile)
		frames := w.rrDied(t, victim)
		if got := ind8Shouts(frames, victim.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.last)}) {
			t.Fatalf("charm %d shouts as it dies = %q, want the %d line", c.id, got, c.last)
		}
	}
}

// TestRoyalRushKeyBoxGivesItsKeyOnce ports RoyalRushKeyBox: talked to, the
// box gives the talker one Sepulcher Key and leaves the world; a box whose
// first int slot says it gave its key gives none and leaves. Like the
// other Royal Rush NPCs, a key box left standing leaves at minute 54.
func TestRoyalRushKeyBoxGivesItsKeyOnce(t *testing.T) {
	t.Parallel()
	w := bootRoyalRushRooms(t, ind8Folk(rrKeyBoxID, nil))
	w.toMinute(t, 10, 0)
	obj, _ := w.place(t, rrKeyBoxID, 100, 0)
	box := obj.(*npc.Folk)
	w.selectNPC(t, box)
	frames := w.talk(t, box)
	if got := w.srv.PlayerItemCount(t, w.player.ObjectID(), rrSepulcherKey); got != 1 {
		t.Fatalf("keys after the talk = %d, want 1", got)
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, box.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the box = %d, want 1", len(got))
	}
	if got := framesOf(frames, serverpackets.OpcodeNpcHtmlMessage, box.ObjectID()); len(got) != 0 {
		t.Fatalf("the box showed %d pages, want none", len(got))
	}
	w.talk(t, box)
	if got := w.srv.PlayerItemCount(t, w.player.ObjectID(), rrSepulcherKey); got != 1 {
		t.Fatalf("keys after a second talk = %d, want 1", got)
	}

	// A box that has given its key (first int slot 1) gives none, and
	// still leaves.
	obj, _ = w.place(t, rrKeyBoxID, 50, 0)
	given := obj.(*npc.Folk)
	w.onFolkQueue(t, given, func() { script.NPCOf(given).SetInt(script.SlotIntAI0, 1) })
	w.selectNPC(t, given)
	frames = w.talk(t, given)
	if got := w.srv.PlayerItemCount(t, w.player.ObjectID(), rrSepulcherKey); got != 1 {
		t.Fatalf("keys after a talk to a box that gave its key = %d, want 1", got)
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, given.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the box that gave its key = %d, want 1", len(got))
	}
	if got := framesOf(frames, serverpackets.OpcodeNpcHtmlMessage, given.ObjectID()); len(got) != 0 {
		t.Fatalf("the box that gave its key showed %d pages, want none", len(got))
	}

	obj, _ = w.place(t, rrKeyBoxID, 100, 100)
	left := obj.(*npc.Folk)
	w.toMinute(t, 53, 59)
	if _, ok := w.srv.State.Object(left.ObjectID()); !ok {
		t.Fatal("the key box left before minute 54")
	}
	w.toMinute(t, 54, 1)
	if _, ok := w.srv.State.Object(left.ObjectID()); ok {
		t.Fatal("the key box is still in the world at minute 54")
	}
}

// TestRoyalRushTriggerBoxStartsItsRoom ports the trigger boxes' first talk.
// Each draws one of its rooms (three for boxes 1 and 4, two for the
// others) and runs maker script events 1000 and 1001 on the npcmaker its
// room_event_<n> AI parameter names, or its fallback name: the room's NPC
// leaves and a new one fills the room. Then the box leaves the world,
// showing no page. A box with no such npcmaker only leaves.
func TestRoyalRushTriggerBoxStartsItsRoom(t *testing.T) {
	t.Parallel()
	named := npc.AIParams{"room_event_1": "rr_room_1", "room_event_2": "rr_room_2", "room_event_3": "rr_room_3"}
	w := bootRoyalRushRooms(t, ind8Folk(rrTrigger1ID, named), ind8Folk(rrTrigger2ID, nil), ind8Folk(rrTrigger3ID, nil),
		ind8Folk(rrTrigger4ID, nil), ind8Folk(rrTrigger5ID, nil), ind8Folk(rrTrigger1NoneID, npc.AIParams{"room_event_1": "rr_none"}),
		ind8Folk(rrTrigger1PlainID, nil))
	w.toMinute(t, 10, 0)

	for _, c := range []struct {
		id   int32
		draw int
		room int // index into rrRoomMakers
	}{
		{rrTrigger1ID, 2, 2},
		{rrTrigger1ID, 0, 0},
		{rrTrigger1ID, 1, 1},
		{rrTrigger1PlainID, 0, 3},
		{rrTrigger1PlainID, 1, 4},
		{rrTrigger1PlainID, 2, 5},
		{rrTrigger2ID, 0, 6},
		{rrTrigger2ID, 1, 7},
		{rrTrigger3ID, 0, 8},
		{rrTrigger3ID, 1, 9},
		{rrTrigger4ID, 0, 10},
		{rrTrigger4ID, 1, 11},
		{rrTrigger4ID, 2, 12},
		{rrTrigger5ID, 0, 13},
		{rrTrigger5ID, 1, 14},
	} {
		before := w.rrRoomsNow()
		if before[c.room] == 0 {
			t.Fatalf("room %s holds no NPC before box %d's talk", rrRoomMakers[c.room], c.id)
		}
		obj, _ := w.place(t, c.id, 100, 0)
		box := obj.(*npc.Folk)
		w.selectNPC(t, box)
		w.draws.set(0, c.draw)
		frames := w.talk(t, box)
		w.srv.Settle(t)
		after := w.rrRoomsNow()
		for i := range rrRoomMakers {
			switch {
			case i == c.room && (after[i] == 0 || after[i] == before[i]):
				t.Fatalf("box %d on draw %d: room %s NPC %d -> %d, want a new one", c.id, c.draw, rrRoomMakers[i], before[i], after[i])
			case i != c.room && after[i] != before[i]:
				t.Fatalf("box %d on draw %d: room %s changed %d -> %d", c.id, c.draw, rrRoomMakers[i], before[i], after[i])
			}
		}
		if got := framesOf(frames, serverpackets.OpcodeDeleteObject, box.ObjectID()); len(got) != 1 {
			t.Fatalf("box %d DeleteObject frames = %d, want 1", c.id, len(got))
		}
		if got := framesOf(frames, serverpackets.OpcodeNpcHtmlMessage, box.ObjectID()); len(got) != 0 {
			t.Fatalf("box %d showed %d pages, want none", c.id, len(got))
		}
	}

	// A box naming an npcmaker there is none of starts nothing and leaves.
	before := w.rrRoomsNow()
	obj, _ := w.place(t, rrTrigger1NoneID, 100, 0)
	box := obj.(*npc.Folk)
	w.selectNPC(t, box)
	w.draws.set(0, 0)
	frames := w.talk(t, box)
	w.srv.Settle(t)
	if got := w.rrRoomsNow(); !slices.Equal(got, before) {
		t.Fatalf("rooms after a talk naming no npcmaker = %v, want %v unchanged", got, before)
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, box.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the box naming no npcmaker = %d, want 1", len(got))
	}
}

// TestRoyalRushTriggerBoxWaitsOutTheClosingMinutes ports the trigger
// boxes' closing check and their guard: in minutes 50 to 59 a talk makes
// the box say the rooms are closed and changes nothing, the box staying;
// a box already started (first int slot 1) stays silent and starts
// nothing. A trigger box, whose created hook never runs the minute-54
// check, stays past minute 54.
func TestRoyalRushTriggerBoxWaitsOutTheClosingMinutes(t *testing.T) {
	t.Parallel()
	named := npc.AIParams{"room_event_1": "rr_room_1", "room_event_2": "rr_room_2", "room_event_3": "rr_room_3"}
	w := bootRoyalRushRooms(t, ind8Folk(rrTrigger1ID, named))
	w.toMinute(t, 49, 59)
	obj, _ := w.place(t, rrTrigger1ID, 100, 0)
	box := obj.(*npc.Folk)
	w.selectNPC(t, box)
	before := w.rrRoomsNow()

	w.toMinute(t, 50, 0)
	frames := w.talk(t, box)
	if got := rrSays(frames, box.ObjectID()); !slices.Equal(got, []string{script.NpcString(1010552)}) {
		t.Fatalf("box lines in minute 50 = %q, want the 1010552 line", got)
	}
	w.toMinute(t, 54, 5)
	frames = w.talk(t, box)
	if got := rrSays(frames, box.ObjectID()); !slices.Equal(got, []string{script.NpcString(1010552)}) {
		t.Fatalf("box lines in minute 54 = %q, want the 1010552 line", got)
	}
	if _, ok := w.srv.State.Object(box.ObjectID()); !ok {
		t.Fatal("the trigger box left in the closing minutes")
	}
	if got := ind8Slot(box, script.SlotIntAI0); got != 0 {
		t.Fatalf("box first int slot after the closed talks = %d, want 0", got)
	}

	w.toMinute(t, 0, 0)
	w.onFolkQueue(t, box, func() { script.NPCOf(box).SetInt(script.SlotIntAI0, 1) })
	frames = w.talk(t, box)
	if got := rrSays(frames, box.ObjectID()); len(got) != 0 {
		t.Fatalf("a started box says %q, want nothing", got)
	}
	if _, ok := w.srv.State.Object(box.ObjectID()); !ok {
		t.Fatal("a started box left on a talk")
	}
	if got := w.rrRoomsNow(); !slices.Equal(got, before) {
		t.Fatalf("rooms after the refused talks = %v, want %v unchanged", got, before)
	}
}

// rrSays returns the plain lines (say type 0) NpcSay frames show from
// objectID.
func rrSays(frames [][]byte, objectID int32) []string {
	var out []string
	for _, f := range framesOf(frames, serverpackets.OpcodeNpcSay, objectID) {
		if int32(binary.LittleEndian.Uint32(f[5:9])) == 0 {
			out = append(out, says([][]byte{f}, objectID)...)
		}
	}
	return out
}
