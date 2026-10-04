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
	if _, got := s.RequestDissolve(leader, in.Now()); got != DissolveScheduled {
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
	if _, got := s.RequestDissolve(leader, in.Now()); got != DissolveScheduled {
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

// TestRerequestedDissolutionComesDueAtTheSecondDeadline dissolves,
// recovers and dissolves again an hour later: the clan is destroyed once,
// at the second request's deadline, not at the first one's.
func TestRerequestedDissolutionComesDueAtTheSecondDeadline(t *testing.T) {
	s, in, r := dissolutionTimerWorld(t, 0)
	leader := inClan(dtLeader, "Leader", dtClan)
	delay := dissolveDelay(s)
	if _, got := s.RequestDissolve(leader, in.Now()); got != DissolveScheduled {
		t.Fatalf("first RequestDissolve = %d, want DissolveScheduled", got)
	}
	in.Advance(30 * time.Minute)
	if _, got := s.RecoverClan(leader); got != Recovered {
		t.Fatalf("RecoverClan = %d, want Recovered", got)
	}
	in.Advance(30 * time.Minute)
	if _, got := s.RequestDissolve(leader, in.Now()); got != DissolveScheduled {
		t.Fatalf("second RequestDissolve = %d, want DissolveScheduled", got)
	}

	in.AdvanceBefore(delay) // an hour past the first deadline, up to the second
	if len(r.due) != 0 {
		t.Fatalf("dissolution came due at %v, before the second deadline", r.due)
	}
	if _, ok := s.table.Get(dtClan); !ok {
		t.Fatal("clan destroyed at the first request's deadline")
	}
	in.Advance(0)
	if want := dtStart.Add(time.Hour + delay); len(r.due) != 1 || !r.due[0].Equal(want) {
		t.Fatalf("dissolution came due at %v, want once at %v", r.due, want)
	}
	in.Advance(2 * delay)
	if len(r.due) != 1 {
		t.Fatalf("dissolution came due %d times, want once", len(r.due))
	}
}

// TestRequestReplacesARestoredTimer restores a clan whose dissolution
// passed while the server was down, so its timer waits the one-minute
// minimum, and has its leader request a new dissolution within that
// minute: the new request's timer replaces the restored one, so the clan
// comes due once, at the new deadline.
func TestRequestReplacesARestoredTimer(t *testing.T) {
	s, in, r := dissolutionTimerWorld(t, dtStart.Add(-time.Hour).UnixMilli())
	leader := inClan(dtLeader, "Leader", dtClan)
	in.Advance(30 * time.Second)
	if _, got := s.RequestDissolve(leader, in.Now()); got != DissolveScheduled {
		t.Fatalf("RequestDissolve over a lapsed expiry = %d, want DissolveScheduled", got)
	}

	in.Advance(time.Hour)
	if len(r.due) != 0 {
		t.Fatalf("replaced restored timer came due at %v", r.due)
	}
	in.Advance(dissolveDelay(s))
	if want := dtStart.Add(30*time.Second + dissolveDelay(s)); len(r.due) != 1 || !r.due[0].Equal(want) {
		t.Fatalf("dissolution came due at %v, want once at %v", r.due, want)
	}
}
