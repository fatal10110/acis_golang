package castlemanor

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

// TestCycleMatchesReferenceProbe replays "cycle <hh:mm> <treasury> <roll>
// <step> <action> => ...": days of the cycle on gludioRows, step by step. A
// step starts the manor, runs the next mode change, sells seeds from
// Gludio's first running seed or resets Gludio, then runs the changes already
// due; the line is the state after it.
func TestCycleMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	var (
		h    *harness
		head string
	)
	for _, l := range manorGolden(t, "cycle") {
		f := strings.Fields(l[0])
		if c := strings.Join(f[:3], " "); c != head || f[4] == "start" {
			head = c
			var hour, minute int
			if _, err := fmt.Sscanf(f[0], "%d:%d", &hour, &minute); err != nil {
				t.Fatal(err)
			}
			h = start(t, at(hour, minute), must(strconv.ParseInt(f[1], 10, 64)), gludioRows(), must(strconv.Atoi(f[2])))
		}
		switch action := strings.Join(f[4:], " "); {
		case action == "start":
		case action == "run":
			h.clock.Advance(max(0, h.m.NextModeChange().Sub(h.clock.Now())))
		case strings.HasPrefix(action, "sell "):
			n := int32(must(strconv.Atoi(f[5])))
			if !h.m.SeedProduction(gludio, false)[0].DecreaseAmount(n) {
				t.Fatalf("%s: cannot %s", l[0], action)
			}
		case action == "reset":
			h.m.Reset(gludio)
		default:
			t.Fatalf("unknown step %q", action)
		}
		h.clock.Advance(0)
		if got := h.cycleState(t); got != l[1] {
			t.Errorf("cycle %s:\n got %s\nwant %s", l[0], got, l[1])
		}
	}
}

// cycleState renders the manor as a cycle line does, taking the effects
// since the last call.
func (h *harness) cycleState(t *testing.T) string {
	t.Helper()
	var sent, items []string
	for _, e := range h.effects.take() {
		var clanID, count, item int
		if leader, ok := strings.CutPrefix(e, "updated "); ok {
			if leader != fmt.Sprintf("%#x", lordsLeader) {
				t.Fatalf("manor update told %s", leader)
			}
			sent = append(sent, "THE_MANOR_INFORMATION_HAS_BEEN_UPDATED")
			continue
		}
		if _, err := fmt.Sscanf(e, "warehouse 0x%x +%d of %d", &clanID, &count, &item); err != nil {
			t.Fatalf("effect %q: %v", e, err)
		}
		items = append(items, fmt.Sprintf("%d:%d", item, count))
	}
	orDash := func(s []string) string {
		if len(s) == 0 {
			return "-"
		}
		return strings.Join(s, ",")
	}
	_, _, saves := h.store.state()
	gl, _ := h.castles.Get(gludio)
	return fmt.Sprintf("mode %s delay %d stores %d sent %s warehouse %s income %d treasury %d gludio%s /%s dion%s /%s",
		h.m.Status(), h.m.NextModeChange().Sub(h.clock.Now()).Milliseconds(), saves, orDash(sent), orDash(items),
		gl.SeedIncome(), gl.Treasury(), h.lists(gludio, false), h.lists(gludio, true), h.lists(dion, false), h.lists(dion, true))
}

// lists renders a castle's lists of a period: seeds then crops, as
// id:amount.
func (h *harness) lists(castleID int, next bool) string {
	var b strings.Builder
	for _, s := range h.m.SeedProduction(castleID, next) {
		fmt.Fprintf(&b, " %d:%d", s.ID, s.Amount())
	}
	b.WriteString(" |")
	for _, c := range h.m.CropProcure(castleID, next) {
		fmt.Fprintf(&b, " %d:%d", c.ID, c.Amount())
	}
	return b.String()
}

// TestAutosaveMatchesReferenceProbe replays "autosave default => delay <ms> period
// <ms>": the lists are saved first after delay, then every period.
func TestAutosaveMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	l := manorGolden(t, "autosave")[0]
	var delay, period int64
	if _, err := fmt.Sscanf(l[1], "delay %d period %d", &delay, &period); err != nil {
		t.Fatal(err)
	}
	h := startWith(t, DefaultConfig(), at(6, 30), 0, gludioRows(), 0)
	for i, step := range []int64{delay - 1, 1, period - 1, 1} {
		h.clock.Advance(time.Duration(step) * time.Millisecond)
		if _, _, saves := h.store.state(); saves != (i+1)/2 {
			t.Fatalf("saves after %d steps = %d, want %d", i+1, saves, (i+1)/2)
		}
	}
}

// TestLimitsMatchReferenceProbe replays "limit <seeds>/<crops> x<rate> =>
// <seeds limit> <crops limit>": the manors.xml limits times the crop rate.
func TestLimitsMatchReferenceProbe(t *testing.T) {
	t.Parallel()
	castles, clans := testCastles(t, 0)
	for _, l := range manorGolden(t, "limit") {
		var seeds, crops, rate int
		if _, err := fmt.Sscanf(l[0], "%d/%d x%d", &seeds, &crops, &rate); err != nil {
			t.Fatal(err)
		}
		cfg := DefaultConfig()
		cfg.CropRate = rate
		m := New(cfg, testSeeds(), castles, clans, nil, nil, zerolog.Nop())
		s := manor.Seed{SeedsLimit: seeds, CropsLimit: crops}
		if got := fmt.Sprintf("%d %d", m.SeedsLimit(s), m.CropsLimit(s)); got != l[1] {
			t.Errorf("limits of %s = %s, want %s", l[0], got, l[1])
		}
	}
}

// TestDecreaseAmountMatchesReferenceProbe replays "decrease <amount>
// <n>,... => <taken>:<left> ...".
func TestDecreaseAmountMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	for _, l := range manorGolden(t, "decrease") {
		f := strings.Fields(l[0])
		p := NewProduction(codaSeed, int32(must(strconv.ParseInt(f[0], 10, 32))), 6, 10)
		var got strings.Builder
		for _, v := range strings.Split(f[1], ",") {
			ok := p.DecreaseAmount(int32(must(strconv.ParseInt(v, 10, 32))))
			fmt.Fprintf(&got, " %t:%d", ok, p.Amount())
		}
		if strings.TrimSpace(got.String()) != l[1] {
			t.Errorf("decrease %s =%s, want %s", l[0], got.String(), l[1])
		}
	}
}
