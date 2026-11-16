package clan

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// titleClanID is the clan bootSeededClan seeds for the founder.
const titleClanID int32 = 0x7f000031

// bootSeededClan boots the founder leading a stored clan of the given
// level owning castle (0 for none), and the recruit, a noble when noble
// is set, both in the world; the recruit is not in the clan yet.
func bootSeededClan(t *testing.T, level, castle int, noble bool, extra ...gameservertest.Option) *clanWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		knightsSeed(t, level, castle),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Recruit", 1, 0).ID
	if noble {
		if _, err := srv.DB.ExecContext(context.Background(), `UPDATE characters SET nobless = 1 WHERE obj_Id = ?`, w.memberID); err != nil {
			t.Fatalf("make the recruit noble: %v", err)
		}
	}
	w.member = srv.DialClient(t, "player2", 1)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return w
}

// knightsSeed seeds Knights, a clan of the given level owning castle (0
// for none) led by the founder, then runs extra statements. A later
// knightsSeed among a boot's options replaces an earlier one.
func knightsSeed(t *testing.T, level, castle int, extra ...string) gameservertest.Option {
	t.Helper()
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		seedStatements(t, db, append([]string{
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
				SELECT ` + itoa(titleClanID) + `, 'Knights', ` + itoa(int32(level)) + `, ` + itoa(int32(castle)) + `, obj_Id FROM characters WHERE char_name = 'Founder'`,
			`UPDATE characters SET clanid = ` + itoa(titleClanID) + `, power_grade = 0 WHERE char_name = 'Founder'`,
		}, extra...)...)
	})
}

func encodeRequestGiveNickName(name, title string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGiveNickName)
	w.WriteString(name)
	w.WriteString(title)
	return w.Bytes()
}

// userInfoTitle decodes the title a UserInfo frame carries.
func userInfoTitle(t *testing.T, frame []byte) string {
	t.Helper()
	if frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64() // exp
	// 6 attributes, 4 HP/MP values, sp, weight, weight limit, bonus slots,
	// the paperdoll's object and item ids.
	for range 6 + 4 + 4 + 2*17 {
		r.ReadInt32()
	}
	for range 14 + 2 + 12 + 2 + 4 { // augmentation shorts and the two ids
		r.ReadUint16()
	}
	for range 12 + 8 { // combat stats, speeds
		r.ReadInt32()
	}
	for range 4 { // speed and attack multipliers, collision
		r.ReadFloat64()
	}
	for range 4 { // hair style, hair color, face, gm
		r.ReadInt32()
	}
	title := r.ReadString()
	if r.Err() != nil {
		t.Fatalf("decode UserInfo: %v", r.Err())
	}
	return title
}

// titleUpdate decodes the TitleUpdate among frames: the titled object and
// its title.
func titleUpdate(t *testing.T, frames [][]byte) (int32, string) {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodeTitleUpdate)
	if !ok {
		t.Fatalf("no TitleUpdate among %x", opcodes(frames))
	}
	r := wire.NewReader(frame[1:])
	return r.ReadInt32(), r.ReadString()
}

// storedTitle reads objectID's stored title once every queued write landed.
func storedTitle(t *testing.T, w *clanWorld, objectID int32) string {
	t.Helper()
	w.srv.FlushPersistence(t)
	var title string
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT title FROM characters WHERE obj_Id = ?`, objectID).Scan(&title); err != nil {
		t.Fatalf("read title of %d: %v", objectID, err)
	}
	return title
}

// wantTitled checks the titled player's own answer: TITLE_CHANGED, then
// its UserInfo and the TitleUpdate over its head, both carrying title.
func wantTitled(t *testing.T, who string, frames [][]byte, objectID int32, title string) {
	t.Helper()
	want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo, serverpackets.OpcodeTitleUpdate}
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("%s: answer = %x, want SystemMessage, UserInfo, TitleUpdate", who, got)
	}
	if id, _ := sysMsg(t, frames[0]); id != serverpackets.SystemMessageTitleChanged {
		t.Fatalf("%s: message = %d, want TITLE_CHANGED", who, id)
	}
	if got := userInfoTitle(t, frames[1]); got != title {
		t.Fatalf("%s: UserInfo title = %q, want %q", who, got, title)
	}
	if id, got := titleUpdate(t, frames); id != objectID || got != title {
		t.Fatalf("%s: TitleUpdate = %d %q, want %d %q", who, id, got, objectID, title)
	}
}

// TestGiveNickNameNobleTitlesItself pins RequestGiveNickName's noble
// branch: a noble naming itself takes the title, clan or none, cut to 16
// characters; the players around it see it, and it is stored. The title
// is checked first: a character outside the allowed set refuses even a
// noble. A noble naming someone else, and a player that is no noble,
// fall to the clan branch, refused outside a clan.
func TestGiveNickNameNobleTitlesItself(t *testing.T) {
	t.Parallel()
	w := bootSeededClan(t, 0, 0, true)

	w.member.Send(encodeRequestGiveNickName("Recruit", "Lord of the Long Hills"))
	wantTitled(t, "noble", drainFrames(t, w.member), w.memberID, "Lord of the Long")
	if id, title := titleUpdate(t, drainFrames(t, w.leader)); id != w.memberID || title != "Lord of the Long" {
		t.Fatalf("onlooker's TitleUpdate = %d %q, want %d \"Lord of the Long\"", id, title, w.memberID)
	}
	if got := storedTitle(t, w, w.memberID); got != "Lord of the Long" {
		t.Fatalf("stored title = %q, want \"Lord of the Long\"", got)
	}

	for _, tc := range []struct {
		who         string
		c           *testsupport.ScriptedClient
		name, title string
		want        int
	}{
		{"an underscore", w.member, "Recruit", "Lord_X", serverpackets.SystemMessageNotWorkingPleaseTryAgainLater},
		{"a non-ASCII letter", w.member, "Recruit", "Lörd", serverpackets.SystemMessageNotWorkingPleaseTryAgainLater},
		{"the name in other case", w.member, "recruit", "Lord", serverpackets.SystemMessageNotAuthorizedToDoThat},
		{"another's name", w.member, "Founder", "Lord", serverpackets.SystemMessageNotAuthorizedToDoThat},
	} {
		tc.c.Send(encodeRequestGiveNickName(tc.name, tc.title))
		if ids := messages(t, drainFrames(t, tc.c)); !slices.Equal(ids, []int{tc.want}) {
			t.Fatalf("%s: answer = %v, want %d", tc.who, ids, tc.want)
		}
	}
	if got := storedTitle(t, w, w.memberID); got != "Lord of the Long" {
		t.Fatalf("stored title after refusals = %q, want it unchanged", got)
	}
}

// TestGiveNickNameNonNobleOutsideClan refuses a player that is no noble,
// even naming itself, outside a clan.
func TestGiveNickNameNonNobleOutsideClan(t *testing.T) {
	t.Parallel()
	w := bootSeededClan(t, 0, 0, false)
	w.member.Send(encodeRequestGiveNickName("Recruit", "Lord"))
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{serverpackets.SystemMessageNotAuthorizedToDoThat}) {
		t.Fatalf("answer = %v, want YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT", ids)
	}
}

// TestGiveNickNameClanMember pins RequestGiveNickName's clan branch in a
// level 3 clan. The leader titles the recruit: the giver is told the
// member's title as asked for, the member takes it cut to 16 characters,
// and it is stored. The leader titles itself without the giver's message.
// A rank 6 member holds no title privilege, a name no member carries and
// a member out of the world are refused.
func TestGiveNickNameClanMember(t *testing.T) {
	t.Parallel()
	w := bootSeededClan(t, 3, 0, false)
	w.recruit(t)

	w.leader.Send(encodeRequestGiveNickName("Recruit", "Squire of the Gate"))
	leader := drainFrames(t, w.leader)
	if got := opcodes(leader); len(got) == 0 || got[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("giver's answer = %x, want the giver's message first", got)
	}
	if id, params := sysMsg(t, leader[0]); id != serverpackets.SystemMessageClanMemberS1TitleChangedToS2 || !slices.Equal(params, []string{"Recruit", "Squire of the Gate"}) {
		t.Fatalf("giver's message = %d %v, want CLAN_MEMBER_S1_TITLE_CHANGED_TO_S2 Recruit \"Squire of the Gate\"", id, params)
	}
	if id, title := titleUpdate(t, leader); id != w.memberID || title != "Squire of the Ga" {
		t.Fatalf("giver's TitleUpdate = %d %q, want %d \"Squire of the Ga\"", id, title, w.memberID)
	}
	wantTitled(t, "member", drainFrames(t, w.member), w.memberID, "Squire of the Ga")
	if got := storedTitle(t, w, w.memberID); got != "Squire of the Ga" {
		t.Fatalf("member's stored title = %q, want \"Squire of the Ga\"", got)
	}

	w.leader.Send(encodeRequestGiveNickName("Founder", "Lord"))
	wantTitled(t, "leader titling itself", drainFrames(t, w.leader), w.leaderID, "Lord")
	drainFrames(t, w.member)

	for _, tc := range []struct {
		who         string
		c           *testsupport.ScriptedClient
		name, title string
		want        int
	}{
		{"rank 6 member", w.member, "Founder", "Fool", serverpackets.SystemMessageNotAuthorizedToDoThat},
		{"nobody", w.leader, "Nobody", "Fool", serverpackets.SystemMessageTargetMustBeInClan},
		{"the name in other case", w.leader, "recruit", "Fool", serverpackets.SystemMessageTargetMustBeInClan},
		{"a bad title", w.leader, "Recruit", "Fool?", serverpackets.SystemMessageNotWorkingPleaseTryAgainLater},
	} {
		tc.c.Send(encodeRequestGiveNickName(tc.name, tc.title))
		if ids := messages(t, drainFrames(t, tc.c)); !slices.Equal(ids, []int{tc.want}) {
			t.Fatalf("%s: answer = %v, want %d", tc.who, ids, tc.want)
		}
	}

	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)
	w.leader.Send(encodeRequestGiveNickName("Recruit", "Fool"))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{serverpackets.SystemMessageTargetNotFound}) {
		t.Fatalf("offline member: answer = %v, want TARGET_IS_NOT_FOUND_IN_THE_GAME", ids)
	}
	if got := storedTitle(t, w, w.memberID); got != "Squire of the Ga" {
		t.Fatalf("offline member's stored title = %q, want it unchanged", got)
	}
}

// TestGiveNickNameNeedsClanLevel3 refuses the leader of a level 2 clan.
func TestGiveNickNameNeedsClanLevel3(t *testing.T) {
	t.Parallel()
	w := bootSeededClan(t, 2, 0, false)
	w.leader.Send(encodeRequestGiveNickName("Founder", "Lord"))
	if ids := messages(t, drainFrames(t, w.leader)); !slices.Equal(ids, []int{serverpackets.SystemMessageClanLvl3NeededToEndowTitle}) {
		t.Fatalf("answer = %v, want CLAN_LVL_3_NEEDED_TO_ENDOWE_TITLE", ids)
	}
}

// leaverTitle returns the title in the first UserInfo among frames.
func leaverTitle(t *testing.T, frames [][]byte) string {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodeUserInfo)
	if !ok {
		t.Fatalf("leaver got no UserInfo among %x", opcodes(frames))
	}
	return userInfoTitle(t, frame)
}

// charInfoTitleOf decodes the title of the CharInfo among frames that
// draws objectID, failing the test when there is none.
func charInfoTitleOf(t *testing.T, frames [][]byte, objectID int32) string {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadBytes(4 * 4) // x, y, z, boat
		if r.ReadInt32() != objectID {
			continue
		}
		r.ReadString()        // name
		r.ReadBytes(3 * 4)    // race, sex, class
		r.ReadBytes(12 * 4)   // paperdoll
		r.ReadBytes(4*2 + 4)  // padding, right-hand augmentation
		r.ReadBytes(12*2 + 4) // padding, left-hand augmentation
		r.ReadBytes(4 * 2)    // padding
		r.ReadBytes(6 * 4)    // pvp flag, karma, casting/attack speed, pvp flag, karma
		r.ReadBytes(8 * 4)    // speeds
		r.ReadBytes(4 * 8)    // speed multipliers, collision
		r.ReadBytes(3 * 4)    // hair style, hair color, face
		title := r.ReadString()
		if err := r.Err(); err != nil {
			t.Fatalf("decode CharInfo: %v", err)
		}
		return title
	}
	t.Fatalf("no CharInfo of %d among %x", objectID, opcodes(frames))
	return ""
}

// TestLeavingClanClearsTitle has a titled member leave its clan in the
// world, by withdrawing or being expelled: noble or not, it loses its
// title in its own UserInfo, in the CharInfo the leader sees, and in its
// stored row (Clan.removeClanMember -> Player.setClan(null) clears every
// online leaver's title). An expelled noble out of the world has its
// stored title cleared too.
func TestLeavingClanClearsTitle(t *testing.T) {
	t.Parallel()
	withdraw := func(t *testing.T, w *clanWorld) (leaver, leader [][]byte) {
		leaver = withdrawRecruit(t, w)
		return leaver, drainFrames(t, w.leader)
	}
	expel := func(t *testing.T, w *clanWorld) (leaver, leader [][]byte) {
		w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
		leader = drainFrames(t, w.leader)
		return drainFrames(t, w.member), leader
	}
	for _, tc := range []struct {
		name  string
		noble bool
		leave func(t *testing.T, w *clanWorld) (leaver, leader [][]byte)
	}{
		{"noble withdraws", true, withdraw},
		{"noble expelled", true, expel},
		{"member withdraws", false, withdraw},
		{"member expelled", false, expel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootSeededClan(t, 3, 0, tc.noble)
			w.recruit(t)
			w.leader.Send(encodeRequestGiveNickName("Recruit", "Lord"))
			drainFrames(t, w.leader)
			drainFrames(t, w.member)
			if got := storedTitle(t, w, w.memberID); got != "Lord" {
				t.Fatalf("stored title before leaving = %q, want Lord", got)
			}
			leaver, leader := tc.leave(t, w)
			if got := leaverTitle(t, leaver); got != "" {
				t.Fatalf("leaver's UserInfo title = %q, want it cleared", got)
			}
			if got := charInfoTitleOf(t, leader, w.memberID); got != "" {
				t.Fatalf("leaver's CharInfo title = %q, want it cleared", got)
			}
			if got := storedTitle(t, w, w.memberID); got != "" {
				t.Fatalf("stored title after leaving = %q, want it cleared", got)
			}
		})
	}

	t.Run("offline noble expelled", func(t *testing.T) {
		t.Parallel()
		w := bootSeededClan(t, 3, 0, true)
		w.recruit(t)
		w.leader.Send(encodeRequestGiveNickName("Recruit", "Lord"))
		drainFrames(t, w.leader)
		w.leaveWorld(t, w.member)
		drainFrames(t, w.leader)
		w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
		drainFrames(t, w.leader)
		if got := storedTitle(t, w, w.memberID); got != "" {
			t.Fatalf("offline noble's stored title after expulsion = %q, want it cleared", got)
		}
	})
}

func withdrawRecruit(t *testing.T, w *clanWorld) [][]byte {
	t.Helper()
	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	return drainFrames(t, w.member)
}

func expelRecruit(t *testing.T, w *clanWorld) [][]byte {
	t.Helper()
	w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
	drainFrames(t, w.leader)
	return drainFrames(t, w.member)
}

// TestJoiningClanClearsTitle has a noble that titled itself join a clan:
// its title is cleared in its own UserInfo, in the CharInfo the inviting
// leader sees, in the clan's roster and in its stored row, as
// Clan.addClanMember -> Player.setClan clears it.
func TestJoiningClanClearsTitle(t *testing.T) {
	t.Parallel()
	w := bootSeededClan(t, 3, 0, true)
	w.member.Send(encodeRequestGiveNickName("Recruit", "Lord"))
	drainFrames(t, w.member)
	drainFrames(t, w.leader)
	if got := storedTitle(t, w, w.memberID); got != "Lord" {
		t.Fatalf("stored title before joining = %q, want Lord", got)
	}

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	drainFrames(t, w.member)
	w.member.Send(encodeRequestAnswerJoinPledge(1))
	joined := drainFrames(t, w.member)
	if _, ok := firstOpcode(joined, serverpackets.OpcodeJoinPledge); !ok {
		t.Fatalf("recruit's join = %x, want JoinPledge", opcodes(joined))
	}
	if got := leaverTitle(t, joined); got != "" {
		t.Fatalf("recruit's UserInfo title after joining = %q, want it cleared", got)
	}
	if got := charInfoTitleOf(t, drainFrames(t, w.leader), w.memberID); got != "" {
		t.Fatalf("recruit's CharInfo title after joining = %q, want it cleared", got)
	}
	cl, ok := w.srv.Clans.Table().Get(titleClanID)
	if !ok {
		t.Fatal("Knights missing from the registry")
	}
	if m, ok := cl.Member(w.memberID); !ok || m.Title != "" {
		t.Fatalf("recruit's roster row = %+v (found %v), want an empty title", m, ok)
	}
	if got := storedTitle(t, w, w.memberID); got != "" {
		t.Fatalf("stored title after joining = %q, want it cleared", got)
	}
}
