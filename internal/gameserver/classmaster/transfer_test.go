package classmaster

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// Class ids the tables below walk: the human fighter line.
const (
	humanFighter = 0
	warrior      = 1
	gladiator    = 2
	knight       = 4
	humanMage    = 10
	duelist      = 88
)

// testJobs is a config offering tier 1 for 100 adena with one reward
// item, and tier 2 for nothing with no reward; tier 3 is not configured.
func testJobs(t *testing.T) map[int]Job {
	t.Helper()
	jobs, err := ParseJobs("1;[57(100)];[20(1)];2;[];[]")
	if err != nil {
		t.Fatalf("ParseJobs: %v", err)
	}
	return jobs
}

// holding is a Talker.ItemCount reading counts from items.
func holding(items map[int32]int) func(int32) int {
	return func(id int32) int { return items[id] }
}

// TestCheckTransfer pins the occupation change check order: the level
// gate (skipped under AllowEntireTree), the class tree, the weight check
// (only when the change hands out items), the configured tier, then the
// item counts.
func TestCheckTransfer(t *testing.T) {
	jobs := testJobs(t)
	rich := map[int32]int{57: 100}
	for _, tc := range []struct {
		name   string
		tree   bool
		talker Talker
		next   int
		want   Refusal
		job    int // configured tier whose job comes back on Accepted
	}{
		{name: "first change at level 20", talker: Talker{ClassID: humanFighter, Level: 20}, next: warrior, want: Accepted, job: 1},
		{name: "other first occupation of the line", talker: Talker{ClassID: humanFighter, Level: 20}, next: knight, want: Accepted, job: 1},
		{name: "below change level", talker: Talker{ClassID: humanFighter, Level: 19}, next: warrior, want: Refused},
		{name: "own class", talker: Talker{ClassID: humanFighter, Level: 20}, next: humanFighter, want: Refused},
		{name: "skip tier without entire tree", talker: Talker{ClassID: humanFighter, Level: 40}, next: gladiator, want: Refused},
		{name: "other line", talker: Talker{ClassID: humanFighter, Level: 20}, next: humanMage, want: Refused},
		{name: "unknown class", talker: Talker{ClassID: humanFighter, Level: 20}, next: -1, want: Refused},
		{name: "short of adena", talker: Talker{ClassID: humanFighter, Level: 20}, next: warrior, want: NotEnoughItems},
		{name: "overweight with reward", talker: Talker{ClassID: humanFighter, Level: 20, WeightPenalty: 3}, next: warrior, want: Overweight},
		{name: "overweight before short items", talker: Talker{ClassID: humanFighter, Level: 20, WeightPenalty: 3, ItemCount: holding(nil)}, next: warrior, want: Overweight},
		{name: "second weight band still changes", talker: Talker{ClassID: humanFighter, Level: 20, WeightPenalty: 2}, next: warrior, want: Accepted, job: 1},
		{name: "overweight without reward", talker: Talker{ClassID: warrior, Level: 40, WeightPenalty: 4}, next: gladiator, want: Accepted, job: 2},
		{name: "unconfigured tier", talker: Talker{ClassID: gladiator, Level: 76}, next: duelist, want: Unconfigured},
		{name: "unconfigured tier overweight", talker: Talker{ClassID: gladiator, Level: 76, WeightPenalty: 3}, next: duelist, want: Unconfigured},
		{name: "entire tree skips level gate", tree: true, talker: Talker{ClassID: humanFighter, Level: 1}, next: warrior, want: Accepted, job: 1},
		{name: "entire tree skip tier pays next tier", tree: true, talker: Talker{ClassID: humanFighter, Level: 1}, next: gladiator, want: Accepted, job: 1},
		{name: "entire tree third occupation", tree: true, talker: Talker{ClassID: humanFighter, Level: 1}, next: duelist, want: Accepted, job: 1},
		{name: "entire tree own class", tree: true, talker: Talker{ClassID: humanFighter, Level: 1}, next: humanFighter, want: Refused},
		{name: "entire tree other line", tree: true, talker: Talker{ClassID: humanFighter, Level: 1}, next: humanMage, want: Refused},
		{name: "entire tree sibling branch", tree: true, talker: Talker{ClassID: warrior, Level: 1}, next: knight, want: Refused},
		{name: "entire tree past configured tiers", tree: true, talker: Talker{ClassID: gladiator, Level: 1}, next: duelist, want: Unconfigured},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewConfig(tc.tree, jobs)
			talker := tc.talker
			if talker.ItemCount == nil {
				items := rich
				if tc.want == NotEnoughItems {
					items = map[int32]int{57: 99}
				}
				talker.ItemCount = holding(items)
			}
			job, got := c.CheckTransfer(talker, tc.next)
			if got != tc.want {
				t.Fatalf("CheckTransfer(%+v, %d) refusal = %d, want %d", tc.talker, tc.next, got, tc.want)
			}
			want := Job{}
			if tc.want == Accepted {
				want = jobs[tc.job]
			}
			if !reflect.DeepEqual(job, want) {
				t.Fatalf("CheckTransfer(%+v, %d) job = %+v, want %+v", tc.talker, tc.next, job, want)
			}
		})
	}
}

// TestParseCommand pins the class manager's commands, matched by prefix,
// and a change_class whose class is missing or does not parse from its
// fourteenth character on.
func TestParseCommand(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    Command
		ok      bool
	}{
		{command: "1stClass", want: Command{Kind: CommandMenu, Tier: 1}, ok: true},
		{command: "2ndClass", want: Command{Kind: CommandMenu, Tier: 2}, ok: true},
		{command: "3rdClassX", want: Command{Kind: CommandMenu, Tier: 3}, ok: true},
		{command: "change_class 88", want: Command{Kind: CommandChangeClass, ClassID: 88}, ok: true},
		{command: "change_class -1", want: Command{Kind: CommandChangeClass, ClassID: -1}, ok: true},
		{command: "change_class", want: Command{Kind: CommandChangeClass, Malformed: true}, ok: true},
		{command: "change_class ", want: Command{Kind: CommandChangeClass, Malformed: true}, ok: true},
		{command: "change_class x", want: Command{Kind: CommandChangeClass, Malformed: true}, ok: true},
		{command: "change_class1", want: Command{Kind: CommandChangeClass, Malformed: true}, ok: true},
		{command: "change_class  1", want: Command{Kind: CommandChangeClass, Malformed: true}, ok: true},
		{command: "become_noble", want: Command{Kind: CommandBecomeNoble}, ok: true},
		{command: "learn_skills now", want: Command{Kind: CommandLearnSkills}, ok: true},
		{command: "Chat 0"},
		{command: "4thClass"},
		{command: ""},
	} {
		got, ok := ParseCommand(tc.command)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseCommand(%q) = %+v, %v; want %+v, %v", tc.command, got, ok, tc.want, tc.ok)
		}
	}
}

// fakePages serves a class manager's pages from a map.
type fakePages map[string]string

func (p fakePages) Get(path string) (string, bool) {
	html, ok := p[path]
	return html, ok
}

// TestMenuPageEntireTree pins the menu a talker below its change level
// sees: under AllowEntireTree it lists every occupation of the asked tier
// in its own line, by class id; otherwise it is told the level to come
// back at.
func TestMenuPageEntireTree(t *testing.T) {
	const npcID, objectID = 50000, int32(7)
	pages := fakePages{
		pagePath(npcID, pageMenu):     "menu %name%|%menu%|%objectId%|%req_items%",
		pagePath(npcID, pageComeBack): "back %level%|%objectId%",
	}
	menu := func(ids ...int) string {
		var b strings.Builder
		for _, id := range ids {
			b.WriteString(`<a action="bypass -h npc_7_change_class ` + strconv.Itoa(id) + `">` + player.ClassName(id) + `</a><br>`)
		}
		return b.String()
	}
	jobs := testJobs(t)
	required := `<tr><td><font color="LEVEL">100</font></td><td>&#57</td></tr>`
	for _, tc := range []struct {
		name string
		tree bool
		tier int
		want string
	}{
		{name: "first tier", tree: true, tier: 1, want: "menu " + player.ClassName(humanFighter) + "|" + menu(1, 4, 7) + "|7|" + required},
		{name: "skip to second tier", tree: true, tier: 2, want: "menu " + player.ClassName(humanFighter) + "|" + menu(2, 3, 5, 6, 8, 9) + "|7|<tr><td>none</td></tr>"},
		{name: "without entire tree", tier: 1, want: "back 20|7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewConfig(tc.tree, jobs).MenuPage(pages, npcID, objectID, humanFighter, 1, tc.tier)
			if got != tc.want {
				t.Fatalf("MenuPage = %q, want %q", got, tc.want)
			}
		})
	}
}
