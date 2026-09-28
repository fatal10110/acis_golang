package combat

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// curseFearSkillID is Curse Fear, whose Fear effect (count 10, time 2) runs
// at half its count against a playable.
const curseFearSkillID = 1169

// fearFleeDistance is how far every flee runs from the effector.
const fearFleeDistance = 500

// landFear applies a real Fear effect from effector to target on target's
// own queue, the way a landed fear skill does, waits for its start hook to
// finish, and returns it.
func landFear(t *testing.T, effector effect.Actor, target effectHolder, skillID modelskill.ID, count int) *effect.Effect {
	t.Helper()
	e, err := effect.New(
		effect.Skill{ID: skillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "Fear", Count: count, Time: 2, Icon: true},
	)
	if err != nil {
		t.Fatalf("effect.New(Fear): %v", err)
	}
	e.Effector, e.Effected = effector, target
	done := make(chan struct{})
	if !target.Queue().Post(func() { target.EffectList().Add(e); close(done) }) {
		t.Fatal("post fear: queue closed")
	}
	<-done
	return e
}

// readMoveOf skips frames until a MoveToLocation for objectID arrives and
// returns its destination.
func readMoveOf(t *testing.T, c *scriptedClient, objectID int32, what string) location.Location {
	t.Helper()
	for {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			t.Fatalf("%s: no MoveToLocation for %d", what, objectID)
		}
		if frame[0] != serverpackets.OpcodeMoveToLocation {
			continue
		}
		if id, dest, _ := moveToLocationCoords(t, frame); id == objectID {
			return dest
		}
	}
}

// readFramesUntilQuiet returns every frame the client receives until the
// server stays quiet for a full read window.
func readFramesUntilQuiet(c *scriptedClient) [][]byte {
	var frames [][]byte
	for {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
}

// TestFearSendsMonsterFleeingEveryTick pins fear on a monster: the landing
// switches it to run stance and walks it 500 units straight away from the
// effector, and every later tick re-rolls the flee from where it now stands,
// for the full configured count.
func TestFearSendsMonsterFleeingEveryTick(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)
	// An idle wander puts the monster in walk stance first.
	tickThinkWander(t, hostile)
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	caster, ok := obj.(effect.Actor)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect actor", objID, obj)
	}
	px, py, _ := caster.Position()

	fear := landFear(t, caster, hostile, curseFearSkillID, 3)
	if !hostile.Afraid() {
		t.Fatal("Afraid() = false after fear landed, want true")
	}
	if got := fear.Remaining(); got != 3 {
		t.Fatalf("Remaining() = %d on a monster, want the configured 3", got)
	}
	runAt, moveAt := -1, -1
	var first, from location.Location
	for i, frame := range readFramesUntilQuiet(c) {
		switch frame[0] {
		case serverpackets.OpcodeChangeMoveType:
			if runAt < 0 {
				assertChangeMoveType(t, frame, hostile.ObjectID(), true)
				runAt = i
			}
		case serverpackets.OpcodeMoveToLocation:
			if id, dest, origin := moveToLocationCoords(t, frame); id == hostile.ObjectID() && moveAt < 0 {
				moveAt, first, from = i, dest, origin
			}
		}
	}
	if moveAt < 0 {
		t.Fatal("fear landing sent no flee MoveToLocation for the monster")
	}
	if runAt < 0 || runAt > moveAt {
		t.Fatalf("ChangeMoveType at %d, MoveToLocation at %d: want run stance before the flee", runAt, moveAt)
	}
	if want := from.FleeFrom(px, py, fearFleeDistance); first.X != want.X || first.Y != want.Y {
		t.Fatalf("landing flee dest = %+v from %+v, want %+v away from (%d,%d)", first, from, want, px, py)
	}

	srv.Advance(t, 2*time.Second)
	before := hostile.Move().Position()
	srv.TickEffects()
	dest := readMoveOf(t, c, hostile.ObjectID(), "tick flee")
	if want := before.FleeFrom(px, py, fearFleeDistance); dest.X != want.X || dest.Y != want.Y {
		t.Fatalf("tick flee dest = %+v from %+v, want %+v", dest, before, want)
	}
	if !hostile.Afraid() {
		t.Fatal("Afraid() = false after a flee tick, want the fear held")
	}

	for range 2 {
		srv.Advance(t, 2*time.Second)
		srv.TickEffects()
	}
	drainUntilQuiet(t, c)
	if hostile.Afraid() {
		t.Fatal("Afraid() = true after the count ran out, want the fear gone")
	}
}

// TestFearOnPlayerFleesOnceThenRefuses pins fear on a player. Curse Fear's
// count is halved before its schedule starts, and the halved count is what
// the effect saves. The landing makes the player run 500 units away from the
// effector; every later tick finds the player already afraid, so its flee is
// refused with ActionFailed and nothing moves.
func TestFearOnPlayerFleesOnceThenRefuses(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	player, ok := obj.(interface {
		effectHolder
		IsMoving() bool
	})
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}
	origin := location.Location{}
	origin.X, origin.Y, origin.Z = player.Position()

	fear := landFear(t, hostile, player, curseFearSkillID, 10)
	if got := fear.Remaining(); got != 5 {
		t.Fatalf("Remaining() = %d for Curse Fear on a player, want the halved 5", got)
	}
	if count, _ := fear.SaveState(time.Now()); count != 5 {
		t.Fatalf("SaveState count = %d, want the halved 5", count)
	}
	if !player.Afraid() {
		t.Fatal("Afraid() = false after fear landed, want true")
	}
	dest := readMoveOf(t, c, objID, "landing flee")
	if want := origin.FleeFrom(hostileX, hostileY, fearFleeDistance); dest.X != want.X || dest.Y != want.Y {
		t.Fatalf("landing flee dest = %+v, want %+v away from the monster", dest, want)
	}
	if !player.IsMoving() {
		t.Fatal("IsMoving() = false after the landing flee, want a live run")
	}
	drainUntilQuiet(t, c)

	srv.Advance(t, 2*time.Second)
	srv.TickEffects()
	var refused bool
	for _, frame := range readFramesUntilQuiet(c) {
		switch frame[0] {
		case serverpackets.OpcodeActionFailed:
			refused = true
		case serverpackets.OpcodeMoveToLocation:
			if id, _, _ := moveToLocationCoords(t, frame); id == objID {
				t.Fatal("tick flee moved an already-afraid player, want it refused")
			}
		}
	}
	if !refused {
		t.Fatal("tick flee sent no ActionFailed, want the refused move answered")
	}
	if got := fear.Remaining(); got != 4 {
		t.Fatalf("Remaining() = %d after one tick, want 4", got)
	}
}

// TestImmobileUntilAttackedExitStopsItsSkillEffects pins the exit of an
// ImmobileUntilAttacked effect: it removes every other effect its own skill
// applied, so both icons go and each removal is announced, the expired one as
// worn off and the one cut short as disappeared. The order of the two
// announcements is not pinned yet (#2630).
func TestImmobileUntilAttackedExitStopsItsSkillEffects(t *testing.T) {
	t.Parallel()
	const skillID = 4501
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	player, ok := obj.(effectHolder)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}

	meta := effect.Skill{ID: skillID, Level: 1, Debuff: true}
	done := make(chan struct{})
	if !player.Queue().Post(func() {
		effect.Apply(player, player, meta, []modelskill.EffectTemplate{
			{Name: "ImmobileUntilAttacked", Count: 1, Time: 2, Icon: true},
			{Name: "Debuff", Count: 1, Time: 60, Icon: true},
		})
		close(done)
	}) {
		t.Fatal("post effects: queue closed")
	}
	<-done
	drainUntilQuiet(t, c)
	if got := len(player.EffectList().All()); got != 2 {
		t.Fatalf("held effects = %d, want both of the skill's effects", got)
	}

	srv.Advance(t, 2*time.Second)
	srv.TickEffects()
	var messages []int32
	for _, frame := range readFramesUntilQuiet(c) {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		messages = append(messages, r.ReadInt32())
	}
	if len(player.EffectList().All()) != 0 {
		t.Fatalf("held effects after exit = %d, want none of the skill's effects left", len(player.EffectList().All()))
	}
	slices.Sort(messages)
	want := []int32{int32(serverpackets.SystemMessageS1HasWornOff), int32(serverpackets.SystemMessageEffectS1Disappeared)}
	if !slices.Equal(messages, want) {
		t.Fatalf("system messages = %v, want one worn off and one disappeared %v", messages, want)
	}
}

// shippedSkills loads the shared datapack's skill definitions, skipping the
// calling test when the datapack is not checked out next to the module.
func shippedSkills(t *testing.T) *modelskill.Table {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to resolve the test file path")
	}
	checkout := filepath.Join(filepath.Dir(thisFile), "..", "..")
	dir := filepath.Join(checkout, "..", "aCis_datapack", "data", "xml", "skills")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("aCis_datapack not checked out near the module root, skipping oracle comparison")
	}
	table, err := xmldata.LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	return table
}

// TestShippedFearCountsOnPlayableAndMonster pins the shipped Horror (65),
// Fear (1092) and Curse Fear (1169) effects, count 10 every 2 seconds: on a
// player each schedule starts, and saves, at 5; on a monster at the full 10.
// Casting each one twice leaves the shared template's count at 10.
func TestShippedFearCountsOnPlayableAndMonster(t *testing.T) {
	t.Parallel()
	skills := shippedSkills(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	player, ok := obj.(effectHolder)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}

	for _, id := range []modelskill.ID{65, 1092, 1169} {
		def, ok := skills.Get(id, 1)
		if !ok || len(def.Effects) != 1 || def.Effects[0].Name != "Fear" {
			t.Fatalf("shipped skill %d level 1 = %+v, want one Fear effect", id, def.Effects)
		}
		for _, tc := range []struct {
			effector effect.Actor
			target   effectHolder
			want     int
		}{
			{hostile, player, 5},
			{hostile, player, 5},
			{player, hostile, 10},
		} {
			e := landShippedEffect(t, tc.effector, tc.target, def)
			if got := e.Remaining(); got != tc.want {
				t.Fatalf("skill %d on %T: Remaining() = %d, want %d", id, tc.target, got, tc.want)
			}
			if count, _ := e.SaveState(time.Now()); int(count) != tc.want {
				t.Fatalf("skill %d on %T: SaveState count = %d, want %d", id, tc.target, count, tc.want)
			}
			stopFear(t, tc.target)
		}
		if again, _ := skills.Get(id, 1); again.Effects[0].Count != 10 {
			t.Fatalf("shipped skill %d template count = %d after casts, want 10", id, again.Effects[0].Count)
		}
	}
}

// landShippedEffect builds def's single effect from its shipped template and
// lands it from effector on target, on target's queue.
func landShippedEffect(t *testing.T, effector effect.Actor, target effectHolder, def modelskill.Definition) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.SkillFromDefinition(def), def.Effects[0])
	if err != nil {
		t.Fatalf("effect.New(%s): %v", def.Effects[0].Name, err)
	}
	done := make(chan struct{})
	if !target.Queue().Post(func() { effect.Attach(e, effector, target); close(done) }) {
		t.Fatal("post fear: queue closed")
	}
	<-done
	return e
}

// stopFear ends every fear target holds, on target's queue.
func stopFear(t *testing.T, target effectHolder) {
	t.Helper()
	done := make(chan struct{})
	if !target.Queue().Post(func() { target.StopEffects(effect.TypeFear); close(done) }) {
		t.Fatal("post stop: queue closed")
	}
	<-done
	if target.Afraid() {
		t.Fatalf("%T still afraid after StopEffects(FEAR)", target)
	}
}
