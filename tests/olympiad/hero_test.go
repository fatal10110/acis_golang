package olympiad

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// heroSkills are the skills hero status grants, each at level 1.
var heroSkills = []int32{395, 396, 1374, 1375, 1376}

// Shipped hero items: the Infinity Blade and Infinity Cleaver hero
// weapons, and the hero circlet.
const (
	infinityBlade   int32 = 6611
	infinityCleaver int32 = 6612
	heroCirclet     int32 = 6842
)

var shippedItems = struct {
	once  sync.Once
	table *item.Table
	err   error
}{}

// heroItemOptions boots the fixture item templates plus the shipped hero
// items.
func heroItemOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	dir := datapack.Path(t, "data", "xml", "items")
	shippedItems.once.Do(func() {
		shippedItems.table, shippedItems.err = gamexml.LoadItemTemplates(dir, zerolog.Nop())
	})
	if shippedItems.err != nil {
		t.Fatalf("load shipped items: %v", shippedItems.err)
	}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{infinityBlade, infinityCleaver, heroCirclet} {
		tmpl, ok := shippedItems.table.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	return []gameservertest.Option{gameservertest.WithItemTemplates(item.NewTable(templates))}
}

// heroRow is one heroes row.
type heroRow struct {
	classID, count int
	played, active bool
}

// heroRows reads every heroes row by character id.
func heroRows(t *testing.T, srv *gameservertest.Server) map[int32]heroRow {
	t.Helper()
	rows, err := srv.DB.QueryContext(context.Background(), "SELECT char_id, class_id, count, played, active FROM heroes")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int32]heroRow{}
	for rows.Next() {
		var (
			id             int32
			r              heroRow
			played, active int
		)
		if err := rows.Scan(&id, &r.classID, &r.count, &played, &active); err != nil {
			t.Fatal(err)
		}
		r.played, r.active = played == 1, active == 1
		out[id] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// itemCount counts each owner's rows of templateID.
func itemCount(t *testing.T, srv *gameservertest.Server, ownerID, templateID int32) int {
	t.Helper()
	var n int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM items WHERE owner_id = ? AND item_id = ?", ownerID, templateID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// systemMessageIDs returns the id of every SystemMessage among frames.
func systemMessageIDs(frames [][]byte) []int {
	var out []int
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			out = append(out, int(wire.NewReader(f[1:]).ReadInt32()))
		}
	}
	return out
}

// lastFrame returns the last of frames with opcode.
func lastFrame(t *testing.T, frames [][]byte, opcode byte) []byte {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == opcode {
			return frames[i]
		}
	}
	t.Fatalf("no frame with opcode %#x among %d", opcode, len(frames))
	return nil
}

// userInfoHero reads UserInfo's hero byte, which follows the noble byte.
func userInfoHero(frame []byte) byte { return frame[len(frame)-35] }

// encodeUseItem asks to use (here, wear) the inventory item objectID.
func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}

// The election's nobles and heroes, besides the character logged in.
const (
	alpha   int32 = 9001 // class 88: 50 points, 10 matches, 3 wins
	bravo   int32 = 9002 // class 88: 50 points, 12 matches, 1 win: more matches, elected
	charlie int32 = 9003 // class 89: 100 points but 4 matches, too few
	delta   int32 = 9004 // class 89: 6 matches but no win
	echo    int32 = 9005 // class 90: elected again; first elected as class 93
	foxtrot int32 = 9006 // inactive hero of the ending era, not elected again
	golf    int32 = 9007 // class 91: tied with hotel, the lower id
	hotel   int32 = 9008
	gm      int32 = 9009 // a game master holding a hero item
	ghost   int32 = 9999 // a class 92 record whose character is gone
)

// electionSeed seeds the nobles' records and heroes the election reads:
// Login is an active hero of the ending era.
func electionSeed() []string {
	stmts := []string{
		`UPDATE characters SET nobless = 1 WHERE char_name = 'Login'`,
		`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, 1, 1 FROM characters WHERE char_name = 'Login'`,
	}
	for id, name := range map[int32]string{alpha: "Alpha", bravo: "Bravo", charlie: "Charlie", delta: "Delta", echo: "Echo", foxtrot: "Foxtrot", golf: "Golf", hotel: "Hotel"} {
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO characters (account_name, obj_Id, char_name, accesslevel) VALUES ('others', %d, '%s', 0)`, id, name))
	}
	stmts = append(stmts,
		fmt.Sprintf(`INSERT INTO characters (account_name, obj_Id, char_name, accesslevel) VALUES ('gm', %d, 'Master', 1)`, gm),
		fmt.Sprintf(`INSERT INTO olympiad_nobles VALUES (%d, 88, 50, 10, 3, 7, 0, 0), (%d, 88, 50, 12, 1, 11, 0, 0),
			(%d, 89, 100, 4, 4, 0, 0, 0), (%d, 89, 10, 6, 0, 6, 0, 0), (%d, 90, 20, 5, 1, 4, 0, 0),
			(%d, 91, 30, 7, 2, 5, 0, 0), (%d, 91, 30, 7, 2, 5, 0, 0), (%d, 92, 500, 20, 20, 0, 0, 0)`,
			alpha, bravo, charlie, delta, echo, golf, hotel, ghost),
		fmt.Sprintf(`INSERT INTO heroes (char_id, class_id, count, played, active) VALUES (%d, 93, 2, 0, 0), (%d, 94, 1, 1, 0)`, echo, foxtrot),
		fmt.Sprintf(`INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES
			(%d, 7001, %d, 1, 'INVENTORY', 0), (%d, 7002, 57, 100, 'INVENTORY', 0), (%d, 7003, %d, 1, 'INVENTORY', 0)`,
			alpha, infinityBlade, alpha, gm, heroCirclet),
	)
	return stmts
}

// TestHeroElection pins the end of an Olympiad's hero election, the
// reference SQL applied to the seeded records by hand: every stored hero
// leaves the running era (played 0, its active flag kept); per third
// class the noble with the most points, then matches, then wins, among
// those with 5 matches and a win whose character exists, is elected
// inactive; a former hero counts one more and keeps the class it was
// first elected with; nobles tied on all three go by character id. Every
// hero item no game master owns is deleted. The ending era's active hero,
// online, loses its status, its hero skills, the hero weapon it wears and
// the hero items it carries.
func TestHeroElection(t *testing.T) {
	t.Parallel()
	opts := append(skillOptions(t), heroItemOptions(t)...)
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			for _, stmt := range electionSeed() {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed %q: %v", stmt, err)
				}
			}
		}),
	}, opts...)...)
	login := srv.SoleObjectID(t)
	blade := srv.GiveItem(t, login, infinityBlade, 1)
	srv.GiveItem(t, login, infinityCleaver, 1)
	c := srv.Client
	burst := startInWorld(t, c)
	if got := userInfoHero(firstFrame(t, burst, serverpackets.OpcodeUserInfo)); got != 1 {
		t.Fatalf("login UserInfo hero byte = %d, want 1", got)
	}
	c.Send(encodeUseItem(blade))
	drainFrames(t, c)

	srv.Olympiad.SelectHeroes()
	srv.Settle(t)
	srv.FlushPersistence(t)
	srv.Settle(t)
	frames := drainFrames(t, c)

	ids := systemMessageIDs(frames)
	for _, want := range []int{serverpackets.SystemMessageOlympiadPeriodS1HasEnded, serverpackets.SystemMessageS1Disarmed, serverpackets.SystemMessageS1Disappeared} {
		if !slices.Contains(ids, want) {
			t.Errorf("system messages = %v, want %d among them", ids, want)
		}
	}
	skills := skillListLevels(t, firstFrame(t, frames, serverpackets.OpcodeSkillList))
	for _, id := range heroSkills {
		if _, ok := skills[id]; ok {
			t.Errorf("SkillList after the election still has hero skill %d", id)
		}
	}
	if got := userInfoHero(lastFrame(t, frames, serverpackets.OpcodeUserInfo)); got != 0 {
		t.Errorf("UserInfo hero byte after the election = %d, want 0", got)
	}

	srv.FlushItems(t)
	srv.FlushPersistence(t)
	for _, tc := range []struct {
		owner, item int32
		want        int
	}{
		{login, infinityBlade, 0}, {login, infinityCleaver, 0}, {alpha, infinityBlade, 0}, {alpha, 57, 1}, {gm, heroCirclet, 1},
	} {
		if got := itemCount(t, srv, tc.owner, tc.item); got != tc.want {
			t.Errorf("owner %d item %d rows = %d, want %d", tc.owner, tc.item, got, tc.want)
		}
	}

	want := map[int32]heroRow{
		login:   {classID: 88, count: 1, played: false, active: true},
		bravo:   {classID: 88, count: 1, played: true},
		echo:    {classID: 93, count: 3, played: true},
		foxtrot: {classID: 94, count: 1},
		golf:    {classID: 91, count: 1, played: true},
	}
	if got := heroRows(t, srv); !maps.Equal(got, want) {
		t.Fatalf("heroes = %+v, want %+v", got, want)
	}
	for id, inactive := range map[int32]bool{bravo: true, echo: true, golf: true, login: false, foxtrot: false, alpha: false} {
		if got := srv.Heroes.IsInactive(id); got != inactive {
			t.Errorf("IsInactive(%d) = %v, want %v", id, got, inactive)
		}
		if srv.Heroes.IsActive(id) {
			t.Errorf("IsActive(%d) after the election, want no active hero", id)
		}
	}
}

// TestHeroElectionWithNobodyToElect pins an election without a single
// eligible noble: every hero leaves the running era, and no hero item is
// deleted.
func TestHeroElectionWithNobodyToElect(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			for _, stmt := range []string{
				`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 2, 1, 1 FROM characters WHERE char_name = 'Login'`,
				`INSERT INTO olympiad_nobles SELECT obj_Id, 88, 50, 4, 4, 0, 0, 0 FROM characters WHERE char_name = 'Login'`,
				`INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) SELECT obj_Id, 7001, 6611, 1, 'WAREHOUSE', 0 FROM characters WHERE char_name = 'Login'`,
			} {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed %q: %v", stmt, err)
				}
			}
		}))
	login := srv.SoleObjectID(t)
	srv.Olympiad.SelectHeroes()
	srv.Settle(t)
	srv.FlushPersistence(t)

	if got, want := heroRows(t, srv), map[int32]heroRow{login: {classID: 88, count: 2, active: true}}; !maps.Equal(got, want) {
		t.Fatalf("heroes = %+v, want %+v", got, want)
	}
	if got := itemCount(t, srv, login, infinityBlade); got != 1 {
		t.Fatalf("hero item rows = %d, want 1 kept", got)
	}
	if srv.Heroes.IsActive(login) || srv.Heroes.IsInactive(login) {
		t.Fatal("the former hero is still a hero of the running era")
	}
}

// TestHeroEntersWorld pins hero status at login: a hero of the running era
// that claimed it shows the hero byte in UserInfo and knows the five hero
// skills at level 1; one that has not claimed it, or a hero of a past era,
// has neither.
func TestHeroEntersWorld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		played, active int
		hero           bool
	}{
		{"active hero", 1, 1, true},
		{"inactive hero", 1, 0, false},
		{"hero of a past era", 0, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stmt := fmt.Sprintf(`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, %d, %d FROM characters WHERE char_name = 'Login'`, tc.played, tc.active)
			_, burst := bootNamed(t, "Login", []string{stmt}, skillOptions(t)...)
			want := byte(0)
			if tc.hero {
				want = 1
			}
			if got := userInfoHero(firstFrame(t, burst, serverpackets.OpcodeUserInfo)); got != want {
				t.Errorf("UserInfo hero byte = %d, want %d", got, want)
			}
			skills := skillListLevels(t, firstFrame(t, burst, serverpackets.OpcodeSkillList))
			for _, id := range heroSkills {
				level, ok := skills[id]
				switch {
				case tc.hero && (!ok || level != 1):
					t.Errorf("SkillList hero skill %d = level %d (known %v), want level 1", id, level, ok)
				case !tc.hero && ok:
					t.Errorf("SkillList of a character who is no hero has hero skill %d", id)
				}
			}
		})
	}
}

// encodeBuildCmd types the admin command text, without its "//".
func encodeBuildCmd(text string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString(text)
	return w.Bytes()
}

// textMessages returns the text of every plain-text SystemMessage among
// frames.
func textMessages(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != serverpackets.SystemMessageS1 || r.ReadInt32() != 1 || r.ReadInt32() != serverpackets.SystemMessageParamText {
			continue
		}
		out = append(out, r.ReadString())
	}
	return out
}

// TestHeroAdminCommands pins //sethero, which toggles the hero status of
// the target, the game master itself without one: the hero skills with its
// skill list, its UserInfo, then the game master's notice; and //endoly,
// which elects the heroes at once and says so.
func TestHeroAdminCommands(t *testing.T) {
	t.Parallel()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	opts := append(skillOptions(t), gameservertest.WithAdmin(data), gameservertest.WithReuseDelays(0, 0))
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Master", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			for _, stmt := range []string{
				`UPDATE characters SET accesslevel = 7 WHERE char_name = 'Master'`,
				`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, 1, 0 FROM characters WHERE char_name = 'Master'`,
			} {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed %q: %v", stmt, err)
				}
			}
		}),
	}, opts...)...)
	master := srv.SoleObjectID(t)
	c := srv.Client
	startInWorld(t, c)

	for _, hero := range []bool{true, false} {
		c.Send(encodeBuildCmd("sethero"))
		frames := drainFrames(t, c)
		order := []byte{}
		for _, f := range frames {
			switch f[0] {
			case serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage:
				order = append(order, f[0])
			}
		}
		if want := []byte{serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage}; string(order) != string(want) {
			t.Fatalf("//sethero answer = %x, want %x", order, want)
		}
		skills := skillListLevels(t, firstFrame(t, frames, serverpackets.OpcodeSkillList))
		if _, ok := skills[heroSkills[0]]; ok != hero {
			t.Errorf("hero=%v: SkillList knows hero skill %d: %v", hero, heroSkills[0], ok)
		}
		want := byte(0)
		if hero {
			want = 1
		}
		if got := userInfoHero(firstFrame(t, frames, serverpackets.OpcodeUserInfo)); got != want {
			t.Errorf("hero=%v: UserInfo hero byte = %d, want %d", hero, got, want)
		}
		if got := textMessages(frames); !slices.Equal(got, []string{"You have modified Master's hero status."}) {
			t.Errorf("hero=%v: notices = %q", hero, got)
		}
	}

	c.Send(encodeBuildCmd("endoly"))
	srv.Settle(t)
	srv.FlushPersistence(t)
	frames := drainFrames(t, c)
	if got := textMessages(frames); !slices.Contains(got, "Heroes have been formed.") {
		t.Fatalf("//endoly notices = %q, want the heroes formed", got)
	}
	if !slices.Contains(systemMessageIDs(frames), serverpackets.SystemMessageOlympiadPeriodS1HasEnded) {
		t.Fatalf("//endoly messages = %v, want the Olympiad period's end", systemMessageIDs(frames))
	}
	if got, want := heroRows(t, srv), map[int32]heroRow{master: {classID: 88, count: 1}}; !maps.Equal(got, want) {
		t.Fatalf("heroes after //endoly = %+v, want %+v", got, want)
	}
}
