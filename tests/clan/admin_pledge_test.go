package clan

import (
	"context"
	"database/sql"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: //pledge (AdminPledge, access level 7) acts on the selected
// player, the game master itself without one. create <name> lifts the
// player's clan creation wait, founds the clan (ClanTable.createClan, its
// answers going to the player), and puts the wait back when the founding
// fails. Every other action needs the player in a clan
// (TARGET_MUST_BE_IN_CLAN, then the game menu): dismiss dissolves it at
// once (ClanTable.destroyClan); info shows the game master GMViewPledgeInfo;
// level <0-8> runs Clan.changeLevel, whose online leader gets the siege
// skills when 3 < level, loses them otherwise, and is told
// CLAN_CAN_ACCUMULATE_CLAN_REPUTATION_POINTS when 4 < level, before every
// member gets PledgeShowInfoUpdate and CLAN_LEVEL_INCREASED; rep<...>
// <points> adds reputation to a clan of level 5 or more, the score and the
// points summing as Java ints; transfer<...> runs Clan.setNewLeader. The
// game master is shown game_menu.htm after every action except a refused
// transfer.

// pledgeWorld is a level-seeded clan the founder leads, the recruit in the
// world beside it outside the clan, and a game master of access level 7
// with the shipped skills, admin tables and game menu.
type pledgeWorld struct {
	*clanWorld
	gm   *testsupport.ScriptedClient
	gmID int32
}

// bootPledgeWorld boots s with the founder as its leader, then runs the
// extra seed statements; the game master selects the recruit.
func bootPledgeWorld(t *testing.T, s clanSeed, extra ...string) *pledgeWorld {
	t.Helper()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	menu, err := os.ReadFile(datapack.Path(t, "data", "html", "admin", "game_menu.htm"))
	if err != nil {
		t.Fatalf("read game menu: %v", err)
	}
	opts := append(clanSkillOptions(t),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedClanRows(t, s, db)
			seedStatements(t, db, extra...)
		}),
		gameservertest.WithAdmin(data),
		gameservertest.WithHTMLPages(map[string]string{"admin/game_menu.htm": string(menu)}))
	w := bootClanWorldCarrying(t, 40, 0, nil, opts...)
	gmID := w.srv.SeedCharacterFor(t, "gm", "Admin", 10, 0).ID
	if _, err := w.srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", gmID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	gm := w.srv.DialClient(t, "gm", 1)
	startInWorld(t, gm)
	pw := &pledgeWorld{clanWorld: w, gm: gm, gmID: gmID}
	pw.selectFor(t, w.memberID)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return pw
}

// selectFor has the game master select objectID.
func (w *pledgeWorld) selectFor(t *testing.T, objectID int32) {
	t.Helper()
	w.gm.Send(encodeAction(objectID, w.at))
	drainFrames(t, w.gm)
}

// pledge sends //pledge args as a typed command and returns the game
// master's answer, then the leader's and the recruit's frames.
func (w *pledgeWorld) pledge(t *testing.T, args string) (gm []string, leader, member [][]byte) {
	t.Helper()
	w.gm.Send(encodeBuildCmd(strings.TrimSpace("pledge " + args)))
	gmFrames := drainFrames(t, w.gm)
	return pledgeAnswer(t, gmFrames), drainFrames(t, w.leader), drainFrames(t, w.member)
}

// encodeBuildCmd is a "//command" typed in chat.
func encodeBuildCmd(command string) []byte {
	pw := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	pw.WriteString(command)
	return pw.Bytes()
}

// gameMenu marks the game menu page in a pledgeAnswer.
const gameMenu = "<game menu>"

// pledgeAnswer decodes the game master's frames: each text it was told,
// "sm<id>" for any other system message, gameMenu for the game menu and
// "gmview <player>" for a clan view. What the players beside it broadcast
// is left out.
func pledgeAnswer(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			id, params := sysMsg(t, f)
			if id == serverpackets.SystemMessageS1 {
				out = append(out, params[0])
			} else {
				out = append(out, "sm"+itoa(int32(id)))
			}
		case serverpackets.OpcodeNpcHtmlMessage:
			r := wire.NewReader(f[1:])
			r.ReadInt32()
			if page := r.ReadString(); !strings.Contains(page, "<title>Game menu</title>") {
				t.Fatalf("game master was shown %q, want the game menu", page)
			}
			out = append(out, gameMenu)
		case serverpackets.OpcodeGMViewPledgeInfo:
			r := wire.NewReader(f[1:])
			out = append(out, "gmview "+r.ReadString())
		}
	}
	return out
}

func wantAnswer(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s answer = %q, want %q", what, got, want)
	}
}

// TestAdminPledgeRefusals answers //pledge without an action with its
// usage, any action but create on a clanless player with
// TARGET_MUST_BE_IN_CLAN, and an unknown action on a member with the game
// menu alone.
func TestAdminPledgeRefusals(t *testing.T) {
	t.Parallel()
	w := bootPledgeWorld(t, clanSeed{level: 5})

	gm, _, _ := w.pledge(t, "")
	wantAnswer(t, "//pledge", gm, "Usage: //pledge create|dismiss|info|level|rep|transfer", gameMenu)
	for _, action := range []string{"info", "dismiss", "level 3", "rep 10", "transfer", "nothing"} {
		gm, _, member := w.pledge(t, action)
		wantAnswer(t, "//pledge "+action+" on a clanless player", gm, "sm"+itoa(serverpackets.SystemMessageTargetMustBeInClan), gameMenu)
		if len(member) != 0 {
			t.Fatalf("//pledge %s sent the clanless player %x", action, opcodes(member))
		}
	}

	w.selectFor(t, w.leaderID)
	gm, _, _ = w.pledge(t, "nothing")
	wantAnswer(t, "//pledge nothing", gm, gameMenu)
}

// TestAdminPledgeInfo shows the game master the leader's clan as the
// character page's link opens it, the clan name it carries not read: the
// header, then the whole roster in the order of the hash table the clan
// keeps it in, where filler 900001 (bucket 12 of 16) comes before filler
// 900000 (bucket 13). Only a sponsor sets a member's last field; an
// apprentice does not.
func TestAdminPledgeInfo(t *testing.T) {
	t.Parallel()
	w := bootPledgeWorld(t, clanSeed{level: 5, reputation: 1234, fillers: 2},
		`UPDATE characters SET sponsor = 900001 WHERE obj_Id = 900000`,
		`UPDATE characters SET apprentice = 900000 WHERE obj_Id = 900001`)
	w.recruit(t)
	w.selectFor(t, w.leaderID)

	w.gm.Send(encodeBypass("admin_pledge info Elsewhere"))
	frames := drainFrames(t, w.gm)
	wantAnswer(t, "//pledge info", pledgeAnswer(t, frames), "gmview Founder", gameMenu)

	view, _ := firstOpcode(frames, serverpackets.OpcodeGMViewPledgeInfo)
	r := wire.NewReader(view[1:])
	if name := r.ReadString(); name != "Founder" {
		t.Fatalf("viewed player = %q", name)
	}
	header := []int32{r.ReadInt32(), r.ReadInt32()}
	names := []string{r.ReadString(), r.ReadString()}
	for range 6 {
		header = append(header, r.ReadInt32()) // crest, level, castle, hall, ladder rank, reputation
	}
	if !slices.Equal(header, []int32{seededClanID, 0, 0, 5, 0, 0, 1, 1234}) || !slices.Equal(names, []string{"Seeded", "Founder"}) {
		t.Fatalf("header = %v %q, want clan %d, level 5, rank 1, reputation 1234, Seeded led by Founder", header, names, seededClanID)
	}
	r.ReadInt32() // dissolving
	r.ReadInt32()
	r.ReadInt32() // ally id
	r.ReadString()
	r.ReadInt32() // ally crest
	r.ReadInt32() // at war
	if n := r.ReadInt32(); n != 4 {
		t.Fatalf("member count = %d, want 4", n)
	}
	type row struct {
		level, online, sponsored int32
	}
	rows := map[string]row{}
	var order []string
	for range 4 {
		name := r.ReadString()
		level := r.ReadInt32()
		r.ReadInt32() // class
		r.ReadInt32() // sex
		r.ReadInt32() // race
		rows[name] = row{level: level, online: r.ReadInt32(), sponsored: r.ReadInt32()}
		order = append(order, name)
	}
	want := map[string]row{
		"Founder": {40, w.leaderID, 0},
		"Recruit": {1, w.memberID, 0},
		"Filler0": {40, 0, 1},
		"Filler1": {40, 0, 0},
	}
	if !maps.Equal(rows, want) {
		t.Fatalf("members = %+v, want %+v", rows, want)
	}
	if slices.Index(order, "Filler1") > slices.Index(order, "Filler0") {
		t.Fatalf("member order = %q, want Filler1 before Filler0", order)
	}
}

// TestAdminPledgeLevel moves the level 5 clan down to 3, then up to 4 and
// 5: the online leader loses its siege skills at 3 and holds them again at
// 4, is told its clan earns reputation only at 5, and every member gets
// the clan's header and the level notice each time; the level is stored.
// A level outside 0-8 or no number is refused.
func TestAdminPledgeLevel(t *testing.T) {
	t.Parallel()
	w := bootPledgeWorld(t, clanSeed{level: 5})
	w.recruit(t)
	w.selectFor(t, w.leaderID)
	wantSiegeSkills(t, w.clanWorld, "level 5 clan's leader", w.leaderID, 1)

	levelNotice := []byte{serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage}
	for _, step := range []struct {
		level     string
		siege     int
		reputable bool
	}{{"3", 0, false}, {"4", 1, false}, {"5", 1, true}} {
		gm, leader, member := w.pledge(t, "level "+step.level)
		wantAnswer(t, "//pledge level "+step.level, gm, "You have set clan Seeded to level "+step.level, gameMenu)
		if got, _ := storedClan(t, w.clanWorld); itoa(int32(got)) != step.level {
			t.Fatalf("stored level = %d, want %s", got, step.level)
		}
		wantSiegeSkills(t, w.clanWorld, "level "+step.level+" clan's leader", w.leaderID, step.siege)

		leaderMessages := messages(t, leader)
		wantLeader := []int{serverpackets.SystemMessageClanLevelIncreased}
		if step.reputable {
			wantLeader = []int{serverpackets.SystemMessageClanCanAccumulateReputation, serverpackets.SystemMessageClanLevelIncreased}
		}
		if !slices.Equal(leaderMessages, wantLeader) {
			t.Fatalf("level %s: leader messages = %v, want %v", step.level, leaderMessages, wantLeader)
		}
		if got := only(leader, serverpackets.OpcodePledgeShowInfoUpdate); len(got) != 1 {
			t.Fatalf("level %s: leader got %d headers, want 1", step.level, len(got))
		}
		if got := only(member, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage); !slices.Equal(got, levelNotice) {
			t.Fatalf("level %s: recruit got %x, want %x", step.level, got, levelNotice)
		}
		if !slices.Equal(messages(t, member), []int{serverpackets.SystemMessageClanLevelIncreased}) {
			t.Fatalf("level %s: recruit messages = %v", step.level, messages(t, member))
		}
	}

	for _, args := range []string{"level 9", "level -1"} {
		gm, _, _ := w.pledge(t, args)
		wantAnswer(t, "//pledge "+args, gm, "This clan level is incorrect. Put a number between 0 and 8.", gameMenu)
	}
	for _, args := range []string{"level", "level x", "level 2147483648"} {
		gm, _, _ := w.pledge(t, args)
		wantAnswer(t, "//pledge "+args, gm, "Invalid number parameter for //pledge setlevel.", gameMenu)
	}
	if got, _ := storedClan(t, w.clanWorld); got != 5 {
		t.Fatalf("stored level after the refusals = %d, want 5", got)
	}
}

// TestAdminPledgeReputation adds and removes reputation, the members being
// told when the score crosses 0; the score and the points add as 32-bit
// integers before the bounds apply, and the removed amount of the lowest
// int stays negative. A clan below level 5 gets none.
func TestAdminPledgeReputation(t *testing.T) {
	t.Parallel()
	w := bootPledgeWorld(t, clanSeed{level: 5, reputation: 100})
	w.recruit(t)
	w.selectFor(t, w.leaderID)

	gm, _, member := w.pledge(t, "rep 500")
	wantAnswer(t, "//pledge rep 500", gm, "You added 500 points to Seeded's reputation. Their current score is: 600", gameMenu)
	if got := only(member, serverpackets.OpcodePledgeShowInfoUpdate); len(got) != 1 {
		t.Fatalf("recruit got %d headers, want 1", len(got))
	}
	if _, rep := storedClan(t, w.clanWorld); rep != 600 {
		t.Fatalf("stored reputation = %d, want 600", rep)
	}

	gm, _, member = w.pledge(t, "reputation -1000")
	wantAnswer(t, "//pledge reputation -1000", gm, "You removed 1000 points from Seeded's reputation. Their current score is: -400", gameMenu)
	if !slices.Contains(messages(t, member), serverpackets.SystemMessageReputationLowClanSkillsDeactivated) {
		t.Fatalf("recruit messages = %v, want the clan skills turned off", messages(t, member))
	}

	gm, _, _ = w.pledge(t, "rep 1000")
	wantAnswer(t, "//pledge rep 1000", gm, "You added 1000 points to Seeded's reputation. Their current score is: 600", gameMenu)
	gm, _, _ = w.pledge(t, "rep 2147483647")
	wantAnswer(t, "//pledge rep 2147483647", gm, "You added 2147483647 points to Seeded's reputation. Their current score is: -100000000", gameMenu)
	gm, _, _ = w.pledge(t, "rep -2147483648")
	wantAnswer(t, "//pledge rep -2147483648", gm, "You removed -2147483648 points from Seeded's reputation. Their current score is: 100000000", gameMenu)
	gm, _, _ = w.pledge(t, "rep 0")
	wantAnswer(t, "//pledge rep 0", gm, "You removed 0 points from Seeded's reputation. Their current score is: 100000000", gameMenu)
	if _, rep := storedClan(t, w.clanWorld); rep != 100000000 {
		t.Fatalf("stored reputation = %d, want 100000000", rep)
	}

	for _, args := range []string{"rep", "rep x"} {
		gm, _, _ := w.pledge(t, args)
		wantAnswer(t, "//pledge "+args, gm, "Invalid number parameter for //pledge rep.", gameMenu)
	}
	w.pledge(t, "level 4")
	gm, _, _ = w.pledge(t, "rep 10")
	wantAnswer(t, "//pledge rep 10 on a level 4 clan", gm, "Only clans of level 5 or above may receive reputation points.", gameMenu)
	gm, _, _ = w.pledge(t, "rep x")
	wantAnswer(t, "//pledge rep x on a level 4 clan", gm, "Invalid number parameter for //pledge rep.", gameMenu)
}

// TestAdminPledgeTransfer makes the recruit the leader: the siege skills
// move with the leadership, every member is told, and the new leader is
// stored. Selecting the leader again is refused without the game menu.
func TestAdminPledgeTransfer(t *testing.T) {
	t.Parallel()
	w := bootPledgeWorld(t, clanSeed{level: 5})
	w.recruit(t)
	w.selectFor(t, w.memberID)

	gm, leader, member := w.pledge(t, "transfer")
	wantAnswer(t, "//pledge transfer", gm, "You set Recruit as the new Clan leader of Seeded.", gameMenu)
	for who, frames := range map[string][][]byte{"former leader": leader, "recruit": member} {
		if got := messages(t, frames); !slices.Contains(got, serverpackets.SystemMessageClanLeaderPrivilegesTransferredToS1) {
			t.Fatalf("%s messages = %v, want the leader change", who, got)
		}
	}
	wantSiegeSkills(t, w.clanWorld, "former leader", w.leaderID, 0)
	wantSiegeSkills(t, w.clanWorld, "new leader", w.memberID, 1)
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w.clanWorld, `SELECT leader_id FROM clan_data WHERE clan_id = ?`, seededClanID); got != int64(w.memberID) {
		t.Fatalf("stored leader = %d, want %d", got, w.memberID)
	}

	gm, _, _ = w.pledge(t, "transferall")
	wantAnswer(t, "//pledge transferall on the leader", gm, "Recruit is already the Clan leader of Seeded.")
}

// TestAdminPledgeDismissAndCreate dissolves the founder's clan at once,
// then founds a new one with the founder despite the wait the dissolution
// put on it; founding again, or with no name, is refused.
func TestAdminPledgeDismissAndCreate(t *testing.T) {
	t.Parallel()
	w := bootPledgeWorld(t, clanSeed{level: 5})
	w.recruit(t)
	w.selectFor(t, w.leaderID)

	gm, leader, member := w.pledge(t, "dismiss")
	wantAnswer(t, "//pledge dismiss", gm, "The clan is now disbanded.", gameMenu)
	for who, frames := range map[string][][]byte{"leader": leader, "recruit": member} {
		if got := messages(t, frames); len(got) == 0 || got[0] != serverpackets.SystemMessageClanHasDispersed {
			t.Fatalf("%s messages = %v, want CLAN_HAS_DISPERSED first", who, got)
		}
	}
	wantSiegeSkills(t, w.clanWorld, "dispersed leader", w.leaderID, 0)
	w.srv.FlushPersistence(t)
	if n := queryInt(t, w.clanWorld, `SELECT COUNT(*) FROM clan_data WHERE clan_id = ?`, seededClanID); n != 0 {
		t.Fatalf("dissolved clan rows = %d, want 0", n)
	}
	if wait := queryInt(t, w.clanWorld, `SELECT clan_create_expiry_time FROM characters WHERE obj_Id = ?`, w.leaderID); wait <= time.Now().UnixMilli() {
		t.Fatalf("dispersed leader's clan creation wait = %d, want one in the future", wait)
	}

	gm, leader, _ = w.pledge(t, "create Phoenix")
	wantAnswer(t, "//pledge create Phoenix", gm, "Clan Phoenix have been created. Clan leader is Founder.", gameMenu)
	if got := only(leader, serverpackets.OpcodePledgeShowMemberListAll, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage); !slices.Equal(got, []byte{serverpackets.OpcodePledgeShowMemberListAll, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("founder got %x, want its roster, UserInfo and CLAN_CREATED", got)
	}
	if got := messages(t, leader); !slices.Equal(got, []int{serverpackets.SystemMessageClanCreated}) {
		t.Fatalf("founder messages = %v, want CLAN_CREATED", got)
	}
	w.srv.FlushPersistence(t)
	if n := queryInt(t, w.clanWorld, `SELECT COUNT(*) FROM clan_data WHERE clan_name = 'Phoenix' AND leader_id = ?`, w.leaderID); n != 1 {
		t.Fatalf("Phoenix rows led by the founder = %d, want 1", n)
	}

	gm, leader, _ = w.pledge(t, "create Another")
	wantAnswer(t, "//pledge create Another", gm, "There was a problem while creating the clan.", gameMenu)
	if got := messages(t, leader); !slices.Equal(got, []int{serverpackets.SystemMessageFailedToCreateClan}) {
		t.Fatalf("founder messages = %v, want FAILED_TO_CREATE_CLAN", got)
	}
	gm, _, _ = w.pledge(t, "create")
	wantAnswer(t, "//pledge create", gm, "Invalid string parameter for //pledge create.", gameMenu)

	// A level 1 player cannot found a clan; the player is told so.
	w.selectFor(t, w.memberID)
	gm, _, member = w.pledge(t, "create Youngsters")
	wantAnswer(t, "//pledge create Youngsters", gm, "There was a problem while creating the clan.", gameMenu)
	if got := messages(t, member); !slices.Equal(got, []int{serverpackets.SystemMessageNotMeetCriteriaToCreateClan}) {
		t.Fatalf("recruit messages = %v, want NOT_MEET_CRITERIA", got)
	}
}
