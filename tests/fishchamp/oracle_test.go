package fishchamp

import (
	"context"
	"database/sql"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// The placeholder templates the probe fills in place of the pages, and the
// fisherman's object id.
const (
	winnersTemplate   = "%TABLE%|%prizeItem%|%prizeFirst%|%prizeTwo%|%prizeThree%|%prizeFour%|%prizeFive%|%refresh%|%objectId%"
	runningTemplate   = "%TABLE%|%prizeItem%|%prizeFirst%|%prizeTwo%|%prizeThree%|%prizeFour%|%prizeFive%"
	fishermanObjectID = 1000
)

// endFroms are the week ends the probe computes the following one from.
var endFroms = []string{
	"2026-10-06T19:00:00Z", "2026-10-06T18:59:59.999Z", "2026-10-06T19:00:00.001Z", "2026-10-07T10:30:15.123Z",
	"2026-10-08T00:00:00Z", "2026-10-09T23:59:59Z", "2026-10-10T12:00:00Z", "2026-10-11T00:00:00Z",
	"2026-10-12T23:59:59.999Z", "2026-10-13T00:00:00Z", "2026-12-29T19:00:00Z", "2026-12-31T08:00:00.500Z",
	"2027-02-23T19:00:00Z", "2028-02-28T19:00:00Z",
}

func parseInstant(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// instant formats ms, Unix milliseconds, as the probe prints an instant:
// UTC, milliseconds only when there are some.
func instant(ms int64) string {
	return strings.Replace(time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z"), ".000Z", "Z", 1)
}

// run is one scenario of testdata/scenarios.txt being played.
type run struct {
	t     *testing.T
	out   *[]string
	db    *sql.DB
	loop  *sim.Inline
	champ *fishchamp.Championship
	rolls []int
}

func (r *run) add(s string) { *r.out = append(*r.out, s) }

// roll answers the championship's dice from the step's scripted rolls,
// checking each draw's range as the reference's Rnd.get(min, max) has it.
func (r *run) roll(n int) int {
	if len(r.rolls) == 0 {
		r.t.Fatalf("roll(%d) with no scripted roll left", n)
	}
	v := r.rolls[0]
	r.rolls = r.rolls[1:]
	if v < 0 || v >= n {
		r.t.Fatalf("scripted roll %d outside [0, %d)", v, n)
	}
	return v
}

// boot restores and starts a championship on the scenario's database.
func (r *run) boot() {
	r.champ = fishchamp.New(fishchamp.DefaultConfig(), gamesql.NewFishingChampionshipStore(r.db), nil,
		r.loop.NewQueue("fishchamp"), zerolog.Nop(), fishchamp.WithRoll(r.roll))
	if err := r.champ.Restore(context.Background()); err != nil {
		r.t.Fatalf("restore: %v", err)
	}
	r.champ.Start()
	r.loop.Run()
}

func (r *run) stop() {
	if err := r.champ.Stop(context.Background()); err != nil {
		r.t.Fatalf("stop: %v", err)
	}
}

func (r *run) clock() string {
	return "@" + instant(r.loop.Now().UnixMilli()) + " minutes=" + strconv.FormatInt(r.champ.MinutesLeft(), 10)
}

func (r *run) step(f []string) {
	t := r.t
	switch f[0] {
	case "start":
		r.loop = sim.NewInline(parseInstant(t, f[1]))
	case "memo":
		if _, err := r.db.Exec("INSERT INTO server_memo (var, value) VALUES ('fishChampionshipEnd', ?)", strconv.FormatInt(parseInstant(t, f[1]).UnixMilli(), 10)); err != nil {
			t.Fatal(err)
		}
	case "row":
		if _, err := r.db.Exec("INSERT INTO fishing_championship (player_name, fish_length, rewarded) VALUES (?, ?, ?)", f[1], f[2], f[3]); err != nil {
			t.Fatal(err)
		}
	case "boot":
		r.boot()
		r.add("BOOT " + r.clock())
	case "fish":
		r.add("FISH " + f[1] + " lure=" + f[2])
		for _, s := range f[3:] {
			n, err := strconv.Atoi(s)
			if err != nil {
				t.Fatal(err)
			}
			r.rolls = append(r.rolls, n)
		}
		lure, err := strconv.Atoi(f[2])
		if err != nil {
			t.Fatal(err)
		}
		catch, ok := r.champ.NewFish(f[1], int32(lure))
		if !ok {
			t.Fatal("NewFish measured nothing")
		}
		if len(r.rolls) > 0 {
			t.Fatalf("rolls left: %v", r.rolls)
		}
		r.add("MSG 1847 " + commons.JavaDouble(catch.Length))
		if catch.Registered {
			r.add("MSG 1848")
		}
	case "mid":
		r.add("MID")
		places, refreshing := r.champ.Running()
		if refreshing {
			r.add("HTML " + path.Base(fishchamp.PageRefreshing))
			break
		}
		r.add("HTML " + path.Base(fishchamp.PageRunning) + " " + r.champ.FillRunning(runningTemplate, places, "Adena"))
	case "champ":
		r.add("CHAMP")
		r.add("HTML " + path.Base(fishchamp.PageWinners) + " " + r.champ.FillWinners(winnersTemplate, fishermanObjectID, "Adena"))
	case "reward":
		r.add("REWARD " + f[1])
		if !r.champ.IsWinner(f[1]) {
			r.add("HTML " + path.Base(fishchamp.PageNotWinner))
			break
		}
		for _, count := range r.champ.Claim(f[1]) {
			if count > 0 {
				r.add("ADD " + strconv.Itoa(int(r.champ.Config().RewardItemID)) + " " + strconv.Itoa(int(count)))
				r.add("HTML " + path.Base(fishchamp.PageRewarded))
			}
		}
	case "advance":
		r.loop.Advance(max(parseInstant(t, f[1]).Sub(r.loop.Now()), 0))
		r.add(r.clock())
	case "restart":
		r.stop()
		r.boot()
		r.add("RESTART " + r.clock())
	case "rows":
		r.stop()
		var memo string
		if err := r.db.QueryRow("SELECT value FROM server_memo WHERE var = 'fishChampionshipEnd'").Scan(&memo); err != nil {
			t.Fatal(err)
		}
		r.add("MEMO " + memo)
		rows, err := r.db.Query("SELECT player_name, fish_length, rewarded FROM fishing_championship")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var e fishchamp.Entry
			if err := rows.Scan(&e.Name, &e.Length, &e.Reward); err != nil {
				t.Fatal(err)
			}
			r.add("ROW " + e.Name + " " + commons.JavaDouble(e.Length) + " " + strconv.Itoa(int(e.Reward)))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown step %q", f[0])
	}
}

// TestFishingChampionshipMatchesReferenceProbe computes the week end that
// follows each of a set of instants, then plays testdata/scenarios.txt on
// a virtual clock in UTC against the database, and compares every catch's
// messages, every page, every prize and the stored rows with
// testdata/fishchamp.golden.
//
// The golden file is the output of a probe of the reference championship
// (aCis 409, data/manager/FishingChampionshipManager.java, and
// model/actor/instance/Fisherman.java's FishingReward check), copied with
// System.currentTimeMillis read from a virtual clock, ThreadPool.schedule
// replaced by stepping that clock to the soonest task, Rnd.get by scripted
// rolls, and server_memo and fishing_championship by in-memory rows, the
// length rounded to three decimals as the column stores it. The probe and
// how to regenerate the golden file are in testdata/oracle.
//
// The scenarios cover an empty table booted on a Wednesday (catches that
// join, improve, fail to improve, fall short of a full ranking, take its
// shortest place, on normal and prize-winning lures, under any case of a
// name; the running ranking's one-minute snapshot; the week's end and its
// winners' claims; a restart that rounds a stored length), a restored week
// still running with past winners of every reward state, seven of them, a
// tie, and an unknown state; a restored week already over at boot; empty
// tables booted on a Monday night and on a Tuesday after the end hour,
// where tied catches keep the order they were made in.
func TestFishingChampionshipMatchesReferenceProbe(t *testing.T) {
	want, err := os.ReadFile("testdata/fishchamp.golden")
	if err != nil {
		t.Fatal(err)
	}
	got := []string{"# setEndOfChamp"}
	for _, from := range endFroms {
		at := parseInstant(t, from)
		got = append(got, "end "+from+" -> "+instant(fishchamp.NextEnd(at.UnixMilli(), time.UTC)))
	}

	script, err := os.ReadFile("testdata/scenarios.txt")
	if err != nil {
		t.Fatal(err)
	}
	var scenarios [][]string
	for _, line := range strings.Split(string(script), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "//"):
		case strings.HasPrefix(line, "scenario "):
			scenarios = append(scenarios, []string{line})
		default:
			scenarios[len(scenarios)-1] = append(scenarios[len(scenarios)-1], line)
		}
	}
	for _, sc := range scenarios {
		name := strings.TrimPrefix(sc[0], "scenario ")
		got = append(got, "# "+name)
		t.Run(name, func(t *testing.T) {
			r := &run{t: t, out: &got, db: sqltest.SharedDB(t)}
			for _, line := range sc[1:] {
				r.step(strings.Fields(line))
			}
		})
	}

	if g, w := strings.Join(got, "\n")+"\n", string(want); g != w {
		gl, wl := strings.Split(g, "\n"), strings.Split(w, "\n")
		for i := range min(len(gl), len(wl)) {
			if gl[i] != wl[i] {
				t.Fatalf("line %d = %q, want %q", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("trace has %d lines, want %d", len(gl), len(wl))
	}
}
