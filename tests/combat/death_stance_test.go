package combat

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// deathStanceWorld is a player with an observer beside it, on a stance clock
// the test moves by hand.
type deathStanceWorld struct {
	srv      *gameservertest.Server
	victim   *scriptedClient
	observer *scriptedClient
	victimID int32
	nowMS    *atomic.Int64
}

func bootDeathStance(t *testing.T) *deathStanceWorld {
	t.Helper()
	nowMS := new(atomic.Int64)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }),
	)
	w := &deathStanceWorld{srv: srv, victim: srv.Client, victimID: srv.SoleObjectID(t), nowMS: nowMS}
	startInWorld(t, w.victim)
	srv.SeedCharacterFor(t, "observer", "Observer", 5, 0)
	w.observer = srv.DialClient(t, "observer", 1)
	startInWorld(t, w.observer)
	drainUntilQuiet(t, w.victim)
	drainUntilQuiet(t, w.observer)
	return w
}

// enterStance has the victim swing at a monster until it is in attack stance.
func (w *deathStanceWorld) enterStance(t *testing.T) {
	t.Helper()
	c := w.victim
	hostile := w.srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, c, w.victimID)
	w.srv.AdvanceUntil(t, "opening swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	awaitAutoAttackStart(t, c, w.victimID)
	if !w.srv.AttackStance.InAttackStance(worldActor{id: w.victimID}) || !onlinePlayer(t, w.srv, w.victimID).InCombat() {
		t.Fatal("the swing left the victim out of attack stance")
	}
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w.observer)
}

func (w *deathStanceWorld) kill(t *testing.T) {
	t.Helper()
	onPlayerQueue(t, w.srv, w.victimID, func(pc *player.Character) { pc.Kill(nil) })
}

// expireOldStance moves the stance clock past the window the victim's last
// swing opened and runs the expiry sweep.
func (w *deathStanceWorld) expireOldStance(t *testing.T) {
	t.Helper()
	w.nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := w.srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	w.srv.Settle(t)
}

// TestPlayerDeathEndsAttackStance pins that a player killed in attack stance
// leaves it at its death: the player and every observer get the player's
// AutoAttackStop as the frame right after its Die, the tracker and the
// in-combat flag are cleared, and a revive inside the old window reads out
// of combat with no second AutoAttackStop when that window would have ended.
func TestPlayerDeathEndsAttackStance(t *testing.T) {
	t.Parallel()
	w := bootDeathStance(t)
	w.enterStance(t)

	w.kill(t)
	for _, c := range []struct {
		name   string
		client *scriptedClient
	}{{"victim", w.victim}, {"observer", w.observer}} {
		frames := framesUntilQuiet(c.client)
		die := indexOf(frames, 0, serverpackets.OpcodeDie, w.victimID)
		if die < 0 {
			t.Fatalf("%s never received the victim's Die", c.name)
		}
		stop := indexOf(frames, 0, serverpackets.OpcodeAutoAttackStop, w.victimID)
		if stop != die+1 {
			t.Fatalf("%s: victim AutoAttackStop at frame %d, want %d right after Die at %d", c.name, stop, die+1, die)
		}
		if again := indexOf(frames, stop+1, serverpackets.OpcodeAutoAttackStop, w.victimID); again >= 0 {
			t.Fatalf("%s: second victim AutoAttackStop at frame %d", c.name, again)
		}
	}
	if w.srv.AttackStance.InAttackStance(worldActor{id: w.victimID}) {
		t.Fatal("the dead victim is still in the stance tracker")
	}
	if onlinePlayer(t, w.srv, w.victimID).InCombat() {
		t.Fatal("the dead victim still reads in combat")
	}

	var revived bool
	onPlayerQueue(t, w.srv, w.victimID, func(pc *player.Character) { revived = pc.Revive() })
	if !revived {
		t.Fatal("Revive() = false")
	}
	if onlinePlayer(t, w.srv, w.victimID).InCombat() {
		t.Fatal("the revived victim reads in combat inside the old stance window")
	}
	drainUntilQuiet(t, w.victim)
	drainUntilQuiet(t, w.observer)

	w.expireOldStance(t)
	for _, c := range []struct {
		name   string
		client *scriptedClient
	}{{"victim", w.victim}, {"observer", w.observer}} {
		if stop := indexOf(framesUntilQuiet(c.client), 0, serverpackets.OpcodeAutoAttackStop, w.victimID); stop >= 0 {
			t.Fatalf("%s got a victim AutoAttackStop at the old stance expiry", c.name)
		}
	}
}

// TestPlayerDeathOutOfStanceSendsNoAutoAttackStop pins that a player killed
// outside attack stance gets no AutoAttackStop with its Die.
func TestPlayerDeathOutOfStanceSendsNoAutoAttackStop(t *testing.T) {
	t.Parallel()
	w := bootDeathStance(t)

	w.kill(t)
	for _, c := range []struct {
		name   string
		client *scriptedClient
	}{{"victim", w.victim}, {"observer", w.observer}} {
		frames := framesUntilQuiet(c.client)
		if indexOf(frames, 0, serverpackets.OpcodeDie, w.victimID) < 0 {
			t.Fatalf("%s never received the victim's Die", c.name)
		}
		if stop := indexOf(frames, 0, serverpackets.OpcodeAutoAttackStop, w.victimID); stop >= 0 {
			t.Fatalf("%s got a victim AutoAttackStop at frame %d for a death out of stance", c.name, stop)
		}
	}
	if onlinePlayer(t, w.srv, w.victimID).InCombat() {
		t.Fatal("the dead victim reads in combat")
	}
}
