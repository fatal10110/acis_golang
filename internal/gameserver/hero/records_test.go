package hero

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// recordStore serves the heroes, diaries, fights and messages a test seeds.
type recordStore struct {
	Store
	mu       sync.Mutex
	rows     []Row
	diaries  map[int32][]DiaryRow
	fights   map[int32][]FightRow
	messages map[int32]string
	before   []int64
	saved    map[int32]string
}

func (s *recordStore) LoadHeroes(context.Context) ([]Row, error) { return s.rows, nil }

func (s *recordStore) LoadDiary(_ context.Context, id int32) ([]DiaryRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.diaries[id], nil
}

func (s *recordStore) LoadFights(_ context.Context, id int32, before int64) ([]FightRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.before = append(s.before, before)
	return s.fights[id], nil
}

func (s *recordStore) LoadMessage(_ context.Context, id int32) (string, bool, error) {
	msg, ok := s.messages[id]
	return msg, ok, nil
}

func (s *recordStore) SaveMessages(_ context.Context, messages map[int32]string) error {
	s.saved = messages
	return nil
}

func (s *recordStore) AddDiaryEntry(_ context.Context, id int32, at int64, action, param int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diaries[id] = append(s.diaries[id], DiaryRow{At: at, Action: action, Param: param})
	return nil
}

func (s *recordStore) ResetPlayed(context.Context) error { return nil }

func (s *recordStore) BestNoble(context.Context, int, int) (int32, string, bool, error) {
	return 0, "", false, nil
}

func (s *recordStore) SaveHeroes(context.Context, map[int32]Hero) error { return nil }

// fixedNames names npc 25001 and castle 1.
type fixedNames struct{}

func (fixedNames) NpcName(id int) (string, bool) { return "Shadith", id == 25001 }

func (fixedNames) CastleName(id int) (string, bool) { return "Gludio", id == 1 }

// recordsNow is the managers' clock: 2026-10-05 14:30:17 UTC.
var recordsNow = time.Date(2026, time.October, 5, 14, 30, 17, 0, time.UTC)

// ms is a UTC instant in Unix milliseconds.
func ms(year int, month time.Month, day, hour, minute, second int) int64 {
	return time.Date(year, month, day, hour, minute, second, 0, time.UTC).UnixMilli()
}

// diaryTemplate keeps the reference page's placeholders, one per line.
const diaryTemplate = "N:%heroname%\nM:%message%\nL:%list%\nP:%buttprev%\nX:%buttnext%"

// diaryLine is one diary entry as the reference page writes it.
func diaryLine(shaded bool, date, action string) string {
	table := "<table width=270>"
	if shaded {
		table = `<table width=270 bgcolor="131210">`
	}
	return "<tr><td>" + table + `<tr><td width=270><font color="LEVEL">` + date + ":xx</font></td></tr><tr><td width=270>" + action +
		"</td></tr><tr><td>&nbsp;</td></tr></table></td></tr>"
}

func newRecordsManager(t *testing.T, store *recordStore) *Manager {
	t.Helper()
	m := New(store, nil, fixedNames{}, nil, 5, func() time.Time { return recordsNow }, zerolog.Nop())
	if err := m.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestDiaryPageShowsEveryHeroesEntries pins the reference's shared diary:
// the running era's heroes' entries, read at restore hero by hero, all go
// to one list, so any hero's page shows every entry, newest first. A past
// hero's entries are not read, and a raid boss or castle the server does
// not know leaves an entry whose page is never sent.
func TestDiaryPageShowsEveryHeroesEntries(t *testing.T) {
	t.Parallel()
	store := &recordStore{
		rows: []Row{
			{ObjectID: 1, Name: "One", ClassID: 88, Played: true, Active: true},
			{ObjectID: 2, Name: "Two", ClassID: 89, Played: true},
			{ObjectID: 3, Name: "Three", ClassID: 90},
		},
		diaries: map[int32][]DiaryRow{
			1: {{At: ms(2026, 9, 1, 10, 5, 0), Action: DiaryHeroGained}, {At: ms(2026, 9, 2, 21, 0, 0), Action: DiaryRaidKilled, Param: 25001}},
			2: {{At: ms(2026, 9, 3, 8, 0, 0), Action: DiaryCastleTaken, Param: 1}},
			3: {{At: ms(2026, 9, 4, 8, 0, 0), Action: DiaryHeroGained}},
		},
		messages: map[int32]string{1: "Glory", 2: ""},
	}
	m := newRecordsManager(t, store)

	want := "N:Two\nM:\nL:" +
		diaryLine(true, "2026-09-03 08", "Gludio Castle was successfuly taken") +
		diaryLine(false, "2026-09-02 21", "Shadith was defeated") +
		diaryLine(true, "2026-09-01 10", "Gained Hero status") + "\nP:\nX:"
	if got, ok := m.DiaryPage(diaryTemplate, "Two", 89, 2, 1); !ok || got != want {
		t.Fatalf("hero 2 diary = %q, %v\nwant %q", got, ok, want)
	}
	if got, ok := m.DiaryPage(diaryTemplate, "One", 88, 1, 1); !ok || !strings.HasPrefix(got, "N:One\nM:Glory\nL:"+diaryLine(true, "2026-09-03 08", "Gludio Castle was successfuly taken")) {
		t.Fatalf("hero 1 diary = %q, %v; want the same entries under its message", got, ok)
	}
	if _, ok := m.DiaryPage(diaryTemplate, "Three", 90, 3, 1); ok {
		t.Fatal("a past hero's diary page was sent")
	}

	store.diaries = map[int32][]DiaryRow{4: {{At: ms(2026, 9, 5, 8, 0, 0), Action: DiaryRaidKilled, Param: 99999}}}
	store.rows = []Row{{ObjectID: 4, Name: "Four", ClassID: 91, Played: true}}
	store.messages = map[int32]string{4: ""}
	unknown := newRecordsManager(t, store)
	if got, ok := unknown.DiaryPage(diaryTemplate, "Four", 91, 4, 1); ok {
		t.Fatalf("a page showing an unknown boss was sent: %q", got)
	}
}

// TestDiaryPagePaging pins the diary's paging: ten entries a page, Prev
// to the next, older page while entries remain past the page's last, Next
// to the previous page from page 2 on. A page past the end shows nothing
// but still offers Prev, and a page before the first is never sent.
func TestDiaryPagePaging(t *testing.T) {
	t.Parallel()
	var rows []DiaryRow
	for day := 1; day <= 12; day++ {
		rows = append(rows, DiaryRow{At: ms(2026, 9, day, 12, 0, 0), Action: DiaryHeroGained})
	}
	store := &recordStore{
		rows:     []Row{{ObjectID: 1, Name: "One", ClassID: 88, Played: true, Active: true}},
		diaries:  map[int32][]DiaryRow{1: rows},
		messages: map[int32]string{1: "m"},
	}
	m := newRecordsManager(t, store)
	prev := func(page int) string {
		return `<button value="Prev" action="bypass _diary?class=88&page=` + string(rune('0'+page)) + `" width=60 height=25 back="L2UI_ct1.button_df" fore="L2UI_ct1.button_df">`
	}
	next := func(page int) string {
		return `<button value="Next" action="bypass _diary?class=88&page=` + string(rune('0'+page)) + `" width=60 height=25 back="L2UI_ct1.button_df" fore="L2UI_ct1.button_df">`
	}

	var first strings.Builder
	for i, day := 0, 12; day >= 3; i, day = i+1, day-1 {
		first.WriteString(diaryLine(i%2 == 0, "2026-09-"+twoDigits(day)+" 12", "Gained Hero status"))
	}
	if got, ok := m.DiaryPage(diaryTemplate, "One", 88, 1, 1); !ok || got != "N:One\nM:m\nL:"+first.String()+"\nP:"+prev(2)+"\nX:" {
		t.Fatalf("page 1 = %q, %v", got, ok)
	}
	second := diaryLine(true, "2026-09-02 12", "Gained Hero status") + diaryLine(false, "2026-09-01 12", "Gained Hero status")
	if got, ok := m.DiaryPage(diaryTemplate, "One", 88, 1, 2); !ok || got != "N:One\nM:m\nL:"+second+"\nP:\nX:"+next(1) {
		t.Fatalf("page 2 = %q, %v", got, ok)
	}
	if got, ok := m.DiaryPage(diaryTemplate, "One", 88, 1, 3); !ok || got != "N:One\nM:m\nL:\nP:"+prev(4)+"\nX:"+next(2) {
		t.Fatalf("page 3 = %q, %v", got, ok)
	}
	if got, ok := m.DiaryPage(diaryTemplate, "One", 88, 1, 0); ok {
		t.Fatalf("page 0 was sent: %q", got)
	}
}

func twoDigits(n int) string { return string(rune('0'+n/10)) + string(rune('0'+n%10)) }

// TestFightsPage pins the fight history page: the fights a running era's
// hero fought before this month, from its side, with the opponent's
// current name and class, the time as mm:ss and the result colored; a
// fight against a character that is gone is left out; the score counts
// the hero's own fights. The month starts at the current second of its
// first midnight.
func TestFightsPage(t *testing.T) {
	t.Parallel()
	store := &recordStore{
		rows:     []Row{{ObjectID: 1, Name: "One", ClassID: 88, Played: true, Active: true}},
		diaries:  map[int32][]DiaryRow{},
		messages: map[int32]string{1: ""},
		fights: map[int32][]FightRow{1: {
			{OneID: 1, TwoID: 7, OneClass: 88, TwoClass: 90, OneName: "One", TwoName: "Rival", OneFound: true, TwoFound: true, Winner: 1, Start: ms(2026, 9, 10, 18, 4, 0), Time: 125_999, Classed: 1},
			{OneID: 8, TwoID: 1, OneClass: 91, TwoClass: 88, OneName: "Foe", TwoName: "One", OneFound: true, TwoFound: true, Winner: 1, Start: ms(2026, 9, 11, 7, 30, 0), Time: 3_725_000},
			{OneID: 1, TwoID: 9, OneClass: 88, TwoClass: 94, OneName: "One", OneFound: true, Winner: 2, Start: ms(2026, 9, 12, 7, 30, 0), Time: 1000},
			{OneID: 1, TwoID: 7, OneClass: 88, TwoClass: 90, OneName: "One", TwoName: "Rival", OneFound: true, TwoFound: true, Winner: 0, Start: ms(2026, 9, 13, 9, 0, 0), Time: 180_000, Classed: 1},
		}},
	}
	m := newRecordsManager(t, store)
	if want := ms(2026, 10, 1, 0, 0, 17); len(store.before) != 1 || store.before[0] != want {
		t.Fatalf("fights read before %v, want %d", store.before, want)
	}

	const template = "N:%heroname%\nS:%win% %draw% %loos%\nL:%list%\nP:%buttprev%\nX:%buttnext%"
	want := "N:One\nS:1 1 1\nL:" +
		`<tr><td><table width=270 bgcolor="131210">2026-09-10 18:04</font>&nbsp;&nbsp;<font color="00ff00">victory</font></td><td width=50 align=right><font color="FFFF99">cls</font></td></tr><tr><td width=220>vs Rival (Phoenix Knight)</td><td width=50 align=right>(02:05)</td></tr><tr><td colspan=2>&nbsp;</td></tr></table></td></tr>` +
		`<tr><td><table width=270><tr><td width=220><font color="LEVEL">2026-09-11 07:30</font>&nbsp;&nbsp;<font color="ff0000">loss</font></td><td width=50 align=right><font color="999999">non-cls<font></td></tr><tr><td width=220>vs Foe (Hell Knight)</td><td width=50 align=right>(02:05)</td></tr><tr><td colspan=2>&nbsp;</td></tr></table></td></tr>` +
		`<tr><td><table width=270 bgcolor="131210">2026-09-13 09:00</font>&nbsp;&nbsp;<font color="ffff00">draw</font></td><td width=50 align=right><font color="FFFF99">cls</font></td></tr><tr><td width=220>vs Rival (Phoenix Knight)</td><td width=50 align=right>(03:00)</td></tr><tr><td colspan=2>&nbsp;</td></tr></table></td></tr>` +
		"\nP:\nX:"
	if got, ok := m.FightsPage(template, "One", 88, 1, 1); !ok || got != want {
		t.Fatalf("fights page = %q, %v\nwant %q", got, ok, want)
	}
	if _, ok := m.FightsPage(template, "Two", 89, 2, 1); ok {
		t.Fatal("a page was sent for a hero without a fight history")
	}

	store.fights = map[int32][]FightRow{1: {{OneID: 1, TwoID: 7, TwoName: "Rival", TwoFound: true, Winner: 3}}}
	odd := newRecordsManager(t, store)
	if got, ok := odd.FightsPage(template, "One", 88, 1, 1); ok {
		t.Fatalf("a page showing a fight without a result was sent: %q", got)
	}
}

// TestElectionAndClaimReloadRecords pins the records across an era: the
// election forgets which heroes have pages and their messages but keeps
// the shared lists, so a new hero's claim, read back after its claim's
// diary entry is stored, shows the old era's entries too, under an empty
// message. A raid boss kill joins the list once stored only for a hero
// with a diary page.
func TestElectionAndClaimReloadRecords(t *testing.T) {
	t.Parallel()
	store := &recordStore{
		rows:     []Row{{ObjectID: 1, Name: "One", ClassID: 88, Played: true, Active: true}},
		diaries:  map[int32][]DiaryRow{1: {{At: ms(2026, 9, 1, 10, 0, 0), Action: DiaryHeroGained}}},
		messages: map[int32]string{1: "old"},
	}
	m := newRecordsManager(t, store)
	m.Elect(context.Background())
	if _, ok := m.DiaryPage(diaryTemplate, "One", 88, 1, 1); ok {
		t.Fatal("the outgoing hero's diary page is still sent")
	}
	m.Shutdown()
	if len(store.saved) != 0 {
		t.Fatalf("messages saved after the election = %v, want none", store.saved)
	}

	m.mu.Lock()
	m.heroes = map[int32]Hero{2: {Name: "Two", ClassID: 89, Count: 1, Played: true}}
	m.mu.Unlock()
	if _, ok := m.Activate(2); !ok {
		t.Fatal("Activate(2) refused")
	}
	m.AddDiaryEntry(2, DiaryRaidKilled, 25001)
	m.AddDiaryEntry(1, DiaryRaidKilled, 25001)
	want := "N:Two\nM:\nL:" +
		diaryLine(true, "2026-10-05 14", "Shadith was defeated") +
		diaryLine(false, "2026-10-05 14", "Gained Hero status") +
		diaryLine(true, "2026-09-01 10", "Gained Hero status") + "\nP:\nX:"
	if got, ok := m.DiaryPage(diaryTemplate, "Two", 89, 2, 1); !ok || got != want {
		t.Fatalf("new hero's diary = %q, %v\nwant %q", got, ok, want)
	}

	m.SetMessage(2, "new words")
	m.Shutdown()
	if got := store.saved; len(got) != 1 || got[2] != "new words" {
		t.Fatalf("saved messages = %v, want hero 2's", got)
	}
}

// TestHeroByClass pins the hero a page names: the running era's hero of
// the class, the lowest object id among several.
func TestHeroByClass(t *testing.T) {
	t.Parallel()
	m := New(&recordStore{}, nil, nil, nil, 5, time.Now, zerolog.Nop())
	m.heroes = map[int32]Hero{9: {ClassID: 93}, 4: {ClassID: 93}, 5: {ClassID: 88}}
	if id, ok := m.HeroByClass(93); !ok || id != 4 {
		t.Fatalf("HeroByClass(93) = %d, %v; want 4", id, ok)
	}
	if _, ok := m.HeroByClass(90); ok {
		t.Fatal("HeroByClass(90) found a hero")
	}
}

// TestReplacePlaceholder pins the reference's page fill of a value: '$'
// is kept as it is, a backslash takes the next character as it is, "$0"
// after a backslash stands for the placeholder, and a trailing backslash
// or any other group sends no page.
func TestReplacePlaceholder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value, want string
		ok          bool
	}{
		{"plain words", "<plain words|plain words>", true},
		{"cost $5", "<cost $5|cost $5>", true},
		{`a\b\\c`, `<ab\c|ab\c>`, true},
		{`\$0 and \$00x`, `<\%m% and \%m%x|\%m% and \%m%x>`, true},
		{`end\`, "", false},
		{`\$1`, "", false},
		{`\$`, "", false},
		{`\${name}`, "", false},
	} {
		got, ok := replacePlaceholder("<%m%|%m%>", "%m%", tc.value)
		if ok != tc.ok || got != tc.want {
			t.Errorf("replacePlaceholder(%q) = %q, %v; want %q, %v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}
