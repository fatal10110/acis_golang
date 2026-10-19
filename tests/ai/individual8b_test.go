package ai

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the second part of the eighth individual behavior batch
// (#193 PR 2), with the NPCs they place.
const (
	ind8OfferingID  = int32(32038) // SacrificialVictim, Folk
	ind8SacrificeID = int32(22145) // the Ritual Sacrifice it leaves
	ind8CameraID    = int32(13015) // TriolCamera, Folk
	ind8ManagerID   = int32(31921) // RoyalRushNpc, Folk
	ind8Manager2ID  = int32(31922) // RoyalRushNpc, Folk
	ind8LockID      = int32(31925) // the first of a manager's five lock NPCs
	ind8RoomNPCID   = int32(29805) // the NPC of a sepulcher room npcmaker
)

// ind8At is where c stands.
func ind8At(c attackable.Combatant) location.Location {
	x, y, z := c.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// ind8Socials returns the social animation ids SocialAction frames show
// from objectID.
func ind8Socials(frames [][]byte, objectID int32) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeSocialAction, objectID) {
		out = append(out, int32(binary.LittleEndian.Uint32(f[5:9])))
	}
	return out
}

// TestSacrificialVictimLeavesItsSacrifice runs SacrificialVictim's
// script-event and timer hooks. Any event but 3 does nothing. Event 3
// makes the Ritual Offering play its social action 1 ten seconds later
// and, 5 s after that, leave the Ritual Sacrifice where it stood as its
// private and leave the world; the sacrifice stays.
func TestSacrificialVictimLeavesItsSacrifice(t *testing.T) {
	t.Parallel()
	w := bootIndividual8(t, raidBoot{}, ind8Folk(ind8OfferingID, nil), warriorTemplate(ind8SacrificeID, nil))
	c, _ := w.place(t, ind8OfferingID, 100, 0)
	offering := c.(*npc.Folk)
	at := ind8At(offering)

	w.send(t, offering, 2, 0, 0)
	w.srv.Advance(t, 20*time.Second)
	if got := ind8Socials(w.ticks(t, 2), offering.ObjectID()); len(got) != 0 || len(w.liveNPCs(ind8SacrificeID)) != 0 {
		t.Fatalf("after event 2: socials %v, %d sacrifices; want nothing", got, len(w.liveNPCs(ind8SacrificeID)))
	}

	w.send(t, offering, 3, 0, 0)
	w.srv.Advance(t, 9900*time.Millisecond)
	if got := ind8Socials(w.ticks(t, 1), offering.ObjectID()); len(got) != 0 {
		t.Fatalf("socials before 10 s = %v, want none", got)
	}
	w.srv.Advance(t, 100*time.Millisecond)
	if got := ind8Socials(w.ticks(t, 1), offering.ObjectID()); !slices.Equal(got, []int32{1}) {
		t.Fatalf("socials at 10 s = %v, want [1]", got)
	}
	w.srv.Advance(t, 4900*time.Millisecond)
	if len(w.liveNPCs(ind8SacrificeID)) != 0 || offering.Decayed() {
		t.Fatal("the offering left before 5 s more")
	}
	w.srv.Advance(t, 100*time.Millisecond)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if !offering.Decayed() || len(framesOf(frames, serverpackets.OpcodeDeleteObject, offering.ObjectID())) != 1 {
		t.Fatal("the offering is still in the world 15 s after event 3; want it deleted")
	}
	sacrifices := w.liveNPCs(ind8SacrificeID)
	if len(sacrifices) != 1 || ind8At(sacrifices[0]).Distance2D(at) > 1 {
		t.Fatalf("sacrifices = %d, want one where the offering stood %+v", len(sacrifices), at)
	}
	w.srv.Advance(t, time.Minute)
	if len(w.liveNPCs(ind8SacrificeID)) != 1 {
		t.Fatal("the sacrifice left the world with its master; want it to stay")
	}
}

// ind8CameraPayload is the SpecialCamera payload, opcode first, of a shot
// centred on objectID: distance, yaw, pitch, time, duration, turn, rise,
// widescreen and the last field.
func ind8CameraPayload(objectID int32, fields ...int32) []byte {
	out := []byte{serverpackets.OpcodeSpecialCamera}
	for _, v := range append([]int32{objectID}, fields...) {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// ind8MusicPayload is the PlaySound payload of music file bound to
// objectID standing at at.
func ind8MusicPayload(objectID int32, file string, at location.Location) []byte {
	out := []byte{serverpackets.OpcodePlaySound}
	out = binary.LittleEndian.AppendUint32(out, 1)
	for _, r := range utf16.Encode([]rune(file)) {
		out = binary.LittleEndian.AppendUint16(out, r)
	}
	out = append(out, 0, 0)
	for _, v := range []int32{1, objectID, int32(at.X), int32(at.Y), int32(at.Z), 0} {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// ind8Pick returns the frames among frames with opcode, in order.
func ind8Pick(frames [][]byte, opcode byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if len(f) > 0 && f[0] == opcode {
			out = append(out, f)
		}
	}
	return out
}

// TestTriolCameraFilmsTheSacrifice runs TriolCamera's created, script-event
// and timer hooks. Event 3 plays BS04_A bound to the camera and keeps the
// creature it names; the five camera shots follow 1 s, 1.3 s, 1.6 s, 11 s
// and 16 s later, and 6 s after the last the camera tells the NPCs within
// 4000 the scene is over (event 4 naming that creature). An event 3 that
// names no creature films the same shots and tells nobody.
func TestTriolCameraFilmsTheSacrifice(t *testing.T) {
	t.Parallel()
	w := bootIndividual8(t, raidBoot{}, ind8Folk(ind8CameraID, nil), warriorTemplate(ind8ListenerID, nil))
	w.spawn(t, ind8ListenerID, 300, 100)
	c, _ := w.place(t, ind8CameraID, 100, 0)
	camera := c.(*npc.Folk)
	id := camera.ObjectID()
	shots := [][]int32{
		{1500, 88, 89, 0, 10000, 0, 110, 1, 0},
		{1500, 88, 89, 0, 10000, 0, 110, 1, 0},
		{450, 88, 3, 5500, 10000, 0, 350, 1, 0},
		{500, 88, 4, 5000, 10000, 0, 0, 1, 0},
		{3000, 88, 4, 6000, 6900, 0, 15, 1, 0},
	}
	gaps := []time.Duration{1000 * time.Millisecond, 300 * time.Millisecond, 300 * time.Millisecond, 9400 * time.Millisecond, 5000 * time.Millisecond}

	film := func(arg1 int32) {
		t.Helper()
		w.send(t, camera, 3, arg1, 0)
		frames := w.srv.ReadQueued(t, w.srv.Client)
		if got := ind8Pick(frames, serverpackets.OpcodePlaySound); len(got) != 1 || !slices.Equal(got[0], ind8MusicPayload(id, "BS04_A", ind8At(camera))) {
			t.Fatalf("PlaySound frames on event 3 = %x, want BS04_A bound to the camera", got)
		}
		if got := ind8Slot(camera, script.SlotIntAI0); got != arg1 {
			t.Fatalf("camera ai0 = %d, want %d", got, arg1)
		}
		for i, gap := range gaps {
			w.srv.Advance(t, gap-time.Millisecond)
			if got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera); len(got) != 0 {
				t.Fatalf("camera frames before shot %d = %x, want none", i+1, got)
			}
			w.srv.Advance(t, time.Millisecond)
			got := ind8Pick(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeSpecialCamera)
			if want := ind8CameraPayload(id, shots[i]...); len(got) != 1 || !slices.Equal(got[0], want) {
				t.Fatalf("camera frames at shot %d = %x, want %x", i+1, got, want)
			}
		}
		w.srv.Advance(t, 5999*time.Millisecond)
		if got := w.heard.take(); len(got) != 0 {
			t.Fatalf("heard before the scene ended: %q, want nothing", got)
		}
		w.srv.Advance(t, time.Millisecond)
	}

	if got := ind8Slot(camera, script.SlotIntAI0); got != 0 {
		t.Fatalf("camera ai0 after its spawn = %d, want 0", got)
	}
	film(4242)
	if got, want := w.heard.take(), []string{"event=4 arg1=4242 arg2=0"}; !slices.Equal(got, want) {
		t.Fatalf("heard as the scene ended: %q, want %q", got, want)
	}
	film(0)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard after a scene naming nobody: %q, want nothing", got)
	}
}

// ind8LockTemplates are the fixture lock NPCs of the two managers: ids
// from first, one per lock.
func ind8LockTemplates(first int32) []*npc.Template {
	var out []*npc.Template
	for i := range int32(5) {
		out = append(out, ind8Folk(first+i, nil))
	}
	return out
}

// ind8LockParams names five lock NPCs, ids from first, as a manager's
// lock_npc_id AI parameters; with placed, it also places lock i at
// 1000*i, 2000, -100 facing 100*i.
func ind8LockParams(first int32, placed bool) npc.AIParams {
	ai := npc.AIParams{}
	for i := 1; i <= 5; i++ {
		ai[fmt.Sprintf("lock_npc_id%d", i)] = fmt.Sprint(first + int32(i-1))
		if placed {
			ai[fmt.Sprintf("lock_x%d", i)] = fmt.Sprint(1000 * i)
			ai[fmt.Sprintf("lock_y%d", i)] = "2000"
			ai[fmt.Sprintf("lock_z%d", i)] = "-100"
			ai[fmt.Sprintf("lock_d%d", i)] = fmt.Sprint(100 * i)
		}
	}
	return ai
}

// TestRoyalRushNpcPlacesItsLocks runs RoyalRushNpc's created hook: a
// manager places the five lock NPCs its AI parameters name as its
// privates, where they place them; one whose lock_x1 is 0 places them at
// the Conquerors' Sepulcher doors instead, facing -32584, each with its
// door number as its first spawn parameter.
func TestRoyalRushNpcPlacesItsLocks(t *testing.T) {
	t.Parallel()
	templates := append(ind8LockTemplates(ind8LockID), ind8LockTemplates(ind8LockID+5)...)
	templates = append(templates, ind8Folk(ind8ManagerID, ind8LockParams(ind8LockID, true)), ind8Folk(ind8Manager2ID, ind8LockParams(ind8LockID+5, false)))
	w := bootIndividual8(t, raidBoot{}, templates...)

	doors := []location.Location{{X: 182727, Y: -85493, Z: -7200}, {X: 184547, Y: -85479, Z: -7200}, {X: 186349, Y: -85473, Z: -7200}, {X: 188154, Y: -85463, Z: -7200}, {X: 189947, Y: -85466, Z: -7200}}
	for _, m := range []struct {
		id     int32
		placed bool
	}{{ind8ManagerID, true}, {ind8Manager2ID, false}} {
		c, _ := w.place(t, m.id, 100, 0)
		manager := c.(*npc.Folk)
		locks := manager.Minions()
		if len(locks) != 5 {
			t.Fatalf("manager %d privates = %d, want 5", m.id, len(locks))
		}
		slices.SortFunc(locks, func(a, b npc.PartyMember) int { return a.NpcID() - b.NpcID() })
		first := ind8LockID
		if !m.placed {
			first += 5
		}
		for i, lock := range locks {
			want, heading, param := location.Location{X: 1000 * (i + 1), Y: 2000, Z: -100}, 100*(i+1), int32(0)
			if !m.placed {
				want, heading, param = doors[i], -32584, int32(i+1)
			}
			if lock.NpcID() != int(first)+i || ind8At(lock) != want || lock.Heading() != heading {
				t.Fatalf("manager %d lock %d = npc %d at %+v facing %d, want npc %d at %+v facing %d",
					m.id, i+1, lock.NpcID(), ind8At(lock), lock.Heading(), int(first)+i, want, heading)
			}
			if got := script.NPCOf(lock).Int(script.SlotParam1); got != param {
				t.Fatalf("manager %d lock %d first spawn parameter = %d, want %d", m.id, i+1, got, param)
			}
		}
	}
}

// ind8Rooms are the sepulcher room npcmakers of the fixture managers, each
// default_maker holding one room NPC.
const ind8Rooms = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="rr_rooms" minZ="-1000" maxZ="1000"><node x="-4000" y="-4000"/><node x="-3000" y="-4000"/><node x="-3000" y="-3000"/><node x="-4000" y="-3000"/></territory>
	<npcmaker name="rr_room_1" territory="rr_rooms" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="29805" total="1" pos="-3900;-3900;0;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="rr_room_2" territory="rr_rooms" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="29805" total="1" pos="-3800;-3900;0;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="rr_room_5" territory="rr_rooms" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="29805" total="1" pos="-3500;-3900;0;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="rr_boss_type2" territory="rr_rooms" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="29805" total="1" pos="-3100;-3900;0;0" respawn="0sec"/>
	</npcmaker>
</list>`

// bootSepulcher boots the managers beside the room npcmakers: the first
// names the rooms and the boss rooms, shouts the minutes and sends the
// players of its territory 3000 east of where the character entered the
// world; the second names nothing. It returns the escape point too.
func bootSepulcher(t *testing.T) (*ind8World, location.Location) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rooms.xml"), []byte(ind8Rooms), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	manager := ind8Folk(ind8ManagerID, npc.AIParams{
		"room_trigger_1": "rr_room_1", "room_trigger_2": "rr_room_2", "room_trigger_5": "rr_room_5", "room_trigger_boss": "rr_boss",
		"ShoutMsg": "1",
	})
	w := bootIndividual8(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}},
		manager, ind8Folk(ind8Manager2ID, nil), warriorTemplate(ind8RoomNPCID, nil))
	// The manager is placed after this, so its spawn reads the escape
	// point set here.
	escape := location.Location{X: w.at.X + 3000, Y: w.at.Y, Z: w.at.Z}
	manager.AIParams["EscapeTelPosX"] = fmt.Sprint(escape.X)
	manager.AIParams["EscapeTelPosY"] = fmt.Sprint(escape.Y)
	manager.AIParams["EscapeTelPosZ"] = fmt.Sprint(escape.Z)
	return w, escape
}

// now reads the clock the scripts run on.
func (w *ind8World) now(t *testing.T) time.Time {
	t.Helper()
	var now time.Time
	if !w.srv.RunScript(t, w.player.ObjectID(), ind8DirectorName, func(s *script.Script, _ *script.Player) { now = s.Now() }) {
		t.Fatal("the clock read panicked")
	}
	return now
}

// toTick advances the clock to the first tick, every second from start,
// that falls in second sec of minute min of an hour, and returns the
// frames sent on the way.
func (w *ind8World) toTick(t *testing.T, start time.Time, minute, sec int) [][]byte {
	t.Helper()
	now := w.now(t)
	at := now.Truncate(time.Hour).Add(time.Duration(minute)*time.Minute + time.Duration(sec)*time.Second)
	if at.Before(now) {
		at = at.Add(time.Hour)
	}
	tick := start.Add((at.Sub(start) + time.Second - 1) / time.Second * time.Second)
	if tick.Minute() != minute || tick.Second() != sec || !tick.After(now) {
		t.Fatalf("tick %v of the timer started %v is not in %02d:%02d after %v", tick, start, minute, sec, now)
	}
	w.srv.Advance(t, tick.Sub(now))
	return w.srv.ReadQueued(t, w.srv.Client)
}

// ind8Shouts returns the shouts NpcSay frames show from objectID.
func ind8Shouts(frames [][]byte, objectID int32) []string {
	var out []string
	for _, f := range framesOf(frames, serverpackets.OpcodeNpcSay, objectID) {
		if int32(binary.LittleEndian.Uint32(f[5:9])) == 1 {
			out = append(out, says([][]byte{f}, objectID)...)
		}
	}
	return out
}

// roomNPC returns the room NPC standing at x, -3900, 0 for none.
func (w *ind8World) roomNPC(x int) int32 {
	for _, h := range w.liveNPCs(ind8RoomNPCID) {
		if at := ind8At(h); at.X == x && at.Y == -3900 {
			return h.ObjectID()
		}
	}
	return 0
}

// TestRoyalRushNpcRunsTheSepulcherHour runs RoyalRushNpc's created and
// timer hooks against the local clock, read every second. At minute 50
// the first manager shouts that the entry closes and sends the character,
// in its territory, to its escape point; the second, with no ShoutMsg and
// no escape point, does neither. In the first 30 s of minute 54 each
// frees its sepulcher (AVQuest0 slot 0). At minute 55 both shout the two
// closing lines. In the first seconds of the hour the first clears its
// first room's npcmaker and refills it 5 s later. At minute 5 the first
// shouts the minutes gone.
func TestRoyalRushNpcRunsTheSepulcherHour(t *testing.T) {
	t.Parallel()
	w, escape := bootSepulcher(t)
	c, _ := w.place(t, ind8ManagerID, 100, 0)
	first := c.(*npc.Folk)
	start := w.now(t)
	c, _ = w.place(t, ind8Manager2ID, -100, 0)
	second := c.(*npc.Folk)
	start2 := w.now(t)
	if start2.Sub(start)%time.Second != 0 {
		t.Fatalf("the managers' timers are out of step: %v, %v", start, start2)
	}
	w.toTick(t, start, 49, 59)
	for _, m := range []*npc.Folk{first, second} {
		script.NPCOf(m).SetInt(npc.IntAVQuest0, 7)
	}
	frames := w.toTick(t, start, 50, 0)
	if got := ind8Shouts(frames, first.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000457)}) {
		t.Fatalf("first manager's shouts at minute 50 = %q, want the 1000457 line", got)
	}
	if got := ind8Shouts(frames, second.ObjectID()); len(got) != 0 {
		t.Fatalf("second manager's shouts at minute 50 = %q, want none: ShoutMsg is 0", got)
	}
	w.srv.Settle(t)
	if x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); (location.Location{X: x, Y: y}).Distance2D(location.Location{X: escape.X, Y: escape.Y}) > 150 {
		t.Fatalf("the character stands at %d, %d after minute 50, want within 100 of the escape point %+v", x, y, escape)
	}
	// The client appears where it was sent, as a real one does.
	w.srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)

	w.toTick(t, start, 53, 59)
	if got := ind8Slot(first, npc.IntAVQuest0); got != 7 {
		t.Fatalf("AVQuest0 before minute 54 = %d, want 7", got)
	}
	w.toTick(t, start, 54, 0)
	for _, m := range []*npc.Folk{first, second} {
		if got := ind8Slot(m, npc.IntAVQuest0); got != 0 {
			t.Fatalf("manager %d AVQuest0 at minute 54 = %d, want 0", m.NpcID(), got)
		}
	}

	frames = w.toTick(t, start, 55, 0)
	for _, m := range []*npc.Folk{first, second} {
		if got := ind8Shouts(frames, m.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000500), script.NpcString(1000501)}) {
			t.Fatalf("manager %d shouts at minute 55 = %q, want the 1000500 and 1000501 lines", m.NpcID(), got)
		}
	}

	// The run may have started late in an hour, so the first room's NPC
	// is read after the climb to minute 55, past any earlier reset.
	room := w.roomNPC(-3900)
	if room == 0 {
		t.Fatal("the first room's npcmaker holds no NPC")
	}
	w.toTick(t, start, 59, 59)
	if got := w.roomNPC(-3900); got != room {
		t.Fatalf("first room NPC before the hour = %d, want %d", got, room)
	}
	w.toTick(t, start, 0, 0)
	if got := w.roomNPC(-3900); got != 0 {
		t.Fatalf("first room NPC as the hour starts = %d, want none", got)
	}
	if got := ind8Slot(first, script.SlotIntAI1); got != 1 {
		t.Fatalf("first manager ai1 as the hour starts = %d, want 1", got)
	}
	w.srv.Advance(t, 5*time.Second)
	w.srv.Settle(t)
	if got := w.roomNPC(-3900); got == 0 || got == room {
		t.Fatalf("first room NPC 5 s into the hour = %d, want a new one", got)
	}
	if got := ind8Slot(first, script.SlotIntAI1); got != 0 {
		t.Fatalf("first manager ai1 5 s into the hour = %d, want 0", got)
	}

	frames = w.toTick(t, start, 5, 0)
	want := script.NpcString(1000455) + "5" + script.NpcString(1000456)
	if got := ind8Shouts(frames, first.ObjectID()); !slices.Equal(got, []string{want}) {
		t.Fatalf("first manager's shouts at minute 5 = %q, want %q", got, want)
	}
	if got := ind8Shouts(frames, second.ObjectID()); len(got) != 0 {
		t.Fatalf("second manager's shouts at minute 5 = %q, want none", got)
	}
}

// TestRoyalRushNpcResetsItsRooms runs RoyalRushNpc's script-event hook:
// events 1001 to 1004 clear and refill the npcmaker of room 2 to 5 the
// manager names, 1005 the boss room its draw of 4 picks (type 1 to 4);
// a room it does not name, and any other event, change nothing.
func TestRoyalRushNpcResetsItsRooms(t *testing.T) {
	t.Parallel()
	w, _ := bootSepulcher(t)
	c, _ := w.place(t, ind8ManagerID, 100, 0)
	manager := c.(*npc.Folk)
	c, _ = w.place(t, ind8Manager2ID, -100, 0)
	bare := c.(*npc.Folk)
	rooms := func() []int32 {
		return []int32{w.roomNPC(-3900), w.roomNPC(-3800), w.roomNPC(-3500), w.roomNPC(-3100)}
	}
	before := rooms()
	if slices.Contains(before, 0) {
		t.Fatalf("room NPCs at boot = %v, want one in each room", before)
	}
	step := func(target *npc.Folk, eventID int32, changed int) {
		t.Helper()
		w.send(t, target, eventID, 0, 0)
		w.srv.Advance(t, 0)
		after := rooms()
		for i := range after {
			if after[i] == 0 || (i == changed) == (after[i] == before[i]) {
				t.Fatalf("room NPCs after event %d to npc %d = %v from %v, want room %d alone replaced", eventID, target.NpcID(), after, before, changed)
			}
		}
		before = after
	}
	step(manager, 1001, 1)
	step(manager, 1004, 2)
	w.draws.set(1)
	step(manager, 1005, 3)
	w.draws.set(0)
	step(manager, 1002, -1)
	step(manager, 1006, -1)
	step(bare, 1001, -1)
}

// The quests the tamed beasts read, and the beasts' fixture skills, items
// and NPCs.
const (
	ind8Q020          = "Q020_BringUpWithLove"
	ind8Q655          = "Q655_AGrandPlanForTamingWildBeasts"
	ind8BuffaloID     = int32(16013) // WarriorPetForPC
	ind8CougarID      = int32(16015) // WarriorPetForPC
	ind8TamerID       = int32(29804) // the master the fixture beasts are placed beside
	ind8HerdMateID    = int32(29806) // an NPC of the beasts' clan with no behavior
	ind8GoldenSpice   = int32(6643)
	ind8CrystalSpice  = int32(6644)
	ind8Jewel         = int32(7185)
	ind8Crystal       = int32(8084)
	ind8PetBuff1      = modelskill.ID(5101)
	ind8PetDebuff1    = modelskill.ID(5111)
	ind8PetDebuff2    = modelskill.ID(5112)
	ind8PetHeal       = modelskill.ID(5120)
	ind8PetTakeSocial = 92
)

// ind8QuestStub is a quest named name with id and no hook.
func ind8QuestStub(name string, id int32) func() script.Script {
	return func() script.Script { return script.Script{Name: name, QuestID: id} }
}

// ind8PetSkills are the tamed beasts' five buffs, two debuffs and heal.
// Each buff and debuff lands an effect of a stack type of its own.
func ind8PetSkills() []modelskill.Definition {
	held := func(d modelskill.Definition) modelskill.Definition {
		d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: fmt.Sprint("ind8_pet_", d.ID), StackOrder: 1}}
		return d
	}
	var out []modelskill.Definition
	for i := range modelskill.ID(5) {
		d := ind1Skill(ind8PetBuff1+i, 600, false)
		d.Offensive, d.Magic, d.SkillType, d.Power = false, false, "BUFF", 0
		out = append(out, held(d))
	}
	heal := ind1Skill(ind8PetHeal, 600, false)
	heal.Offensive, heal.SkillType = false, "HEAL"
	return append(out, held(ind1Skill(ind8PetDebuff1, 600, false)), held(ind1Skill(ind8PetDebuff2, 600, false)), heal)
}

// ind8PetTemplate is a fixture tamed beast of id of the beast clan, with
// the fixture skills under its skill types and ai as its AI parameters.
func ind8PetTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.Clans, tmpl.ClanRange = []string{"beast"}, 500
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeBuff1: {ID: ind8PetBuff1, Level: 1}, npc.SkillTypeBuff2: {ID: ind8PetBuff1 + 1, Level: 1},
		npc.SkillTypeBuff3: {ID: ind8PetBuff1 + 2, Level: 1}, npc.SkillTypeBuff4: {ID: ind8PetBuff1 + 3, Level: 1},
		npc.SkillTypeBuff5: {ID: ind8PetBuff1 + 4, Level: 1}, npc.SkillTypeDebuff1: {ID: ind8PetDebuff1, Level: 1},
		npc.SkillTypeDebuff2: {ID: ind8PetDebuff2, Level: 1}, npc.SkillTypeHeal: {ID: ind8PetHeal, Level: 1},
	}
	return tmpl
}

// ind8PetItems is the fixture item catalog with the spices, the Jewel of
// Innocence and the Crystal of Purity.
func ind8PetItems() *item.Table {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{ind8GoldenSpice, ind8CrystalSpice, ind8Jewel, ind8Crystal} {
		templates = append(templates, &item.Template{
			ID: id, Name: fmt.Sprint("Item ", id), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Dropable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return item.NewTable(templates)
}

// bootTamer boots the tamed beasts beside their master, a beast clan-mate
// and the listener, with the beasts' items and opts, and spawns the master
// 600 east of the character and the listener; ready runs as raidBoot's.
func bootTamer(t *testing.T, ready func(srv *gameservertest.Server, objID int32), opts ...gameservertest.Option) (*ind8World, *npc.Hostile) {
	t.Helper()
	mate := warriorTemplate(ind8HerdMateID, nil)
	mate.Clans, mate.ClanRange = []string{"beast"}, 500
	base := []gameservertest.Option{gameservertest.WithItemTemplates(ind8PetItems()), gameservertest.WithScriptSkills(ind8PetSkills()...)}
	w := bootIndividual8(t, raidBoot{opts: append(base, opts...), ready: ready},
		ind8PetTemplate(ind8BuffaloID, npc.AIParams{"TakeSocial": fmt.Sprint(ind8PetTakeSocial)}), ind8PetTemplate(ind8CougarID, nil),
		warriorTemplate(ind8TamerID, nil), mate, warriorTemplate(ind8ListenerID, nil))
	master := w.spawn(t, ind8TamerID, 600, 600)
	w.spawn(t, ind8ListenerID, 300, 100)
	return w, master
}

// tame places a beast of id dx east of the character as master's private,
// its first spawn parameter the character and its second spice, and
// returns it with the frames its spawn sent.
func (w *ind8World) tame(t *testing.T, master *npc.Hostile, id, spice int32, dx int) (*npc.Hostile, [][]byte) {
	t.Helper()
	var pet *script.NPC
	if !w.srv.RunScript(t, w.player.ObjectID(), ind8DirectorName, func(s *script.Script, p *script.Player) {
		pet = s.Spawner().CreateOnePrivateEx(script.NPCOf(master), id, script.Loc{X: w.at.X + dx, Y: w.at.Y, Z: w.at.Z}, 0, script.Params{P1: p.ObjectID(), P2: spice})
	}) || pet == nil {
		t.Fatalf("tame %d: no beast placed", id)
	}
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	for _, h := range w.liveNPCs(id) {
		if h.ObjectID() == pet.ObjectID() {
			return h, frames
		}
	}
	t.Fatalf("tame %d: beast %d not in the world", id, pet.ObjectID())
	return nil, nil
}

// startQuest gives the character a started state of the quest stub name
// at cond.
func (w *ind8World) startQuest(t *testing.T, name string, cond int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), name, func(s *script.Script, p *script.Player) {
		st := s.NewQuestState(p)
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(cond)
	}) {
		t.Fatalf("start %s panicked", name)
	}
	w.srv.Settle(t)
}

// questCond reads the cond of the character's state in the quest name.
func (w *ind8World) questCond(t *testing.T, name string) int32 {
	t.Helper()
	var cond int32
	if !w.srv.RunScript(t, w.player.ObjectID(), name, func(s *script.Script, p *script.Player) {
		cond = s.QuestState(p, name).Cond()
	}) {
		t.Fatalf("read %s panicked", name)
	}
	return cond
}

// TestWarriorPetForPCClaimsItsOwner runs WarriorPetForPC's created,
// script-event and timer hooks. A beast whose first spawn parameter names
// the character keeps him as its owner and, on its 5 in 100 draw, names
// him in one of its five greetings; it eats Golden Spice for skill 2188,
// Crystal Spice otherwise, and tells the NPCs within 1500 that it is his
// newest (event 10022 with his object id and the game tick it came at). A
// claim of his naming a later tick makes an older beast leave, one of the
// same tick or of another player does not (the game tick counts the wall
// clock's seconds, so two beasts tamed here come at one tick unless a
// second turns between them); every 5 s a beast tells
// the NPCs within 700 that it is there (event 10017 with its object id),
// and every 15 s repeats its claim within 500. One whose parameter names
// no player does none of that.
func TestWarriorPetForPCClaimsItsOwner(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, nil)
	owner := w.player.ObjectID()

	nobody, frames := func() (*npc.Hostile, [][]byte) {
		t.Helper()
		var pet *script.NPC
		if !w.srv.RunScript(t, owner, ind8DirectorName, func(s *script.Script, _ *script.Player) {
			pet = s.Spawner().CreateOnePrivateEx(script.NPCOf(master), ind8CougarID, script.Loc{X: w.at.X, Y: w.at.Y + 100, Z: w.at.Z}, 0, script.Params{P1: 999999})
		}) || pet == nil {
			t.Fatal("no beast placed")
		}
		w.srv.Settle(t)
		return w.liveNPCs(ind8CougarID)[0], w.srv.ReadQueued(t, w.srv.Client)
	}()
	if got := says(frames, nobody.ObjectID()); len(got) != 0 || len(w.heard.take()) != 0 || script.NPCOf(nobody).Slot(script.SlotCreature0) != nil {
		t.Fatalf("an ownerless beast said %q and claimed someone; want nothing", got)
	}
	w.srv.Advance(t, 16*time.Second)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard from an ownerless beast in 16 s: %q, want nothing", got)
	}
	w.deleteMe(t, nobody)

	first, frames := w.tame(t, master, ind8BuffaloID, 2188, 100)
	if got := says(frames, first.ObjectID()); !slices.Equal(got, []string{script.NpcString(2024, "Hunter")}) {
		t.Fatalf("beast lines on its spawn = %q, want the 2024 greeting naming Hunter", got)
	}
	if got := ind8Slot(first, script.SlotIntAI3); got != ind8GoldenSpice {
		t.Fatalf("spice of a skill 2188 beast = %d, want %d", got, ind8GoldenSpice)
	}
	if got := script.NPCOf(first).Slot(script.SlotCreature0); got == nil || got.ObjectID() != owner {
		t.Fatalf("beast owner = %v, want the character", got)
	}
	came := ind8Slot(first, script.SlotIntAI1)
	if got, want := w.heard.take(), []string{fmt.Sprintf("event=10022 arg1=%d arg2=%d", owner, came)}; !slices.Equal(got, want) {
		t.Fatalf("heard on the beast's spawn = %q, want %q", got, want)
	}
	w.srv.Advance(t, 5*time.Second)
	if got, want := w.heard.take(), []string{fmt.Sprintf("event=10017 arg1=%d arg2=0", first.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("heard 5 s on = %q, want %q", got, want)
	}
	w.srv.Advance(t, 10*time.Second)
	if got, want := w.heard.take(), fmt.Sprintf("event=10022 arg1=%d arg2=%d", owner, came); !slices.Contains(got, want) {
		t.Fatalf("heard 15 s on = %q, want %q among them", got, want)
	}

	w.draws.set(99)
	second, frames := w.tame(t, master, ind8CougarID, 0, -100)
	if got := says(frames, second.ObjectID()); len(got) != 0 {
		t.Fatalf("beast lines on a 99 draw = %q, want none", got)
	}
	if got := ind8Slot(second, script.SlotIntAI3); got != ind8CrystalSpice {
		t.Fatalf("spice of another beast = %d, want %d", got, ind8CrystalSpice)
	}
	got := ind8Slot(second, script.SlotIntAI1)
	if got < came || first.Decayed() != (got > came) {
		t.Fatalf("a newer beast came at tick %d after %d: older decayed %v; want it gone only for a later tick", got, came, first.Decayed())
	}
	w.send(t, second, 10022, owner+1, got+1)
	w.send(t, second, 10022, owner, got)
	if second.Decayed() {
		t.Fatal("the beast left on a claim of another player, or of its own tick; want it to stay")
	}
	w.send(t, second, 10022, owner, got+1)
	if !second.Decayed() {
		t.Fatal("the beast stayed on its owner's claim of a later tick; want it gone")
	}
}

// TestWarriorPetForPCGivesTheJewelOfInnocence runs the Bring Up With Love
// branch of WarriorPetForPC's created hook: a beast of an owner on that
// quest gives him the Jewel of Innocence on its 5 in 100 draw, when he
// holds none, and moves the quest to its second step.
func TestWarriorPetForPCGivesTheJewelOfInnocence(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, nil)
	owner := w.player.ObjectID()
	w.draws.set(99)
	w.tame(t, master, ind8BuffaloID, 0, 100)
	if got := w.srv.PlayerItemCount(t, owner, ind8Jewel); got != 0 {
		t.Fatalf("jewels off the quest = %d, want 0", got)
	}

	w.startQuest(t, ind8Q020, 1)
	w.draws.set(99, 99, 5)
	w.tame(t, master, ind8BuffaloID, 0, 100)
	if got := w.srv.PlayerItemCount(t, owner, ind8Jewel); got != 0 || w.questCond(t, ind8Q020) != 1 {
		t.Fatalf("on a draw of 5: %d jewels, cond %d; want none, 1", got, w.questCond(t, ind8Q020))
	}
	w.draws.set(99, 99, 4)
	w.tame(t, master, ind8BuffaloID, 0, 100)
	if got := w.srv.PlayerItemCount(t, owner, ind8Jewel); got != 1 || w.questCond(t, ind8Q020) != 2 {
		t.Fatalf("on a draw of 4: %d jewels, cond %d; want 1, 2", got, w.questCond(t, ind8Q020))
	}
	w.draws.set(99, 99, 0)
	w.tame(t, master, ind8BuffaloID, 0, 100)
	if got := w.srv.PlayerItemCount(t, owner, ind8Jewel); got != 1 {
		t.Fatalf("jewels when he holds one = %d, want still 1", got)
	}
}

// TestWarriorPetForPCGivesTheCrystalOfPurity runs the A Grand Plan for
// Taming Wild Beasts branch of WarriorPetForPC's created hook: a beast of
// a clan leader on the quest's first step gives him a Crystal of Purity,
// the tenth moving the quest on; past that step it gives none.
func TestWarriorPetForPCGivesTheCrystalOfPurity(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, func(srv *gameservertest.Server, objID int32) {
		srv.GiveItem(t, objID, ind8Crystal, 8)
	}, gameservertest.WithClanSeed(func(db *sql.DB) {
		for _, q := range []string{
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) SELECT 7001, 'Tamers', 1, 0, obj_Id FROM characters WHERE char_name = 'Hunter'`,
			`UPDATE characters SET clanid = 7001 WHERE char_name = 'Hunter'`,
		} {
			if _, err := db.Exec(q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	}))
	owner := w.player.ObjectID()
	w.draws.set(99)
	w.startQuest(t, ind8Q655, 1)
	w.tame(t, master, ind8BuffaloID, 0, 100)
	if got := w.srv.PlayerItemCount(t, owner, ind8Crystal); got != 9 || w.questCond(t, ind8Q655) != 1 {
		t.Fatalf("after one beast: %d crystals, cond %d; want 9, 1", got, w.questCond(t, ind8Q655))
	}
	w.tame(t, master, ind8BuffaloID, 0, 100)
	if got := w.srv.PlayerItemCount(t, owner, ind8Crystal); got != 10 || w.questCond(t, ind8Q655) != 2 {
		t.Fatalf("after the tenth crystal: %d crystals, cond %d; want 10, 2", got, w.questCond(t, ind8Q655))
	}
	w.tame(t, master, ind8BuffaloID, 0, 100)
	if got := w.srv.PlayerItemCount(t, owner, ind8Crystal); got != 10 {
		t.Fatalf("crystals past the first step = %d, want still 10", got)
	}
}

// TestWarriorPetForPCBuffsFeedsAndFollows runs WarriorPetForPC's timer and
// no-desire hooks. Idle, a beast runs to 150 short of its owner (to 0, 0
// when it stands on him). Every 5 s, when he lacks three of its five
// buffs, it wants to cast the one its draw of 100 picks at him, weight
// 1000000, if he lacks it. Every minute it eats ConsumeFeedNum (1) of his
// spice, plays its TakeSocial social action and says one of ten lines;
// with too little spice it leaves. One more than 2000 from him leaves on
// its next 5 s.
func TestWarriorPetForPCBuffsFeedsAndFollows(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, func(srv *gameservertest.Server, objID int32) {
		srv.GiveItem(t, objID, ind8GoldenSpice, 2)
	})
	owner := w.player.ObjectID()
	w.draws.set(99)
	pet, _ := w.tame(t, master, ind8BuffaloID, 2188, 100)

	w.ticks(t, 2)
	move := ind8Desires(pet, actorai.IntentionMoveTo)
	if want := (location.Location{X: w.at.X + 150, Y: w.at.Y, Z: w.at.Z}); len(move) != 1 || move[0].Location != want || move[0].Weight != 10000 {
		t.Fatalf("idle move desires = %+v, want one to %+v with weight 10000", move, want)
	}

	w.draws.set(0)
	w.srv.Advance(t, 5*time.Second)
	casts := ind8Desires(pet, actorai.IntentionCast)
	if len(casts) != 1 || casts[0].Skill != (modelskill.Ref{ID: ind8PetBuff1, Level: 1}) || casts[0].FinalTarget.ObjectID() != owner || casts[0].Weight != 1000000 {
		t.Fatalf("cast desires 5 s on = %+v, want the first buff at the owner with weight 1000000", casts)
	}

	w.srv.Advance(t, 55*time.Second)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := w.srv.PlayerItemCount(t, owner, ind8GoldenSpice); got != 1 {
		t.Fatalf("spice after a minute = %d, want 1", got)
	}
	if got := says(frames, pet.ObjectID()); !slices.Equal(got, []string{script.NpcString(2029)}) {
		t.Fatalf("beast lines as it ate = %q, want the 2029 line", got)
	}
	social := ind8Desires(pet, actorai.IntentionSocial)
	if len(social) != 1 || social[0].ItemObjectID != 1 || social[0].Timer != ind8PetTakeSocial*1000/30 || social[0].Weight != 200 {
		t.Fatalf("social desires as it ate = %+v, want action 1 for %d ms with weight 200", social, ind8PetTakeSocial*1000/30)
	}
	w.srv.Advance(t, time.Minute)
	if got := w.srv.PlayerItemCount(t, owner, ind8GoldenSpice); got != 0 || pet.Decayed() {
		t.Fatalf("after two minutes: spice %d, beast gone %v; want 0, there", got, pet.Decayed())
	}
	w.srv.Advance(t, time.Minute)
	if !pet.Decayed() {
		t.Fatal("the beast stayed with no spice to eat; want it gone")
	}

	onHim, _ := w.tame(t, master, ind8CougarID, 0, 0)
	w.ticks(t, 2)
	move = ind8Desires(onHim, actorai.IntentionMoveTo)
	if want := (location.Location{X: 0, Y: 0, Z: w.at.Z}); len(move) != 1 || move[0].Location != want {
		t.Fatalf("idle move desires of a beast on its owner = %+v, want one to %+v", move, want)
	}
	far, _ := w.tame(t, master, ind8CougarID, 0, 2100)
	w.srv.Advance(t, 4*time.Second)
	if far.Decayed() {
		t.Fatal("the far beast left before its 5 s")
	}
	w.srv.Advance(t, time.Second)
	if !far.Decayed() {
		t.Fatal("a beast 2100 from its owner stayed past its 5 s; want it gone")
	}
}

// TestWarriorPetForPCGuardsItsClan runs WarriorPetForPC's clan-attacked
// hook. A beast of the clan of a member hit at 80 % HP or more, while its
// owner holds neither of its debuffs, wants each debuff, on its draw of 33
// below, at that member, weight 1000000; below a quarter of his HP it
// wants to heal its owner on a draw below 40, below half on one below 20.
// A hit on the beast itself does nothing of that.
func TestWarriorPetForPCGuardsItsClan(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, nil)
	owner := w.player.ObjectID()
	w.draws.set(99)
	pet, _ := w.tame(t, master, ind8BuffaloID, 0, 100)
	mate := w.spawn(t, ind8HerdMateID, 200, 0)

	w.hit(t, pet, 10)
	if got := ind8Desires(pet, actorai.IntentionCast); len(got) != 0 {
		t.Fatalf("cast desires after a hit on the beast = %+v, want none", got)
	}
	w.draws.set(0, 32, 33)
	w.hit(t, mate, 10)
	casts := ind8Desires(pet, actorai.IntentionCast)
	if len(casts) != 1 || casts[0].Skill.ID != ind8PetDebuff1 || casts[0].FinalTarget.ObjectID() != mate.ObjectID() || casts[0].Weight != 1000000 {
		t.Fatalf("cast desires after the mate's hit = %+v, want the first debuff at the mate", casts)
	}

	hp := w.srv.PlayerMaxHP(t, owner)
	w.srv.DamagePlayerHP(t, owner, hp/2+1)
	w.draws.set(99, 99, 99, 19)
	w.hit(t, mate, 10)
	casts = ind8Desires(pet, actorai.IntentionCast)
	if len(casts) != 2 || casts[1].Skill.ID != ind8PetHeal || casts[1].FinalTarget.ObjectID() != owner {
		t.Fatalf("cast desires with the owner below half = %+v, want the heal at the owner added", casts)
	}
}

// hold lands the effect of the pet skill id on the character, cast by
// himself, and checks he holds it.
func (w *ind8World) hold(t *testing.T, id modelskill.ID) {
	t.Helper()
	ref := modelskill.Ref{ID: id, Level: 1}
	var level int32
	if !w.srv.RunScript(t, w.player.ObjectID(), ind8DirectorName, func(s *script.Script, p *script.Player) {
		s.ApplyEffects(ref, p, p)
		level = s.AbnormalLevel(p, ref)
	}) || level <= 0 {
		t.Fatalf("the character holds no effect of skill %d (level %d)", id, level)
	}
}

// ind8Casts renders h's queued cast desires as "skill>target" pairs.
func ind8Casts(h *npc.Hostile) []string {
	var out []string
	for _, d := range ind8Desires(h, actorai.IntentionCast) {
		out = append(out, fmt.Sprintf("%d>%d", d.Skill.ID, d.FinalTarget.ObjectID()))
	}
	slices.Sort(out)
	return out
}

// TestWarriorPetForPCBuffsOnlyALackingOwner runs the buff gates of
// WarriorPetForPC's 5 s timer. With two of its five buffs on its owner it
// still lacks three, so every 5 s it draws which buff to give, and gives
// it only if he lacks that one: the draws of 0 and 20 pick buffs he holds,
// 40 the third, which it wants to cast. With three on him it draws
// nothing and gives nothing.
func TestWarriorPetForPCBuffsOnlyALackingOwner(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, nil)
	owner := w.player.ObjectID()
	w.hold(t, ind8PetBuff1)
	w.hold(t, ind8PetBuff1+1)
	w.draws.set(99, 99, 0, 20, 40)
	pet, _ := w.tame(t, master, ind8BuffaloID, 0, 100)

	for i, want := range [][]string{nil, nil, {fmt.Sprintf("%d>%d", ind8PetBuff1+2, owner)}} {
		w.srv.Advance(t, 5*time.Second)
		if got := ind8Casts(pet); !slices.Equal(got, want) {
			t.Fatalf("cast desires at %d s = %q, want %q", 5*(i+1), got, want)
		}
	}
	w.hold(t, ind8PetBuff1+2)
	w.draws.set(99, 80)
	w.srv.Advance(t, 5*time.Second)
	if got := ind8Casts(pet); !slices.Equal(got, []string{fmt.Sprintf("%d>%d", ind8PetBuff1+2, owner)}) {
		t.Fatalf("cast desires with three buffs held = %q, want no new one", got)
	}
}

// TestWarriorPetForPCGuardsItsClanWithinItsGates runs the gates of
// WarriorPetForPC's clan-attacked hook. A member hit at 80 % HP or more
// draws for each debuff; one hit below 80 % (the HP the hit finds) draws
// for none. An owner below a
// quarter of his HP is healed on a draw below 40, not on 40.
func TestWarriorPetForPCGuardsItsClanWithinItsGates(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, nil)
	owner := w.player.ObjectID()
	w.draws.set(99)
	pet, _ := w.tame(t, master, ind8BuffaloID, 0, 100)
	mate := w.spawn(t, ind8HerdMateID, 200, 0)
	debuff2 := fmt.Sprintf("%d>%d", ind8PetDebuff2, mate.ObjectID())

	w.draws.set(99, 99, 0)
	w.hit(t, mate, 10)
	if got := ind8Casts(pet); !slices.Equal(got, []string{debuff2}) {
		t.Fatalf("cast desires after a hit at full HP = %q, want the second debuff at the mate", got)
	}
	// The hook reads the HP the hit finds, so the 250 lands first.
	w.draws.set(99)
	w.hit(t, mate, 250)
	w.draws.set(99, 0, 0)
	w.hit(t, mate, 10)
	if got := ind8Casts(pet); !slices.Equal(got, []string{debuff2}) {
		t.Fatalf("cast desires after a hit below 80 %% = %q, want no new one", got)
	}

	hp := w.srv.PlayerMaxHP(t, owner)
	w.srv.DamagePlayerHP(t, owner, hp*3/4+1)
	w.draws.set(99, 40)
	w.hit(t, mate, 10)
	if got := ind8Casts(pet); !slices.Equal(got, []string{debuff2}) {
		t.Fatalf("cast desires on a heal draw of 40 = %q, want no new one", got)
	}
	w.draws.set(99, 39)
	w.hit(t, mate, 10)
	if got, want := ind8Casts(pet), []string{debuff2, fmt.Sprintf("%d>%d", ind8PetHeal, owner)}; !slices.Equal(got, want) {
		t.Fatalf("cast desires on a heal draw of 39 = %q, want %q", got, want)
	}
}

// TestWarriorPetForPCSparesTheClanWhileItsOwnerHoldsADebuff runs the owner
// gate of WarriorPetForPC's clan-attacked hook: while its owner holds one
// of its debuffs, a member hit at full HP gets neither debuff.
func TestWarriorPetForPCSparesTheClanWhileItsOwnerHoldsADebuff(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, nil)
	w.hold(t, ind8PetDebuff1)
	w.draws.set(99)
	pet, _ := w.tame(t, master, ind8BuffaloID, 0, 100)
	mate := w.spawn(t, ind8HerdMateID, 200, 0)
	w.draws.set(99, 0, 0)
	w.hit(t, mate, 10)
	if got := ind8Casts(pet); len(got) != 0 {
		t.Fatalf("cast desires while the owner holds a debuff = %q, want none", got)
	}
}

// TestWarriorPetForPCLeavesADeadOwner runs the owner gate of
// WarriorPetForPC's minute timer: a beast whose owner is dead leaves,
// however much spice he holds, and eats none of it.
func TestWarriorPetForPCLeavesADeadOwner(t *testing.T) {
	t.Parallel()
	w, master := bootTamer(t, func(srv *gameservertest.Server, objID int32) {
		srv.GiveItem(t, objID, ind8GoldenSpice, 5)
	})
	owner := w.player.ObjectID()
	w.draws.set(99)
	pet, _ := w.tame(t, master, ind8BuffaloID, 2188, 100)
	w.srv.MarkPlayerDead(t, owner)
	w.srv.Advance(t, time.Minute)
	if !pet.Decayed() || w.srv.PlayerItemCount(t, owner, ind8GoldenSpice) != 5 {
		t.Fatalf("a minute after its owner died: beast gone %v, spice %d; want gone, 5 left", pet.Decayed(), w.srv.PlayerItemCount(t, owner, ind8GoldenSpice))
	}
}
