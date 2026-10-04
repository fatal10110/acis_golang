package npcs

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// monumentID is a Monument of Heroes.
const monumentID = 31690

// heroClaimText is the notice refusing a claim of the hero status off the
// base class or below level 76.
const heroClaimText = "You may only become an hero on a main class whose level is 75 or more."

// heroPages is signsPages with the shipped Monument of Heroes pages.
func heroPages(t *testing.T) map[string]string {
	t.Helper()
	pages := signsPages()
	for _, name := range []string{"hero_main.htm", "hero_main2.htm", "hero_confirm.htm"} {
		raw, err := os.ReadFile(datapack.Path(t, "data", "html", "olympiad", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		pages["olympiad/"+name] = string(raw)
	}
	return pages
}

// seedStatements runs stmts against the database.
func seedStatements(t *testing.T, stmts ...string) func(db *sql.DB) {
	return func(db *sql.DB) {
		for _, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("seed %q: %v", stmt, err)
			}
		}
	}
}

// heroSeed makes Talker a hero of the running era, active or not.
func heroSeed(t *testing.T, active bool) gameservertest.Option {
	flag := 0
	if active {
		flag = 1
	}
	return gameservertest.WithOlympiadSeed(seedStatements(t,
		fmt.Sprintf(`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, 1, %d FROM characters WHERE char_name = 'Talker'`, flag)))
}

// bootMonument enters the world as Talker at level, seeded by extra, and
// spawns a Monument of Heroes next to it.
func bootMonument(t *testing.T, level int, extra ...gameservertest.Option) (*folkWorld, *npc.Folk) {
	t.Helper()
	w := bootFolkWorldAs(t, gameservertest.WithCharacter("Talker", level, 0), heroPages(t), append([]gameservertest.Option{noBypassReuse}, extra...)...)
	return w, w.spawnFolk(t, folkTemplate("OlympiadManagerNpc", monumentID), 40)
}

// TestMonumentOfHeroesPages pins a Monument of Heroes' first page: a hero,
// or one elected and not yet claiming the status, gets hero_main.htm, with
// the claim link only for the latter; anyone else hero_main2.htm. The page
// comes before the closing ActionFailed.
func TestMonumentOfHeroesPages(t *testing.T) {
	t.Parallel()
	claim := `<a action="bypass -h npc_%d_Olympiad 5">"I want to be a Hero."</a><br>`
	for _, tc := range []struct {
		name      string
		extra     []gameservertest.Option
		page      string
		wantClaim bool
	}{
		{"nobody", nil, "hero_main2", false},
		{"elected hero", []gameservertest.Option{heroSeed(t, false)}, "hero_main", true},
		{"hero", []gameservertest.Option{heroSeed(t, true)}, "hero_main", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, monument := bootMonument(t, 76, tc.extra...)
			page := w.talkPage(t, monument, false)
			if strings.Contains(page, "%") {
				t.Fatalf("page = %q, want every placeholder filled", page)
			}
			isMain := strings.Contains(page, "Quest HeroWeapon")
			if isMain != (tc.page == "hero_main") {
				t.Fatalf("page = %q, want %s.htm", page, tc.page)
			}
			if got := strings.Contains(page, fmt.Sprintf(claim, monument.ObjectID())); got != tc.wantClaim {
				t.Fatalf("page = %q, claim link %v, want %v", page, got, tc.wantClaim)
			}
		})
	}
}

// claimFrames keeps the frames a claim of the hero status answers with.
func claimFrames(frames [][]byte) []byte {
	var out []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSkillList, serverpackets.OpcodeSocialAction, serverpackets.OpcodeUserInfo,
			serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed,
			serverpackets.OpcodeNpcHtmlMessage:
			out = append(out, f[0])
		}
	}
	return out
}

// TestHeroClaim pins the claim of the hero status at a Monument of Heroes:
// the confirmation page, then the claim: the hero skills with the skill
// list, the hero's social action (16), UserInfo, and for a clan of level 5
// or more 1000 reputation, with the clan's header and the notice naming
// the hero and the points, before the closing ActionFailed. The claim is
// stored, with its diary entry, and cannot be made twice.
func TestHeroClaim(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 76, heroSeed(t, false), gameservertest.WithClanSeed(seedStatements(t,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) SELECT 600, 'Valor', 5, 100, obj_Id FROM characters WHERE char_name = 'Talker'`,
		`UPDATE characters SET clanid = 600 WHERE char_name = 'Talker'`)))
	w.talkPage(t, monument, false)
	confirm := w.bypass(t, npcCommand(monument, "Olympiad 5"))
	if got := string(opcodes(confirm)); got != string([]byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("Olympiad 5 answer = %x, want the confirmation page then ActionFailed", got)
	}
	if _, page := pageOf(t, confirm); !strings.Contains(page, fmt.Sprintf("npc_%d_Olympiad 6", monument.ObjectID())) {
		t.Fatalf("confirmation page = %q", page)
	}
	// Back to the main page, which still offers the claim.
	if _, page := pageOf(t, w.bypass(t, npcCommand(monument, "Olympiad 7"))); !strings.Contains(page, fmt.Sprintf("npc_%d_Olympiad 5", monument.ObjectID())) {
		t.Fatalf("main page = %q, want the claim link", page)
	}
	w.bypass(t, npcCommand(monument, "Olympiad 5"))

	frames := w.bypass(t, npcCommand(monument, "Olympiad 6"))
	want := []byte{
		serverpackets.OpcodeSkillList, serverpackets.OpcodeSocialAction, serverpackets.OpcodeUserInfo,
		serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeActionFailed,
	}
	if got := claimFrames(frames); string(got) != string(want) {
		t.Fatalf("claim answer = %x, want %x", got, want)
	}
	social, _ := firstOpcode(frames, serverpackets.OpcodeSocialAction)
	if r := wire.NewReader(social[1:]); r.ReadInt32() != w.player || r.ReadInt32() != 16 {
		t.Fatalf("SocialAction = %x, want the hero's action 16", social)
	}
	if info := frames[lastIndex(frames, serverpackets.OpcodeUserInfo)]; info[len(info)-35] != 1 {
		t.Fatal("UserInfo after the claim shows no hero")
	}
	notice := frames[lastIndex(frames, serverpackets.OpcodeSystemMessage)]
	r := wire.NewReader(notice[1:])
	if id, n := r.ReadInt32(), r.ReadInt32(); id != serverpackets.SystemMessageClanMemberS1BecameHeroAndGainedS2ReputationPoints || n != 2 {
		t.Fatalf("notice = %x, want 1776 with two parameters", notice)
	}
	if typ, name := r.ReadInt32(), r.ReadString(); typ != serverpackets.SystemMessageParamText || name != "Talker" {
		t.Fatalf("notice hero = %d %q, want Talker", typ, name)
	}
	if typ, points := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber || points != 1000 {
		t.Fatalf("notice points = %d %d, want 1000", typ, points)
	}
	if !w.srv.Heroes.IsActive(w.player) {
		t.Fatal("the claim left the hero inactive")
	}

	again := w.bypass(t, npcCommand(monument, "Olympiad 6"))
	if got := claimFrames(again); strings.ContainsAny(string(got), string([]byte{serverpackets.OpcodeSocialAction, serverpackets.OpcodeSystemMessage})) {
		t.Fatalf("second claim answer = %x, want no claim", got)
	}

	w.srv.FlushPersistence(t)
	ctx := context.Background()
	var active, reputation, diary int
	if err := w.srv.DB.QueryRowContext(ctx, `SELECT active FROM heroes WHERE char_id = ?`, w.player).Scan(&active); err != nil || active != 1 {
		t.Fatalf("heroes.active = %d, %v; want 1", active, err)
	}
	if err := w.srv.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM heroes_diary WHERE char_id = ? AND action = 2 AND param = 0`, w.player).Scan(&diary); err != nil || diary != 1 {
		t.Fatalf("hero gained diary entries = %d, %v; want 1", diary, err)
	}
	if err := w.srv.DB.QueryRowContext(ctx, `SELECT reputation_score FROM clan_data WHERE clan_id = 600`).Scan(&reputation); err != nil || reputation != 1100 {
		t.Fatalf("clan reputation = %d, %v; want 1100", reputation, err)
	}
}

// TestHeroClaimBelowLevel76 pins the claim of an elected hero below level
// 76: it is told why, then released, and stays elected.
func TestHeroClaimBelowLevel76(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 75, heroSeed(t, false))
	w.talkPage(t, monument, false)
	w.bypass(t, npcCommand(monument, "Olympiad 5"))
	frames := w.bypass(t, npcCommand(monument, "Olympiad 6"))
	if got := string(opcodes(frames)); got != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("claim answer = %x, want the notice then ActionFailed", got)
	}
	if got := systemMessageText(t, frames[0]); got != heroClaimText {
		t.Fatalf("notice = %q, want %q", got, heroClaimText)
	}
	if w.srv.Heroes.IsActive(w.player) || !w.srv.Heroes.IsInactive(w.player) {
		t.Fatal("a refused claim changed the hero status")
	}
}

// TestHeroList pins "Olympiad 4": ExHeroList with every hero of the running
// era, by class: name, class, clan name and crest, alliance name and
// crest, and how many times it was elected.
func TestHeroList(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 76,
		gameservertest.WithClanSeed(seedStatements(t,
			`INSERT INTO characters (account_name, obj_Id, char_name, clanid) VALUES ('others', 9100, 'Other', 600)`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, crest_id) VALUES (600, 'Valor', 3, 9100, 0)`)),
		gameservertest.WithOlympiadSeed(seedStatements(t,
			`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 90, 2, 1, 0 FROM characters WHERE char_name = 'Talker'`,
			`INSERT INTO heroes (char_id, class_id, count, played, active) VALUES (9100, 88, 1, 1, 1), (9101, 89, 4, 0, 1)`)))
	w.talkPage(t, monument, false)
	frames := w.bypass(t, npcCommand(monument, "Olympiad 4"))
	if got := string(opcodes(frames)); got != string([]byte{serverpackets.OpcodeExtended, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("Olympiad 4 answer = %x, want ExHeroList then ActionFailed", got)
	}
	r := wire.NewReader(frames[0][1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExHeroList {
		t.Fatalf("sub-opcode = %#x, want ExHeroList", sub)
	}
	type entry struct {
		name             string
		class            int32
		clan             string
		clanCrest        int32
		ally             string
		allyCrest, count int32
	}
	var got []entry
	for range int(r.ReadInt32()) {
		got = append(got, entry{r.ReadString(), r.ReadInt32(), r.ReadString(), r.ReadInt32(), r.ReadString(), r.ReadInt32(), r.ReadInt32()})
	}
	want := []entry{{"Other", 88, "Valor", 0, "", 0, 1}, {"Talker", 90, "", 0, "", 0, 2}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("ExHeroList = %+v, want %+v", got, want)
	}
}
