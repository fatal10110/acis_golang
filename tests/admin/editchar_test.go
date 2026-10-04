package admin

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// bootEditAdmin boots the GM with the shipped player level table, the
// noble skills' definitions and the character pages, then brings "Player"
// (account player2) into the world next to it, its row seeded by seed.
func bootEditAdmin(t *testing.T, seed ...string) (srv *gameservertest.Server, gm, user *testsupport.ScriptedClient, userID int32) {
	t.Helper()
	levels, err := gamexml.LoadPlayerLevels(datapack.Path(t, "data", "xml", "playerLevels.xml"))
	if err != nil {
		t.Fatalf("load player levels: %v", err)
	}
	var defs []modelskill.Definition
	for _, ref := range modelskill.NobleSkills() {
		defs = append(defs, modelskill.Definition{ID: ref.ID, Level: 1, Name: "Noble " + strconv.Itoa(int(ref.ID))})
	}
	db := sqltest.SharedDB(t)
	srv, _ = bootAdmin(t, adminLevel,
		gameservertest.WithLevels(levels),
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "charinfo.htm", "partyinfo.htm")),
	)
	gm = srv.Client
	enterWorld(t, gm)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	for _, stmt := range seed {
		if _, err := srv.DB.ExecContext(context.Background(), stmt, ch.ID); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	user = srv.DialClient(t, "player2", 1)
	enterWorld(t, user)
	drain(t, gm)
	return srv, gm, user, ch.ID
}

// selectPlayer has gm select the player id, leaving both streams quiet.
func selectPlayer(t *testing.T, gm, user *testsupport.ScriptedClient, id int32) {
	t.Helper()
	exchange(t, gm, encodeAction(id))
	settle(t, user)
}

// storedColumn waits for the characters column of objID to read want.
func storedColumn(t *testing.T, srv *gameservertest.Server, objID int32, column, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		if err := srv.DB.QueryRowContext(context.Background(), "SELECT CAST("+column+" AS CHAR) FROM characters WHERE obj_Id = ?", objID).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", column, err)
		}
		if got == want {
			return
		}
		time.Sleep(accessPollPeriod)
	}
	t.Fatalf("stored %s of %d = %q, want %q", column, objID, got, want)
}

// opcodes returns the opcodes of frames.
func opcodes(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}

// TestAdminSetPlayerFields pins //set color|tcolor|exp|karma|level|noble|
// rec|sp|title on a selected player (AdminEditChar.java admin_set): each
// changes the player, shows it the reference packets and tells the GM, a
// malformed value answers the field's usage, and a selection that is no
// player answers nothing.
func TestAdminSetPlayerFields(t *testing.T) {
	t.Parallel()
	srv, gm, user, userID := bootEditAdmin(t)
	target := onlineCharacter(t, srv, userID)
	selectPlayer(t, gm, user, userID)

	// color: a hexadecimal name color, UserInfo to the player, CharInfo
	// to the players around it.
	sent := exchange(t, gm, encodeBuildCmd("set color FF00"))
	if got := onlyUserInfo(t, settle(t, user)); got.nameColor != 0xFF00 || got.titleColor != userTitleColor {
		t.Fatalf("UserInfo after //set color = %+v, want name color 0xFF00", got)
	}
	charInfos, rest := split(append(sent, settle(t, gm)...))
	if len(charInfos) != 1 {
		t.Fatalf("GM frames = %x, want one CharInfo of Player", testsupport.FrameOpcodes(charInfos))
	}
	assertTexts(t, rest, "You successfully set color name of Player.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set color -FF")), "Usage: //set color <number>")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set color 80000000")), "Usage: //set color <number>")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set color")), "Usage: //set color <number>")
	// Any digit Integer.decode reads is a hexadecimal digit: fullwidth
	// letters included.
	sent = exchange(t, gm, encodeBuildCmd("set color \uFF26\uFF26"))
	assertTexts(t, messages(append(sent, settle(t, gm)...)), "You successfully set color name of Player.")
	settle(t, user)
	if target.NameColor() != 0xFF {
		t.Fatalf("name color = %#x after //set color with fullwidth digits, want 0xff", target.NameColor())
	}

	// tcolor reads its value from the space before it on, which never
	// parses: the reference answers the usage and changes nothing.
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set tcolor FF00")), "Usage: //set tcolor <number>")
	if target.TitleColor() != userTitleColor {
		t.Fatalf("title color = %#x after //set tcolor, want it unchanged", target.TitleColor())
	}

	// karma: the player's karma message, StatusUpdate and UserInfo.
	// The GM around the player gets its new relation.
	_, rest = split(exchange(t, gm, encodeBuildCmd("set karma 50")))
	if got := opcodes(rest); len(got) != 2 || got[0] != serverpackets.OpcodeRelationChanged {
		t.Fatalf("GM frames after //set karma = %x, want RelationChanged, the message", got)
	}
	assertTexts(t, rest[1:], "You successfully set Player's karma to 50.")
	frames := settle(t, user)
	if got := opcodes(frames); !slices.Equal(got[:3], []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeStatusUpdate, serverpackets.OpcodeUserInfo}) {
		t.Fatalf("player frames after //set karma = %x, want SystemMessage, StatusUpdate, UserInfo first", got)
	}
	if target.Karma() != 50 {
		t.Fatalf("karma = %d, want 50", target.Karma())
	}
	settle(t, gm)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set karma -1")), "The karma value must be greater or equal to 0.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set karma x")), "Usage: //set karma <number>")

	// rec: clamped to 255.
	exchange(t, gm, encodeBuildCmd("set rec 300"))
	settle(t, user)
	if got := target.RecommendationsHave(); got != 255 {
		t.Fatalf("recommendations = %d, want 255", got)
	}

	// level: the experience the level starts at; exp and sp as given.
	// The GM around the player sees its level-up animation.
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("set level 20"))), "You successfully set Player's level to 20.")
	settle(t, user)
	if target.Level() != 20 || target.ProgressionValues().Exp != 835854 {
		t.Fatalf("level %d exp %d after //set level 20, want 20 and 835854", target.Level(), target.ProgressionValues().Exp)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set level 82")), "Invalid used level for //set level.")
	settle(t, gm)
	// Long.parseLong reads any Unicode decimal digit: fullwidth "500".
	exchange(t, gm, encodeBuildCmd("set exp \uFF15\uFF10\uFF10"))
	settle(t, user)
	if target.Level() != 3 || target.ProgressionValues().Exp != 500 {
		t.Fatalf("level %d exp %d after //set exp 500, want 3 and 500", target.Level(), target.ProgressionValues().Exp)
	}
	settle(t, gm)
	exchange(t, gm, encodeBuildCmd("set sp 1000"))
	settle(t, user)
	exchange(t, gm, encodeBuildCmd("set sp 10"))
	settle(t, user)
	if sp := target.ProgressionValues().SP; sp != 10 {
		t.Fatalf("sp = %d, want 10", sp)
	}
	settle(t, gm)

	// title: the first word, UserInfo then TitleUpdate to the player,
	// TitleUpdate to the GM around it, stored.
	sent = exchange(t, gm, encodeBuildCmd("set title Hero Of Old"))
	frames = settle(t, user)
	if got := opcodes(frames); !slices.Equal(got, []byte{serverpackets.OpcodeUserInfo, serverpackets.OpcodeTitleUpdate}) {
		t.Fatalf("player frames after //set title = %x, want UserInfo, TitleUpdate", got)
	}
	r := wire.NewReader(frames[1][1:])
	if id, title := r.ReadInt32(), r.ReadString(); id != userID || title != "Hero" {
		t.Fatalf("TitleUpdate = %d %q, want %d Hero", id, title, userID)
	}
	gmFrames := append(sent, settle(t, gm)...)
	if got := opcodes(gmFrames); !slices.Equal(got, []byte{serverpackets.OpcodeTitleUpdate, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("GM frames after //set title = %x, want TitleUpdate, the message", got)
	}
	assertTexts(t, gmFrames[1:], "You successfully set your target's title to Hero.")
	storedColumn(t, srv, userID, "title", "Hero")

	// noble: toggled, the noble skills granted then taken away, SkillList
	// then UserInfo, stored at once.
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set noble")), "You have modified Player's noble status.")
	if got := opcodes(settle(t, user)); !slices.Equal(got, []byte{serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo}) {
		t.Fatalf("player frames after //set noble = %x, want SkillList, UserInfo", got)
	}
	if !target.IsNoble() {
		t.Fatal("//set noble left Player no noble")
	}
	for _, ref := range modelskill.NobleSkills() {
		if target.SkillLevel(int(ref.ID)) != 1 {
			t.Fatalf("noble skill %d level = %d, want 1", ref.ID, target.SkillLevel(int(ref.ID)))
		}
	}
	storedColumn(t, srv, userID, "nobless", "1")
	exchange(t, gm, encodeBuildCmd("set noble"))
	settle(t, user)
	if target.IsNoble() || target.SkillLevel(int(modelskill.NobleSkills()[0].ID)) != 0 {
		t.Fatal("second //set noble left Player noble or its skills")
	}
	storedColumn(t, srv, userID, "nobless", "0")

	// Unknown fields answer the usage.
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set foo")),
		"Usage: //set access|class|color|exp|karma",
		"Usage: //set level|name|rec|sex|sp|tcolor|title")

	// A selection that is no player answers nothing; a title on an NPC
	// is covered by TestAdminSetNPCNameTitle.
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	exchange(t, gm, encodeAction(monster.ObjectID()))
	if frames := exchange(t, gm, encodeBuildCmd("set karma 5")); len(frames) != 0 {
		t.Fatalf("//set karma on a monster frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	// //info on an NPC is not ported yet (#3325): it releases the client.
	if got := opcodes(exchange(t, gm, encodeBuildCmd("info"))); !slices.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("//info on an NPC frames = %x, want ActionFailed", got)
	}
}

// TestAdminRemove pins //remove (AdminEditChar.java admin_remove) on the
// selected player: clan_penalty lifts the named clan penalty and is stored,
// death_penalty lifts the death penalty (DEATH_PENALTY_LIFTED, then
// EtcStatusUpdate), skill_reuse ends every reuse delay and resends
// SkillCoolTime; a missing or unknown kind answers the usage.
func TestAdminRemove(t *testing.T) {
	t.Parallel()
	future := strconv.FormatInt(time.Now().Add(24*time.Hour).UnixMilli(), 10)
	srv, gm, user, userID := bootEditAdmin(t,
		"UPDATE characters SET clan_join_expiry_time = "+future+", clan_create_expiry_time = "+future+", death_penalty_level = 3 WHERE obj_Id = ?")
	target := onlineCharacter(t, srv, userID)
	selectPlayer(t, gm, user, userID)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("remove")), "Usage: //remove <clan_penalty|death_penalty|skill_reuse>")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("remove foo")), "Usage: //remove <clan_penalty|death_penalty|skill_reuse>")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("remove clan_penalty")), "Usage: //remove clan_penalty join|create")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("remove clan_penalty join")), "Clan penalty is successfully removed for Player.")
	if target.ClanJoinExpiryTime() != 0 || target.ClanCreateExpiryTime() == 0 {
		t.Fatalf("clan penalties = %d %d, want the join one lifted only", target.ClanJoinExpiryTime(), target.ClanCreateExpiryTime())
	}
	storedColumn(t, srv, userID, "clan_join_expiry_time", "0")
	storedColumn(t, srv, userID, "clan_create_expiry_time", future)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("remove clan_penalty create")), "Clan penalty is successfully removed for Player.")
	storedColumn(t, srv, userID, "clan_create_expiry_time", "0")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("remove death_penalty")), "Player's Death Penalty has been lifted.")
	frames := settle(t, user)
	if got := opcodes(frames); !slices.Equal(got, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeEtcStatusUpdate}) {
		t.Fatalf("player frames after //remove death_penalty = %x, want SystemMessage, EtcStatusUpdate", got)
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageDeathPenaltyLifted)
	if target.DeathPenaltyLevel() != 0 {
		t.Fatalf("death penalty level = %d, want 0", target.DeathPenaltyLevel())
	}
	// Nothing to lift: the player hears nothing.
	exchange(t, gm, encodeBuildCmd("remove death_penalty"))
	if frames := settle(t, user); len(frames) != 0 {
		t.Fatalf("player frames after a second //remove death_penalty = %x, want none", testsupport.FrameOpcodes(frames))
	}

	target.AddSkillReuse(modelskill.Ref{ID: 1001, Level: 1}, 1001, time.Hour)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("remove skill_reuse")), "Player's skills reuse timers are now cleaned.")
	frames = settle(t, user)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSkillCoolTime {
		t.Fatalf("player frames after //remove skill_reuse = %x, want SkillCoolTime", testsupport.FrameOpcodes(frames))
	}
	if n := wire.NewReader(frames[0][1:]).ReadInt32(); n != 0 {
		t.Fatalf("SkillCoolTime entries = %d, want 0", n)
	}
	if target.SkillDisabled(1001) {
		t.Fatal("skill 1001 still disabled after //remove skill_reuse")
	}
}

// TestAdminCharacterPages pins //debug, //info on a player and
// //party_info (AdminEditChar.java gatherPlayerInfo and admin_party_info):
// the GM selects the player and is shown charinfo.htm filled in, and the
// party page lists the members, the leader highlighted.
func TestAdminCharacterPages(t *testing.T) {
	t.Parallel()
	srv, gm, user, userID := bootEditAdmin(t)
	_ = srv

	frames := exchange(t, gm, encodeBuildCmd("debug Player"))
	if i := slices.Index(opcodes(frames), serverpackets.OpcodeMyTargetSelected); i < 0 {
		t.Fatalf("//debug frames = %x, want Player selected", testsupport.FrameOpcodes(frames))
	}
	pages := only(frames, serverpackets.OpcodeNpcHtmlMessage)
	if len(pages) != 1 {
		t.Fatalf("//debug frames = %x, want one page", testsupport.FrameOpcodes(frames))
	}
	page := htmlBody(t, pages[0])
	for _, want := range []string{
		"Player: Player",
		"<td>" + strconv.Itoa(int(userID)) + "</td>",
		">player2</a>",
		"Human Fighter",
		"IDLE <> IDLE",
		"10, 20, 30, 0",
		">127.0.0.1</a>",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("//debug page lacks %q:\n%s", want, page)
		}
	}
	if strings.Count(page, "<td>N/A</td>") != 2 {
		t.Fatalf("//debug page clan and party = not N/A:\n%s", page)
	}

	// //info on the selected player opens the same page.
	if body := htmlBody(t, only(exchange(t, gm, encodeBuildCmd("info")), serverpackets.OpcodeNpcHtmlMessage)[0]); body != page {
		t.Fatalf("//info page differs from //debug's:\n%s", body)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("party_info Player")), "Player isn't in a party.")

	// Player invites the GM: the party page lists both, Player highlighted.
	user.Send(encodeJoinParty("Admin", 0))
	drain(t, gm)
	gm.Send(encodeAnswerJoinParty(1))
	drain(t, gm)
	drain(t, user)
	frames = exchange(t, gm, encodeBuildCmd("party_info"))
	page = htmlBody(t, only(frames, serverpackets.OpcodeNpcHtmlMessage)[0])
	for _, want := range []string{
		`<a action="bypass -h admin_debug Player"><font color="LEVEL">Player (1)</font></a></td><td width=120 align=right>Human Fighter</td>`,
		`<a action="bypass -h admin_debug Admin">Admin (1)</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("//party_info page lacks %q:\n%s", want, page)
		}
	}
	page = htmlBody(t, only(exchange(t, gm, encodeBuildCmd("debug")), serverpackets.OpcodeNpcHtmlMessage)[0])
	if !strings.Contains(page, `<a action="bypass -h admin_party_info Player">2 members</a>`) {
		t.Fatalf("//debug page lacks the party link:\n%s", page)
	}
}
