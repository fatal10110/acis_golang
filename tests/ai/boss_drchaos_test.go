package ai

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/drchaos"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Doctor Chaos, the machine it works at and the golem it raises.
const (
	drChaosID        = int32(32033)
	drChaosMachineID = int32(32032)
	drChaosGolemID   = int32(25512)
	drChaosDirector  = "DrChaosDirector"
)

// Where Doctor Chaos and its machine stand as shipped, where the character
// enters the world beside them, where Doctor Chaos works, where the party
// lands and where the golem rises.
var (
	drChaosHome    = location.Location{X: 96524, Y: -111070, Z: -3360}
	drChaosMachine = location.Location{X: 96341, Y: -110891, Z: -3360}
	drChaosAt      = location.Location{X: 96480, Y: -111040, Z: -3360}
	drChaosWork    = location.Location{X: 96320, Y: -110912, Z: -3360}
	drChaosRoom    = location.Location{X: 94832, Y: -112624, Z: -3328}
	drChaosRaid    = location.Location{X: 94640, Y: -112496, Z: -3360}
)

// bootDrChaos boots Doctor Chaos's behavior with the character entering
// the world beside its shipped spot. The scenario drchaos_talk covers its
// first line and walk; these tests cover what the format cannot render:
// its walks, cutscene cameras and social action, the party teleport and
// the golem private it raises.
func bootDrChaos(t *testing.T) *warriorWorld {
	t.Helper()
	chaos := gameservertest.FolkTemplate("Folk", int(drChaosID))
	chaos.CanMove, chaos.CollisionRadius, chaos.BaseAttackRange = true, 9, 40
	machine := gameservertest.FolkTemplate("Folk", int(drChaosMachineID))
	machine.CollisionRadius = 16
	golem := warriorTemplate(drChaosGolemID, nil)
	golem.Type = "RaidBoss"
	kinds := map[int32]script.NPCKind{drChaosID: script.KindFolk, drChaosMachineID: script.KindFolk, drChaosGolemID: script.KindHostile}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.drchaos.DoctorChaosNpc"},
		{Path: "test." + drChaosDirector},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":       defaultnpc.New,
		"script.ai.boss.drchaos.DoctorChaosNpc": drchaos.DoctorChaosNpc,
		"test." + drChaosDirector:               func() script.Script { return script.Script{} },
	}
	class := gameservertest.ClassTemplate()
	class.Spawns = []location.Location{drChaosAt}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(class),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{chaos, machine, golem})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	return &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
}

// drChaosSpawn places the civilian of id at the absolute point at.
func drChaosSpawn(t *testing.T, w *warriorWorld, id int32, at location.Location) *npc.Folk {
	t.Helper()
	return w.spawnFolk(t, id, at.X-w.at.X, at.Y-w.at.Y)
}

// drChaosWalk runs position ticks until f's walk ends and returns the
// frames the character was sent.
func drChaosWalk(t *testing.T, w *warriorWorld, f *npc.Folk) [][]byte {
	t.Helper()
	if !f.IsMoving() {
		t.Fatal("Doctor Chaos is not walking")
	}
	var frames [][]byte
	for i := 0; f.IsMoving(); i++ {
		if i >= 2000 {
			t.Fatal("the walk never arrived")
		}
		w.srv.TickPositions()
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	w.srv.Settle(t)
	return append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
}

// drChaosSetOff runs AI ticks 100 ms apart until f takes up its walk, at
// most ten, and returns the frames the character was sent: a swing in
// flight holds the next desire back until its hit animation ends.
func drChaosSetOff(t *testing.T, w *warriorWorld, f *npc.Folk) [][]byte {
	t.Helper()
	frames := cycle(t, w.srv)
	for i := 0; i < 10 && !f.IsMoving(); i++ {
		frames = append(frames, drChaosAdvance(t, w, 100*time.Millisecond)...)
		frames = append(frames, cycle(t, w.srv)...)
	}
	return frames
}

// drChaosAdvance lets d pass and returns the frames the character was sent.
func drChaosAdvance(t *testing.T, w *warriorWorld, d time.Duration) [][]byte {
	t.Helper()
	w.srv.Advance(t, d)
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// drChaosSays counts the lines of f in frames.
func drChaosSays(frames [][]byte, f *npc.Folk) int {
	return len(framesOf(frames, serverpackets.OpcodeNpcSay, f.ObjectID()))
}

// drChaosGolems returns the live golems.
func drChaosGolems(w *warriorWorld) []*npc.Hostile {
	var out []*npc.Hostile
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && int32(h.Instance.Template.ID) == drChaosGolemID && !h.Decayed() {
			out = append(out, h)
		}
	}
	return out
}

// drChaosGone reports whether f has left the world.
func drChaosGone(w *warriorWorld, f *npc.Folk) bool {
	_, ok := w.srv.State.Object(f.ObjectID())
	return !ok || f.Decayed()
}

// drChaosTalk selects and talks to Doctor Chaos and returns the frames of
// the talk.
func drChaosTalk(t *testing.T, w *warriorWorld, f *npc.Folk) [][]byte {
	t.Helper()
	w.srv.Client.Send(encodeFolkAction(f.ObjectID(), w.at))
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Client.Send(encodeFolkAction(f.ObjectID(), w.at))
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// drChaosToMachine runs the talk and the walk to the machine: the first
// line, and 2 s later a walk to the machine. With the machine in sight, the
// arrival's attack on it takes the still queued walk up again on the
// machine's spot: one zero-length MoveToLocation, arriving (and opening the
// move-finished hook again) one move tick later, so the walk ends that long
// after the arrival. It returns how long after the arrival the walk ended.
func drChaosToMachine(t *testing.T, w *warriorWorld, chaos *npc.Folk, machineInSight bool) time.Duration {
	t.Helper()
	if got := drChaosSays(drChaosTalk(t, w, chaos), chaos); got != 1 {
		t.Fatalf("lines at the talk = %d, want 1", got)
	}
	frames := drChaosAdvance(t, w, 1999*time.Millisecond)
	frames = append(frames, cycle(t, w.srv)...)
	if chaos.IsMoving() {
		t.Fatal("Doctor Chaos walks before 2 s passed")
	}
	drChaosAdvance(t, w, time.Millisecond)
	drChaosSetOff(t, w, chaos)
	walk := drChaosWalk(t, w, chaos)
	if x, y, _ := chaos.Position(); x != drChaosWork.X || y != drChaosWork.Y {
		t.Fatalf("Doctor Chaos stopped at %d,%d, want its machine's %d,%d", x, y, drChaosWork.X, drChaosWork.Y)
	}
	if got := drChaosSays(frames, chaos); got != 0 {
		t.Fatalf("lines before the walk = %d, want none", got)
	}
	inPlace := 0
	for _, f := range framesOf(walk, serverpackets.OpcodeMoveToLocation, chaos.ObjectID()) {
		if dest := moveDestination(f); dest.X == drChaosWork.X && dest.Y == drChaosWork.Y && dest == moveOrigin(f) {
			inPlace++
		}
	}
	if !machineInSight {
		if inPlace != 0 {
			t.Fatalf("zero-length moves at the machine's spot = %d, want none with no machine to strike", inPlace)
		}
		return 0
	}
	if inPlace != 1 {
		t.Fatalf("zero-length moves at the machine's spot = %d, want the walk taken up again once", inPlace)
	}
	return move.PositionUpdateInterval
}

// drChaosCutscene runs the cutscene from Doctor Chaos's arrival at the
// machine to its arrival at the door, the machine camera centred on
// filmed, and returns the door's frames. The walk to the machine ended
// pastArrival after its arrival.
func drChaosCutscene(t *testing.T, w *warriorWorld, chaos *npc.Folk, filmed int32, pastArrival time.Duration) [][]byte {
	t.Helper()
	if got := drChaosSays(drChaosAdvance(t, w, 3*time.Second-pastArrival), chaos); got != 1 {
		t.Fatalf("lines 3 s after the arrival = %d, want the cutscene's 1", got)
	}
	frames := drChaosAdvance(t, w, 2*time.Second)
	want := ind8CameraPayload(filmed, 1, -200, 15, 10000, 20000, 0, 0, 0, 0)
	if got := ind8Pick(frames, serverpackets.OpcodeSpecialCamera); len(got) != 1 || string(got[0]) != string(want) {
		t.Fatalf("cameras 2 s into the cutscene = %x, want one on the machine %x", got, want)
	}
	frames = append(frames, cycle(t, w.srv)...)
	rest := drChaosAdvance(t, w, 8999*time.Millisecond)
	if got := ind8Pick(rest, serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("cameras before 11 s into the cutscene = %x, want none", got)
	}
	social := framesOf(append(frames, rest...), serverpackets.OpcodeSocialAction, chaos.ObjectID())
	if len(social) != 1 || binary.LittleEndian.Uint32(social[0][5:9]) != 3 {
		t.Fatalf("social actions after the machine's camera = %x, want one of action 3", social)
	}
	frames = drChaosAdvance(t, w, time.Millisecond)
	want = ind8CameraPayload(chaos.ObjectID(), 1, -150, 10, 3000, 20000, 0, 0, 0, 0)
	if got := ind8Pick(frames, serverpackets.OpcodeSpecialCamera); len(got) != 1 || string(got[0]) != string(want) {
		t.Fatalf("cameras 11 s into the cutscene = %x, want one on Doctor Chaos %x", got, want)
	}
	drChaosSetOff(t, w, chaos)
	return drChaosWalk(t, w, chaos)
}

// drChaosAssertRaised checks that 3 s after the door Doctor Chaos raises
// one golem at the golem's spot, leading it, with the talker's object id as
// its second parameter, and that the golem's death takes Doctor Chaos out
// of the world.
func drChaosAssertRaised(t *testing.T, w *warriorWorld, chaos *npc.Folk) {
	t.Helper()
	drChaosAdvance(t, w, 2999*time.Millisecond)
	if got := drChaosGolems(w); len(got) != 0 {
		t.Fatalf("golems before 3 s passed = %d, want none", len(got))
	}
	drChaosAdvance(t, w, time.Millisecond)
	golems := drChaosGolems(w)
	if len(golems) != 1 {
		t.Fatalf("golems 3 s after the door = %d, want 1", len(golems))
	}
	golem := golems[0]
	if x, y, _ := golem.Position(); x != drChaosRaid.X || y != drChaosRaid.Y {
		t.Fatalf("golem at %d,%d, want %d,%d", x, y, drChaosRaid.X, drChaosRaid.Y)
	}
	s := golem.Scratch()
	if a, b, c := s.Int(npc.IntParam1), s.Int(npc.IntParam2), s.Int(npc.IntParam3); a != 1000 || b != w.player.ObjectID() || c != 0 {
		t.Fatalf("golem params = %d, %d, %d; want 1000, %d, 0", a, b, c, w.player.ObjectID())
	}
	if minions := chaos.Minions(); len(minions) != 1 || minions[0].ObjectID() != golem.ObjectID() {
		t.Fatalf("Doctor Chaos leads %d privates, want the golem", len(minions))
	}

	w.kill(t, golem)
	if !drChaosGone(w, chaos) {
		t.Fatal("Doctor Chaos stays after its golem died")
	}
}

// TestDrChaosLeadsThePlayerToTheGolem runs Doctor Chaos's whole cutscene
// for a character in no party. Talked to, it speaks and 2 s later walks to
// its machine. 3 s after it arrives, the talker standing within 600, it
// speaks again, 2 s later films the machine and does social action 3, and
// 11 s after the line films itself and walks to the door. There it speaks
// a last line, sends the character to the golem's room and leaves for
// -113091,-243942,-15536; 3 s later it raises the golem, whose death takes
// it out of the world.
func TestDrChaosLeadsThePlayerToTheGolem(t *testing.T) {
	t.Parallel()
	w := bootDrChaos(t)
	machine := drChaosSpawn(t, w, drChaosMachineID, drChaosMachine)
	chaos := drChaosSpawn(t, w, drChaosID, drChaosHome)

	pastArrival := drChaosToMachine(t, w, chaos, true)
	frames := drChaosCutscene(t, w, chaos, machine.ObjectID(), pastArrival)
	if got := drChaosSays(frames, chaos); got != 1 {
		t.Fatalf("lines at the door = %d, want 1", got)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	if x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); x != drChaosRoom.X || y != drChaosRoom.Y {
		t.Fatalf("character at %d,%d after the door, want the golem's room %d,%d", x, y, drChaosRoom.X, drChaosRoom.Y)
	}
	if x, y, z := chaos.Position(); x != -113091 || y != -243942 || z != -15536 {
		t.Fatalf("Doctor Chaos at %d,%d,%d after the door, want -113091,-243942,-15536", x, y, z)
	}
	drChaosAssertRaised(t, w, chaos)
}

// TestDrChaosSendsTheWholeParty covers the door's party branch: every
// member of the talker's party lands in the golem's room. With no machine
// in sight, Doctor Chaos strikes nothing at its machine's spot and the
// machine's camera films Doctor Chaos itself.
func TestDrChaosSendsTheWholeParty(t *testing.T) {
	t.Parallel()
	w := bootDrChaos(t)
	w.joinMate(t)
	chaos := drChaosSpawn(t, w, drChaosID, drChaosHome)

	pastArrival := drChaosToMachine(t, w, chaos, false)
	drChaosCutscene(t, w, chaos, chaos.ObjectID(), pastArrival)
	w.srv.Settle(t)
	players := 0
	for _, obj := range w.srv.State.Objects() {
		if _, ok := w.srv.State.Player(obj.ObjectID()); !ok {
			continue
		}
		players++
		if x, y, _ := w.srv.PlayerPosition(t, obj.ObjectID()); x != drChaosRoom.X || y != drChaosRoom.Y {
			t.Fatalf("party member %d at %d,%d after the door, want the golem's room %d,%d", obj.ObjectID(), x, y, drChaosRoom.X, drChaosRoom.Y)
		}
	}
	if players != 2 {
		t.Fatalf("players in the world = %d, want the character and Mate", players)
	}
}

// TestDrChaosGoesHomeWithoutTheTalker covers the cutscene's refusal: the
// talker standing 600 or more from Doctor Chaos 3 s after it reached its
// machine, it walks home silently and forgets the talker, so no camera
// and no door follow.
func TestDrChaosGoesHomeWithoutTheTalker(t *testing.T) {
	t.Parallel()
	w := bootDrChaos(t)
	drChaosSpawn(t, w, drChaosMachineID, drChaosMachine)
	chaos := drChaosSpawn(t, w, drChaosID, drChaosHome)

	drChaosToMachine(t, w, chaos, true)
	w.instantTeleport(t, location.Location{X: drChaosWork.X + 600, Y: drChaosWork.Y, Z: drChaosWork.Z})
	frames := drChaosAdvance(t, w, 3*time.Second)
	frames = append(frames, drChaosSetOff(t, w, chaos)...)
	frames = append(frames, drChaosWalk(t, w, chaos)...)
	if x, y, _ := chaos.Position(); x != drChaosHome.X || y != drChaosHome.Y {
		t.Fatalf("Doctor Chaos stopped at %d,%d, want home %d,%d", x, y, drChaosHome.X, drChaosHome.Y)
	}
	frames = append(frames, drChaosAdvance(t, w, 20*time.Second)...)
	frames = append(frames, cycle(t, w.srv)...)
	if got := drChaosSays(frames, chaos); got != 0 {
		t.Fatalf("lines without the talker = %d, want none", got)
	}
	if got := ind8Pick(frames, serverpackets.OpcodeSpecialCamera); len(got) != 0 {
		t.Fatalf("cameras without the talker = %x, want none", got)
	}
	if chaos.IsMoving() || len(drChaosGolems(w)) != 0 {
		t.Fatal("Doctor Chaos went on without the talker")
	}
}

// TestDrChaosRaisesTheGolemInItsRoom covers the created hook at z -15536:
// Doctor Chaos raises the golem at once, with no parameters, and script
// event 10029 takes it out of the world; another event leaves it.
func TestDrChaosRaisesTheGolemInItsRoom(t *testing.T) {
	t.Parallel()
	w := bootDrChaos(t)
	// The fixture world has no geodata: a spawn stands 20 above the asked z.
	n, err := w.srv.NpcSpawns.AddSpawn(drChaosID, -113091, -243942, -15556, 0, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	chaos := n.(*npc.Folk)
	golems := drChaosGolems(w)
	if len(golems) != 1 {
		t.Fatalf("golems as Doctor Chaos appears in its room = %d, want 1", len(golems))
	}
	if x, y, _ := golems[0].Position(); x != drChaosRaid.X || y != drChaosRaid.Y {
		t.Fatalf("golem at %d,%d, want %d,%d", x, y, drChaosRaid.X, drChaosRaid.Y)
	}
	s := golems[0].Scratch()
	if a, b, c := s.Int(npc.IntParam1), s.Int(npc.IntParam2), s.Int(npc.IntParam3); a != 0 || b != 0 || c != 0 {
		t.Fatalf("golem params = %d, %d, %d; want none", a, b, c)
	}
	event := func(id int32) {
		if !w.srv.RunScript(t, w.player.ObjectID(), drChaosDirector, func(s *script.Script, _ *script.Player) {
			s.SendScriptEvent(script.NPCOf(chaos), id, 0, 0)
		}) {
			t.Fatal("the script event panicked")
		}
		w.srv.Settle(t)
	}
	event(10028)
	if drChaosGone(w, chaos) {
		t.Fatal("Doctor Chaos left on script event 10028")
	}
	event(10029)
	if !drChaosGone(w, chaos) {
		t.Fatal("Doctor Chaos stays after script event 10029")
	}
}
