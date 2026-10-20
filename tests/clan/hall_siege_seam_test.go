package clan

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: a siegable hall's script overrides ClanHallSiege's steps
// (FlagWar.prepareSiege: no free(), no message, its own start schedule;
// FlagWar.startSiege and endSiege), which the siege task and instantSiege
// dispatch to; FortressOfResistance and FlagWar override loadAttackers to
// load none of clanhall_siege_attackers; FlagWar.doorIsAutoAttackable is
// false (Door.isAttackableBy); SiegableHall.addAttacker/removeAttacker
// change the attackers ClanHallManager stores. Q504 and Q655 read
// getSiegableHall, isWaitingBattle and getSiegeDate.

const (
	seamQuest = "Q962_HallSeam"
	// devastatedGateName is the gate Hall.CloseDoor closes, one of
	// Devastated Castle's gates.
	devastatedGateName = "Devastated_castle_001"
)

// seamRecord is what the script's step overrides saw, in order.
type seamRecord struct {
	mu     sync.Mutex
	steps  []string
	delays []time.Duration
}

func (r *seamRecord) add(step string, delay time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, step)
	r.delays = append(r.delays, delay)
}

func (r *seamRecord) seen() ([]string, []time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.steps), slices.Clone(r.delays)
}

func seamScripts() gameservertest.Option {
	return gameservertest.WithScripts([]script.Listing{{Path: "quest." + seamQuest}}, script.Catalog{
		"quest." + seamQuest: func() script.Script { return script.Script{Title: "Hall Seam", QuestID: 962} },
	})
}

// TestHallSiegeScriptStepOverrides binds Devastated Castle's siege from a
// script whose registrations' close is its own: it banishes, sets the
// status and arms the start itself, and neither frees the hall nor tells
// anyone. Its start and end run the base steps as their part. The siege's
// timer runs the close an hour before the date, the armed start runs the
// start, End and Instant run the overrides. The attackers the script adds
// and removes are stored at once, and the script reads the hall as the
// hall quests do.
func TestHallSiegeScriptStepOverrides(t *testing.T) {
	t.Parallel()
	date := time.Now().Add(time.Hour + time.Minute).UnixMilli()
	w := bootHallSiege(t, hallSiegeSetup{owned: true, unbound: true, endDate: date, extra: []gameservertest.Option{seamScripts()}})
	rec := &seamRecord{}
	var winners atomic.Int32
	hooks := script.HallSiegeHooks{
		Winner: func(h *script.Hall) int32 {
			winners.Add(1)
			return besiegersClanID
		},
		Prepare: func(h *script.Hall, _ script.HallBase, delay time.Duration) {
			rec.add("prepare", delay)
			h.BanishForeigners()
			h.SetSiegeStatus(script.SiegeRegistrationOver)
			h.ArmStart(delay)
		},
		Start: func(_ *script.Hall, base script.HallBase) {
			rec.add("start", 0)
			base.Start()
		},
		End: func(_ *script.Hall, base script.HallBase) {
			rec.add("end", 0)
			base.End()
		},
	}
	var h *script.Hall
	w.srv.RunScript(t, w.objID, seamQuest, func(s *script.Script, _ *script.Player) {
		h = s.BindHallSiege(devastatedCastle, hooks)
		if s.BindHallSiege(devastatedCastle, hooks) != nil {
			t.Error("Devastated Castle's siege bound twice")
		}
	})
	if h == nil {
		t.Fatal("Devastated Castle's siege did not bind")
	}

	// The attackers a script adds and removes are stored at once.
	if !h.AddAttacker(besiegersClanID) || h.AddAttacker(besiegersClanID) {
		t.Fatal("Besiegers joined twice, or not at all")
	}
	if got := w.storedAttackers(t, devastatedCastle); !slices.Equal(got, []int32{besiegersClanID}) {
		t.Fatalf("stored attackers after the add = %#x, want Besiegers", got)
	}
	h.RemoveAttacker(besiegersClanID)
	if got := w.storedAttackers(t, devastatedCastle); len(got) != 0 {
		t.Fatalf("stored attackers after the removal = %#x, want none", got)
	}
	h.AddAttacker(besiegersClanID)
	h.AddAttacker(holdersClanID)
	h.ClearAttackers()
	if got := w.storedAttackers(t, devastatedCastle); len(got) != 0 || len(h.Attackers()) != 0 {
		t.Fatalf("attackers after the clear = %#x, stored %#x, want none", h.Attackers(), got)
	}
	h.AddAttacker(besiegersClanID)
	drainFrames(t, w.c)

	// The quests' reads, and the clans a hall script reads.
	w.srv.RunScript(t, w.objID, seamQuest, func(s *script.Script, _ *script.Player) {
		read := s.SiegableHall(devastatedCastle)
		if read == nil || read.ID() != devastatedCastle || read.Name() != "Devastated Castle" {
			t.Errorf("SiegableHall(34) = %v", read)
			return
		}
		if !read.Registering() || read.WaitingBattle() || read.InSiege() || read.Unowned() || read.OwnerID() != holdersClanID {
			t.Errorf("before the close: registering %v, waiting %v, in siege %v, owner %#x",
				read.Registering(), read.WaitingBattle(), read.InSiege(), read.OwnerID())
		}
		if !read.SiegeDate().Equal(time.UnixMilli(date)) || read.SiegeLength() != time.Hour {
			t.Errorf("siege date %v, length %v; want %v, 1h", read.SiegeDate(), read.SiegeLength(), time.UnixMilli(date))
		}
		if at, ok := read.RandomSpawn(script.HallSpawnBanish); !ok || at.X != devastatedBanish.X || at.Y != devastatedBanish.Y {
			t.Errorf("banish spawn = %+v %v, want %+v", at, ok, devastatedBanish)
		}
		if s.SiegableHall(1) != nil {
			t.Error("hall 1 reads as a siegable hall")
		}
		cl := s.Clan(besiegersClanID)
		if cl == nil || cl.Name() != "Besiegers" || cl.LeaderName() != "Attacker" || cl.LeaderID() != w.objID ||
			cl.MembersCount() != 1 || len(cl.OnlineMembers()) != 1 || cl.HasHall() || cl.DissolvingExpiry() != 0 {
			t.Errorf("Besiegers reads as %+v", cl)
		}
		if h := s.Clan(holdersClanID); h == nil || !h.HasHall() {
			t.Error("Holders reads as owning no hall")
		}
		read.CloseDoor(devastatedGateID)
	})
	if _, open := w.srv.DoorState(t, devastatedGateID); open {
		t.Fatalf("%s stands open after the hall closed it", devastatedGateName)
	}

	// The siege's timer runs the script's close an hour before the date.
	w.srv.Advance(t, time.Duration(date-w.srv.Halls.Now()-3_600_000)*time.Millisecond)
	if steps, delays := rec.seen(); !slices.Equal(steps, []string{"prepare"}) || delays[0] != time.Hour {
		t.Fatalf("after the close's time: steps %v, delays %v; want prepare after 1h", steps, delays)
	}
	if !w.siege.WaitingBattle() {
		t.Fatal("the script's close left the registrations open")
	}
	if owner, _, _ := w.hallRow(t); owner != int64(holdersClanID) {
		t.Fatalf("hall owner after the script's close = %#x, want Holders kept", owner)
	}
	for _, m := range sysMsgParams(t, drainFrames(t, w.c)) {
		if m[0] == "293" {
			t.Fatalf("the script's close told everyone %v", m)
		}
	}
	if !slices.Equal(w.siege.Attackers(), []int32{besiegersClanID}) {
		t.Fatalf("attackers after the script's close = %#x, want Besiegers alone", w.siege.Attackers())
	}

	// The start the script armed runs its start, which runs the base start.
	w.srv.Advance(t, time.Hour)
	if steps, _ := rec.seen(); !slices.Equal(steps, []string{"prepare", "start"}) || !w.siege.InProgress() {
		t.Fatalf("after the start's time: steps %v, in progress %v", steps, w.siege.InProgress())
	}
	if !w.field.Active() {
		t.Fatal("the base start left the battlefield off")
	}

	// End runs the script's end, which runs the base end: it asks the
	// winner first; no mission was accomplished, so it is a draw.
	if !w.siege.End() {
		t.Fatal("no siege to end")
	}
	if n := winners.Load(); n != 1 {
		t.Fatalf("the end asked the winner %d times, want once", n)
	}
	if steps, _ := rec.seen(); !slices.Equal(steps, []string{"prepare", "start", "end"}) || !w.siege.Registering() {
		t.Fatalf("after the end: steps %v, registering %v", steps, w.siege.Registering())
	}
	if got := w.storedAttackers(t, devastatedCastle); len(got) != 0 {
		t.Fatalf("stored attackers after the end = %#x, want none", got)
	}

	// Instant runs the script's close at once.
	if !w.siege.Instant() {
		t.Fatal("no siege to close")
	}
	if steps, delays := rec.seen(); len(steps) != 4 || steps[3] != "prepare" || delays[3] != 0 {
		t.Fatalf("after Instant: steps %v, delays %v; want a prepare after 0", steps, delays)
	}
}

// TestHallSiegeScriptOwnAttackersAndDoors binds Devastated Castle's siege
// to a script loading its own attackers and keeping the doors out of
// attacks: the stored attacker row is neither taken nor touched, and during
// the siege the attacking Attacker still may not attack the gate.
func TestHallSiegeScriptOwnAttackersAndDoors(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{
		unbound: true,
		seed:    []string{"INSERT INTO clanhall_siege_attackers VALUES (34, " + itoa(besiegersClanID) + ")"},
		extra: []gameservertest.Option{gameservertest.WithHallSiegeScript(devastatedCastle,
			clanhall.SiegeScript{OwnAttackers: true, DoorsUnattackable: true})},
	})
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers at bind = %#x, want none", got)
	}
	if got := w.storedAttackers(t, devastatedCastle); !slices.Equal(got, []int32{besiegersClanID}) {
		t.Fatalf("stored attackers = %#x, want the row untouched", got)
	}
	w.siege.AddAttacker(besiegersClanID)
	if !w.siege.Instant() {
		t.Fatal("no siege to start")
	}
	w.srv.Advance(t, 0)
	if !w.siege.Attacking(besiegersClanID) {
		t.Fatal("Besiegers does not attack the siege under way")
	}
	if w.srv.DoorAttackableBy(t, devastatedGateID, w.objID) {
		t.Fatal("the gate is attackable during a siege whose script keeps doors out of attacks")
	}
}
