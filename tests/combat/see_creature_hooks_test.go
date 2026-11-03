package combat

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The template ids of the see-creature scenarios: a monster and a civilian
// the test behavior is bound to, and a monster it is not.
const (
	seeMonster   = int32(20060)
	seeFolk      = int32(30110)
	seeBystander = int32(20061)
)

// seeLog records the see-creature hooks of a test behavior, one line per
// call. Hooks run on each NPC's own queue, or on the caller of a look
// around, so every field is guarded by mu.
type seeLog struct {
	mu sync.Mutex
	// names maps an object id to the name the lines use.
	names map[int32]string
	// hostiles are the bound monsters by object id, for the golden lines'
	// lifetime and intention.
	hostiles map[int32]*npc.Hostile
	// attack makes the hook queue an attack of weight 200 on what it sees,
	// as the aggressive behaviors' tryToAttack does.
	attack bool
	lines  []string
}

func newSeeLog() *seeLog {
	return &seeLog{names: map[int32]string{}, hostiles: map[int32]*npc.Hostile{}}
}

func (l *seeLog) name(id int32, name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names[id] = name
}

func (l *seeLog) trackHostile(name string, h *npc.Hostile) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names[h.ObjectID()] = name
	l.hostiles[h.ObjectID()] = h
}

func (l *seeLog) setAttack(on bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attack = on
}

// take returns the lines since the last call, sorted: NPCs on different
// queues record in no fixed order.
func (l *seeLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines := l.lines
	l.lines = nil
	slices.Sort(lines)
	return lines
}

// see is the hook. A line reads "SEE <npc> <creature>"; for a tracked
// monster it adds the golden's "see <creature> lifetime=<n>
// intention=<intention>" and, with attack on, the attack it queues.
func (l *seeLog) see(_ *script.Script, e script.SeeCreature) {
	l.mu.Lock()
	npcName, creature := l.names[e.NPC.ObjectID()], l.names[e.Creature.ObjectID()]
	l.lines = append(l.lines, fmt.Sprintf("SEE %s %s", npcName, creature))
	h, attack := l.hostiles[e.NPC.ObjectID()], l.attack
	if h != nil {
		l.lines = append(l.lines, fmt.Sprintf("%s: see %s lifetime=%d intention=%s", npcName, creature, h.LifeTime(), intentionName(h)))
	}
	l.mu.Unlock()
	if h == nil || !attack {
		return
	}
	e.NPC.AddAttackDesire(e.Creature, 200)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf("%s: add attack desire on %s -> intention=%s", npcName, creature, intentionName(h)))
}

func intentionName(h *npc.Hostile) string {
	return strings.ToUpper(h.AI().CurrentIntention().String())
}

// bootSeeHooks boots one character in the world, with the AI task, and
// the recording behavior bound to the monster and the civilian ids.
func bootSeeHooks(t *testing.T, log *seeLog, opts ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	kinds := map[int32]script.NPCKind{seeMonster: script.KindHostile, seeBystander: script.KindHostile, seeFolk: script.KindFolk}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.SeeRecorder"}}, script.Catalog{
			"ai.SeeRecorder": func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{seeMonster, seeFolk}, Hooks: script.Hooks{OnSeeCreature: log.see}}
			},
		}),
	}, opts...)...)
	startInWorld(t, srv.Client)
	log.name(srv.SoleObjectID(t), "p")
	return srv
}

// spawnSeer spawns a monster of template id at at, named name, on the AI
// task.
func spawnSeer(t *testing.T, srv *gameservertest.Server, log *seeLog, name string, id int32, at location.Location) *npc.Hostile {
	t.Helper()
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = int(id), int(id)
	h := srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	if id == seeMonster {
		log.trackHostile(name, h)
	} else {
		log.name(h.ObjectID(), name)
	}
	srv.AI.Add(h)
	return h
}

func wantSeeLines(t *testing.T, log *seeLog, what string, want ...string) {
	t.Helper()
	var got []string
	for _, l := range log.take() {
		if strings.HasPrefix(l, "SEE ") {
			got = append(got, l)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("see-creature calls %s =\n  %s\nwant\n  %s", what, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestSeeCreatureTickScanGates pins the see-creature scan that opens each
// AI tick of a bound monster and a bound civilian: each calls its behavior
// once for the player in its 450 see range, and again only after the
// player left the range and came back; one further away makes no call.
// While the player is spawn-protected, flying or invisible no NPC sees it,
// and one that has not yet seen it sees it once the state ends. An NPC
// whose id has no behavior scans nothing. Both executors run it: the
// hooks run on each NPC's own queue.
func TestSeeCreatureTickScanGates(t *testing.T) {
	t.Parallel()
	for _, exec := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(exec.name, func(t *testing.T) {
			t.Parallel()
			tickScanGates(t, exec.opts...)
		})
	}
}

func tickScanGates(t *testing.T, opts ...gameservertest.Option) {
	log := newSeeLog()
	srv := bootSeeHooks(t, log, opts...)
	objID := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, objID)
	at := func(dx, dy int) location.Location { return location.Location{X: x + dx, Y: y + dy, Z: z} }

	spawnSeer(t, srv, log, "near", seeMonster, at(200, 0))
	spawnSeer(t, srv, log, "far", seeMonster, at(0, 460))
	spawnSeer(t, srv, log, "bystander", seeBystander, at(-100, 0))
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", int(seeFolk)), at(-200, 0))
	log.name(folk.ObjectID(), "folk")
	drainUntilQuiet(t, srv.Client)

	runAICycle(t, srv)
	wantSeeLines(t, log, "on the first tick", "SEE folk p", "SEE near p")
	runAICycle(t, srv)
	wantSeeLines(t, log, "on the second tick")

	// Out of range, then back: told again.
	player, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from the world")
	}
	moveTo := func(l location.Location) {
		t.Helper()
		if err := srv.State.Move(player, l.X, l.Y, l.Z); err != nil {
			t.Fatalf("move player: %v", err)
		}
	}
	moveTo(at(0, -700))
	runAICycle(t, srv)
	wantSeeLines(t, log, "with the player out of range")
	moveTo(at(0, 0))
	runAICycle(t, srv)
	wantSeeLines(t, log, "with the player back in range", "SEE folk p", "SEE near p")

	character, ok := network.OnlineCharacter(player)
	if !ok {
		t.Fatalf("player %T is not an online character", player)
	}
	for _, gate := range []struct {
		name string
		set  func(on bool)
	}{
		{"spawn-protected", character.SetSpawnProtection},
		{"flying", func(on bool) { srv.SetPlayerFlying(t, objID, on) }},
		{"invisible", character.SetInvisible},
	} {
		gate.set(true)
		spawnSeer(t, srv, log, gate.name, seeMonster, at(250, 0))
		runAICycle(t, srv)
		wantSeeLines(t, log, "while the player is "+gate.name)
		gate.set(false)
		runAICycle(t, srv)
		wantSeeLines(t, log, "once the player is no longer "+gate.name, "SEE "+gate.name+" p")
	}
}

// TestSeeCreatureFirstAggroTick replays the first-aggro tick goldens: a
// monster whose behavior, on seeing the player, queues an attack of weight
// 200 attacks in the tick that sees it when its aggro list holds no one,
// and one tick later at lifetime 0 or, at a later lifetime, in the same
// tick, when it already holds someone. With the attack off, the monster
// tells about the player once while it stays in range.
func TestSeeCreatureFirstAggroTick(t *testing.T) {
	t.Parallel()
	log := newSeeLog()
	srv := bootSeeHooks(t, log)
	objID := srv.SoleObjectID(t)
	player, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from the world")
	}
	p := liveCombatant(t, srv)
	for i, table := range []string{"aggro.first_tick", "aggro.see_once"} {
		for j, r := range scriptcontract.Lookup(t, table).Rows {
			t.Run(table+"/"+r.ID, func(t *testing.T) {
				px, py, pz := srv.PlayerPosition(t, objID)
				// Each row's monster stands on its own spot, off the
				// previous ones' sight.
				home := location.Location{X: px + 3000*(2*i+j+1), Y: py, Z: pz}
				log.setAttack(r.Bool(t, "attack"))
				replayFirstAggroRow(t, srv, log, r, player, p, home)
			})
		}
	}
}

func replayFirstAggroRow(t *testing.T, srv *gameservertest.Server, log *seeLog, r scriptcontract.Row, player world.Player, p attackable.Combatant, home location.Location) {
	t.Helper()
	place := func(dist int) {
		t.Helper()
		if err := srv.State.Move(player, home.X, home.Y-dist, home.Z); err != nil {
			t.Fatalf("move player: %v", err)
		}
	}
	if r.Str(t, "start") == "in" {
		place(100)
	} else {
		place(1000)
	}
	h := spawnSeer(t, srv, log, "npc", seeMonster, home)
	var bystander *npc.Hostile
	if r.Bool(t, "aggro_list_held") {
		bystander = spawnSeer(t, srv, log, "p2", seeBystander, location.Location{X: home.X + 300, Y: home.Y, Z: home.Z})
		srv.AI.Remove(bystander)
	}
	t.Cleanup(func() {
		h.DeleteNow()
		if bystander != nil {
			bystander.DeleteNow()
		}
		srv.Settle(t)
	})
	drainUntilQuiet(t, srv.Client)
	for range r.Int(t, "age") {
		runAICycle(t, srv)
	}
	if bystander != nil {
		h.AI().AddDamageHate(bystander, 0, 1)
	}
	log.take()

	var got []string
	for _, line := range r.Lines {
		switch {
		case strings.HasPrefix(line, " "):
			// An output line of the tick above.
		case strings.HasPrefix(line, "setup "):
			px, py, pz := p.Position()
			hx, hy, hz := h.Position()
			where := "out"
			if dx, dy, dz := int64(px-hx), int64(py-hy), int64(pz-hz); dx*dx+dy*dy+dz*dz < npc.DefaultSeeRange*npc.DefaultSeeRange {
				where = "in"
			}
			s := fmt.Sprintf("setup lifetime=%d intention=%s p=%s", h.LifeTime(), intentionName(h), where)
			if bystander != nil {
				s += " aggro-list holds p2"
			}
			got = append(got, s)
		case line == "p moves in range":
			place(100)
			got = append(got, line)
		case line == "p moves out of range":
			place(1000)
			got = append(got, line)
		case strings.HasPrefix(line, "attack desire on p added outside a tick"):
			h.AI().AddAttackDesire(p, 200)
			srv.Settle(t)
			got = append(got, "attack desire on p added outside a tick -> intention="+intentionName(h))
		case strings.HasPrefix(line, "tick "):
			runAICycle(t, srv)
			got = append(got, line)
			for _, l := range log.takeOrdered() {
				if rest, ok := strings.CutPrefix(l, "npc: "); ok {
					got = append(got, "  "+rest)
				}
			}
			got = append(got, fmt.Sprintf("  after intention=%s lifetime=%d", intentionName(h), h.LifeTime()))
		default:
			t.Fatalf("golden line %q has no step", line)
		}
	}
	if !slices.Equal(got, r.Lines) {
		t.Fatalf("replay =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(r.Lines, "\n  "))
	}
}

// takeOrdered returns the lines since the last call in the order they were
// recorded.
func (l *seeLog) takeOrdered() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines := l.lines
	l.lines = nil
	return lines
}

// TestLookNeighborThrottlesPerNPC pins the look-around a behavior asks of
// its NPC: it tells the behavior about every creature in its see range,
// NPCs included, then does nothing until 30 s have passed since. Both
// executors check a second look right after the first; only the driven
// clock can place a look 1 ms either side of the 30 s mark, since on the
// wall clock the time between two looks is never that exact.
func TestLookNeighborThrottlesPerNPC(t *testing.T) {
	t.Parallel()
	log := newSeeLog()
	srv := bootSeeHooks(t, log)
	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	looker := spawnSeer(t, srv, log, "looker", seeMonster, location.Location{X: x + 200, Y: y, Z: z})
	srv.AI.Remove(looker)
	other := spawnSeer(t, srv, log, "bystander", seeBystander, location.Location{X: x + 300, Y: y, Z: z})
	srv.AI.Remove(other)
	drainUntilQuiet(t, srv.Client)
	look := func() {
		t.Helper()
		script.NPCOf(looker).LookNeighbor()
	}

	before := time.Now()
	look()
	wantSeeLines(t, log, "on the first look", "SEE looker bystander", "SEE looker p")
	look()
	if time.Since(before) >= 30*time.Second {
		t.Skip("30 s of wall time passed between the first two looks")
	}
	wantSeeLines(t, log, "right after the first look")
	if !srv.DrivesClock() {
		return
	}
	srv.Advance(t, 30*time.Second-time.Millisecond)
	look()
	wantSeeLines(t, log, "29.999 s after the first look")
	srv.Advance(t, time.Millisecond)
	look()
	wantSeeLines(t, log, "30 s after the first look", "SEE looker bystander", "SEE looker p")
}
