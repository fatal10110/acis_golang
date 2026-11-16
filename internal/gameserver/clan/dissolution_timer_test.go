package clan

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// Ids of the dissolution timer world.
const (
	dtClan   = 1
	dtLeader = 1000
)

var dtStart = time.UnixMilli(1_800_000_000_000)

// dueRecorder is the Dissolver of the timer tests: it destroys the clan
// as the network side does and records when each dissolution came due.
type dueRecorder struct {
	s   *Service
	in  *sim.Inline
	due []time.Time
}

func (r *dueRecorder) DissolveDue(cl *Clan) {
	r.due = append(r.due, r.in.Now())
	r.s.Destroy(cl, true, nil, r.in.Now())
}

// dissolutionTimerWorld restores one clan, its dissolution pending until
// expiry when it is not 0, and runs the dissolutions on an inline clock
// starting at dtStart.
func dissolutionTimerWorld(t *testing.T, expiry int64) (*Service, *sim.Inline, *dueRecorder) {
	t.Helper()
	snap := Snapshot{
		Clans:   []Row{{ID: dtClan, Name: "Timed", Level: 3, LeaderID: dtLeader, DissolvingExpiry: expiry}},
		Members: []MemberRow{{ClanID: dtClan, Member: Member{ObjectID: dtLeader, Name: "Leader"}}},
	}
	table := NewTable()
	table.Restore(snap, dtStart, 1)
	s := NewService(table, newFakeStore(), &laneWriter{}, nil, DefaultConfig(), nil, zerolog.Nop())
	in := sim.NewInline(dtStart)
	r := &dueRecorder{s: s, in: in}
	s.StartDissolutions(in.NewQueue("clan-dissolution"), r)
	return s, in, r
}

func dissolveDelay(s *Service) time.Duration {
	return time.Duration(s.cfg.DissolveDays) * 24 * time.Hour
}

// TestRequestedDissolutionComesDue requests a dissolution at runtime: it
// comes due once, exactly DissolveDays later, and destroys the clan.
func TestRequestedDissolutionComesDue(t *testing.T) {
	s, in, r := dissolutionTimerWorld(t, 0)
	leader := inClan(dtLeader, "Leader", dtClan)
	if _, got := s.RequestDissolve(leader, in.Now(), nil); got != DissolveScheduled {
		t.Fatalf("RequestDissolve = %d, want DissolveScheduled", got)
	}
	delay := dissolveDelay(s)

	in.AdvanceBefore(delay)
	if len(r.due) != 0 {
		t.Fatalf("dissolution came due early, at %v", r.due)
	}
	in.Advance(0)
	if want := dtStart.Add(delay); len(r.due) != 1 || !r.due[0].Equal(want) {
		t.Fatalf("dissolution came due at %v, want once at %v", r.due, want)
	}
	if _, ok := s.table.Get(dtClan); ok {
		t.Fatal("clan still registered after its dissolution came due")
	}
	in.Advance(2 * delay)
	if len(r.due) != 1 {
		t.Fatalf("dissolution came due %d times, want once", len(r.due))
	}
}

// TestRecoveredDissolutionNeverComesDue recovers a requested dissolution
// before its deadline: nothing comes due, and the clan stays.
func TestRecoveredDissolutionNeverComesDue(t *testing.T) {
	s, in, r := dissolutionTimerWorld(t, 0)
	leader := inClan(dtLeader, "Leader", dtClan)
	if _, got := s.RequestDissolve(leader, in.Now(), nil); got != DissolveScheduled {
		t.Fatalf("RequestDissolve = %d, want DissolveScheduled", got)
	}
	in.Advance(time.Hour)
	if _, got := s.RecoverClan(leader); got != Recovered {
		t.Fatalf("RecoverClan = %d, want Recovered", got)
	}

	in.Advance(3 * dissolveDelay(s))
	if len(r.due) != 0 {
		t.Fatalf("recovered dissolution came due at %v", r.due)
	}
	if _, ok := s.table.Get(dtClan); !ok {
		t.Fatal("recovered clan left the registry")
	}
}

// TestRerequestedDissolutionComesDueAtTheFirstDeadline dissolves,
// recovers and dissolves again an hour later: recovering leaves the first
// request's timer armed, and as it runs out a dissolution is pending
// again, so the clan is destroyed once, at the first request's deadline.
func TestRerequestedDissolutionComesDueAtTheFirstDeadline(t *testing.T) {
	s, in, r := dissolutionTimerWorld(t, 0)
	leader := inClan(dtLeader, "Leader", dtClan)
	delay := dissolveDelay(s)
	if _, got := s.RequestDissolve(leader, in.Now(), nil); got != DissolveScheduled {
		t.Fatalf("first RequestDissolve = %d, want DissolveScheduled", got)
	}
	in.Advance(30 * time.Minute)
	if _, got := s.RecoverClan(leader); got != Recovered {
		t.Fatalf("RecoverClan = %d, want Recovered", got)
	}
	in.Advance(30 * time.Minute)
	if _, got := s.RequestDissolve(leader, in.Now(), nil); got != DissolveScheduled {
		t.Fatalf("second RequestDissolve = %d, want DissolveScheduled", got)
	}

	in.AdvanceBefore(delay - time.Hour) // up to the first deadline
	if len(r.due) != 0 {
		t.Fatalf("dissolution came due at %v, before the first deadline", r.due)
	}
	in.Advance(0)
	if want := dtStart.Add(delay); len(r.due) != 1 || !r.due[0].Equal(want) {
		t.Fatalf("dissolution came due at %v, want once at %v", r.due, want)
	}
	if _, ok := s.table.Get(dtClan); ok {
		t.Fatal("clan still stands after the first request's deadline")
	}
	in.Advance(2 * delay)
	if len(r.due) != 1 {
		t.Fatalf("dissolution came due %d times, want once", len(r.due))
	}
}

// TestRequestOverARestoredTimerComesDueAtTheMinute restores a clan whose
// dissolution passed while the server was down, so its timer waits the
// one-minute minimum, and has its leader request a new dissolution within
// that minute: the restored timer, running out with a dissolution pending,
// destroys the clan at the minute, once.
func TestRequestOverARestoredTimerComesDueAtTheMinute(t *testing.T) {
	s, in, r := dissolutionTimerWorld(t, dtStart.Add(-time.Hour).UnixMilli())
	leader := inClan(dtLeader, "Leader", dtClan)
	in.Advance(30 * time.Second)
	if _, got := s.RequestDissolve(leader, in.Now(), nil); got != DissolveScheduled {
		t.Fatalf("RequestDissolve over a lapsed expiry = %d, want DissolveScheduled", got)
	}

	in.Advance(30 * time.Second)
	if want := dtStart.Add(time.Minute); len(r.due) != 1 || !r.due[0].Equal(want) {
		t.Fatalf("dissolution came due at %v, want once at %v", r.due, want)
	}
	in.Advance(2 * dissolveDelay(s))
	if len(r.due) != 1 {
		t.Fatalf("dissolution came due %d times, want once", len(r.due))
	}
}
