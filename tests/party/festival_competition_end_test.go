package party

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// dueTimers is a clock and its timers, run in the order they come due.
type dueTimers struct {
	mu    sync.Mutex
	now   time.Time
	queue []dueTimer
}

type dueTimer struct {
	at time.Time
	fn func()
}

func (c *dueTimers) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *dueTimers) after(d time.Duration, fn func()) *time.Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queue = append(c.queue, dueTimer{at: c.now.Add(d), fn: fn})
	return nil
}

// next runs the timer due first, of timers due together the one armed
// first, moving the clock on to it.
func (c *dueTimers) next(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	if len(c.queue) == 0 {
		c.mu.Unlock()
		t.Fatal("no festival timer armed")
	}
	first := 0
	for i, d := range c.queue {
		if d.at.Before(c.queue[first].at) {
			first = i
		}
	}
	d := c.queue[first]
	c.queue = append(c.queue[:first], c.queue[first+1:]...)
	c.now = d.at
	c.mu.Unlock()
	d.fn()
}

// pending are the delays, from now, of the timers still armed.
func (c *dueTimers) pending() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []time.Duration
	for _, d := range c.queue {
		out = append(out, d.at.Sub(c.now))
	}
	return out
}

// talk has player i talk to the guide and returns its page, links cut,
// leaving every client drained.
func (f *festivalRun) talk(t *testing.T, i int) string {
	t.Helper()
	p := f.players[i]
	x, y, z := f.srv.PlayerPosition(t, p.id)
	// A first click only selects the guide; the next one talks.
	for range 2 {
		p.c.Send(encodeAction(f.guide.ObjectID(), x, y, z))
		if frames := drainFrames(t, p.c); len(frames) > 0 {
			if _, ok := firstFrame(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
				// The others see the talker turn to the guide.
				f.quiet(t)
				return festivalPage(t, frames)
			}
		}
	}
	t.Fatalf("%s: talking to the guide opened no page", p.name)
	return ""
}

// The competition's end stops only the festival cycles not yet begun: a
// cycle whose signup is under way keeps opening a new signup period while
// nobody signs up, through the results, seal validation and recruiting.
// The guides keep counting a fresh 1 to 20 minutes down, and a party
// signing up in the recruiting period runs its festival when the signup
// ends. That festival is the cycle's last: it runs to its end even as the
// next competition begins, its party taken back out, while the schedule
// the competition starts runs its own first signup.
func TestFestivalSignupOutlivesTheCompetitionEnd(t *testing.T) {
	t.Parallel()
	clock := &dueTimers{now: festivalStart}
	var changeMu sync.Mutex
	var change func()
	pages := festivalPages()
	pages["seven_signs/festival/dawn_guide.htm"] = "<html><body>guide %festivalMins%" + festivalLinks + "</body></html>"
	f := bootFestivalWith(t, festivalSetup{
		seats:      []seat{{"Leader", 20}, {"Member", 20}},
		blueStones: 1000,
		minPlayers: 2,
		pages:      pages,
		extra: []gameservertest.Option{
			gameservertest.WithFestivalClock(clock.Now),
			gameservertest.WithFestivalTimer(clock.after),
			gameservertest.WithSevenSignsClock(clock.Now),
			gameservertest.WithSevenSignsTimer(func(_ time.Duration, fn func()) *time.Timer {
				changeMu.Lock()
				defer changeMu.Unlock()
				change = fn
				return nil
			}),
		},
	})
	periodChange := func() {
		changeMu.Lock()
		fn := change
		changeMu.Unlock()
		fn()
		f.quiet(t)
	}
	notice := func(minutes string) string {
		return `guide <font color="FF0000">The next festival will begin in ` + minutes + ` minute(s).</font>`
	}

	clock.next(t) // the first cycle begins
	clock.next(t) // its signup ends with nobody signed up
	periodChange()
	if got := f.talk(t, 0); got != notice("20") {
		t.Fatalf("guide as the results begin = %q", got)
	}
	clock.next(t) // the next signup ends with nobody signed up
	if got := f.talk(t, 0); got != notice("20") {
		t.Fatalf("guide a signup period into the results = %q", got)
	}

	periodChange() // seal validation
	clock.next(t)
	periodChange() // recruiting
	clock.next(t)
	if got := f.talk(t, 0); got != notice("20") {
		t.Fatalf("guide in the recruiting period = %q", got)
	}

	f.invite(t, 0, 1, 0)
	f.command(t, 1, "1")
	f.command(t, 0, "1")
	if got := festivalPage(t, f.command(t, 0, "2 6360")); got != "page 2e" {
		t.Fatalf("payment = %q, want page 2e", got)
	}
	origins := map[int32][2]int{}
	for _, p := range f.players {
		x, y, _ := f.srv.PlayerPosition(t, p.id)
		origins[p.id] = [2]int{x, y}
	}
	f.quiet(t)

	clock.next(t) // the signup ends: the party is taken into the arena
	for _, p := range f.players {
		frames := drainFrames(t, p.c)
		if _, ok := firstFrame(frames, serverpackets.OpcodeTeleportToLocation); !ok {
			t.Fatalf("%s frames %x, want a teleport into the arena", p.name, opcodes(frames))
		}
		p.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	f.quiet(t)
	if !f.srv.Festival.Initialized() {
		t.Fatal("the festival did not begin")
	}

	periodChange() // the next competition begins mid-festival
	for steps := 0; f.srv.Festival.Initialized(); steps++ {
		if steps == 10 {
			t.Fatal("the festival never ended")
		}
		clock.next(t)
	}
	for _, p := range f.players {
		frames := drainFrames(t, p.c)
		want := []string{"You have been removed from the festival arena.", "The festival has ended. Your party leader must now register your score before the next festival takes place."}
		if got := texts(frames); !equalStrings(got, want) {
			t.Fatalf("%s told %q, want %q", p.name, got, want)
		}
		x, y, _ := f.srv.PlayerPosition(t, p.id)
		if o := origins[p.id]; !near(x, y, o[0], o[1], 20) {
			t.Fatalf("%s at (%d, %d), want back near (%d, %d)", p.name, x, y, o[0], o[1])
		}
		p.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	f.quiet(t)
	// The new schedule's first signup, begun 2 minutes after the
	// competition, ends 19 minutes later: 3 minutes after the festival's
	// 18.
	if got := f.talk(t, 0); got != notice("4") {
		t.Fatalf("guide after the last festival = %q", got)
	}
	if got := clock.pending(); len(got) != 1 || got[0] != 3*time.Minute {
		t.Fatalf("timers armed after the last festival = %v, want only the new signup's end in 3m", got)
	}
	clock.next(t)
	if got := f.talk(t, 0); got != notice("20") {
		t.Fatalf("guide after the new schedule's first signup = %q", got)
	}
}
