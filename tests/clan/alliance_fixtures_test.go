package clan

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Seeded alliance-test clans and their leaders: far above anything the id
// factory hands out during a test.
const (
	knightsClanID int32 = 0x7f000011
	rivalsClanID  int32 = 0x7f000012
	rivalLeaderID int32 = 0x7f200001
)

// The alliance system messages, as the reference numbers them.
const (
	allyMsgIsNotAClanLeader       = 9
	allyMsgCannotInviteYourself   = 4
	allyMsgNotAClanMember         = 212
	allyMsgFeatureOnlyForLeader   = 464
	allyMsgNoCurrentAlliances     = 465
	allyMsgExceededTheLimit       = 466
	allyMsgCantInviteWithin1Day   = 467
	allyMsgMayNotAllyClanBattle   = 469
	allyMsgOnlyLeaderWithdraw     = 470
	allyMsgLeaderCantWithdraw     = 471
	allyMsgDifferentAlliance      = 473
	allyMsgClanDoesntExist        = 474
	allyMsgNoResponse             = 477
	allyMsgYouDidNotRespond       = 478
	allyMsgInfoHead               = 491
	allyMsgInfoName               = 492
	allyMsgConnection             = 493
	allyMsgInfoLeader             = 494
	allyMsgClanTotal              = 495
	allyMsgClanHead               = 496
	allyMsgClanName               = 497
	allyMsgClanLeader             = 498
	allyMsgClanLevel              = 499
	allyMsgClanSeparator          = 500
	allyMsgClanFoot               = 501
	allyMsgAlreadyJoined          = 502
	allyMsgIncorrectName          = 506
	allyMsgIncorrectNameLength    = 507
	allyMsgAlreadyExists          = 508
	allyMsgAccepted               = 517
	allyMsgWithdrawn              = 519
	allyMsgExpelledAClan          = 521
	allyMsgDissolved              = 523
	allyMsgRequested              = 527
	allyMsgLevel5Needed           = 549
	allyMsgAlreadyMemberOfAlly    = 691
	allyMsgCantEnterWithin1DayS1  = 761
	allyMsgCantCreate10DaysDissol = 505
)

// The alliance chat channel id.
const sayAlliance int32 = 9

// alliancePages extends the clan dialog pages with the alliance dialog's,
// whose links name the alliance commands.
func alliancePages(t *testing.T) map[string]string {
	t.Helper()
	pages := clanPages(t)
	key := "villagemaster/" + strconv.Itoa(masterID) + ".htm"
	for _, name := range []string{"9001-01.htm", "9001-02.htm", "9001-03.htm"} {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "feature", "Alliance", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		pages[key] += string(data)
	}
	return pages
}

// seedRival stores the rival clan's leader, Rival, on account player2.
func seedRival(t *testing.T) gameservertest.Option {
	return gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		tmpl, ok := gameservertest.Templates(t).Get(0)
		if !ok {
			t.Fatal("missing test class template")
		}
		ch, err := player.NewCharacter(rivalLeaderID, tmpl, "player2", "Rival", 1, 0, 0, player.SexMale)
		if err != nil {
			t.Fatalf("seed rival: %v", err)
		}
		ch.CharLevel = 10
		if err := chars.Create(context.Background(), ch); err != nil {
			t.Fatalf("seed rival: %v", err)
		}
	})
}

// seedAllianceClans seeds the founder's level-5 clan Knights and the
// rival's level-5 clan Rivals, each led by its player, the founder with
// 60000 experience, plus extra statements.
func seedAllianceClans(t *testing.T, extra ...string) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		ctx := context.Background()
		stmts := append([]string{
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
				SELECT ` + itoa(knightsClanID) + `, 'Knights', 5, obj_Id FROM characters WHERE char_name = 'Founder'`,
			`UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 0, exp = 60000 WHERE char_name = 'Founder'`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
				VALUES (` + itoa(rivalsClanID) + `, 'Rivals', 5, ` + itoa(rivalLeaderID) + `)`,
			`UPDATE characters SET clanid = ` + itoa(rivalsClanID) + `, power_grade = 0 WHERE obj_Id = ` + itoa(rivalLeaderID),
		}, extra...)
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				t.Fatalf("seed alliance clans: %v", err)
			}
		}
	})
}

// bootAllianceWorld boots the founder, leading Knights, and the rival,
// leading Rivals, both in the world beside a village master whose page
// links the clan and alliance commands.
func bootAllianceWorld(t *testing.T, extra ...gameservertest.Option) *clanWorld {
	t.Helper()
	return bootAllianceWorldSeeded(t, nil, extra...)
}

// bootAllianceWorldSeeded is bootAllianceWorld with stmts run on the
// seeded clans before they are restored.
func bootAllianceWorldSeeded(t *testing.T, stmts []string, extra ...gameservertest.Option) *clanWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(alliancePages(t)),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithLevels(warLevels(t)),
		seedRival(t),
		seedAllianceClans(t, stmts...),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t), memberID: rivalLeaderID}
	w.member = srv.DialClient(t, "player2", 1)
	w.enter(t)
	return w
}

// formAlliance has the founder found the alliance Ally and the rival's
// clan join it.
func (w *clanWorld) formAlliance(t *testing.T) {
	t.Helper()
	w.masterCommandBy(t, w.leader, "create_ally Ally")
	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.member.Send(encodeAllyTarget(clientpackets.OpcodeRequestAnswerJoinAlly, 1))
	if ids := messages(t, drainFrames(t, w.member)); len(ids) == 0 || ids[len(ids)-1] != allyMsgAccepted {
		t.Fatalf("rival's acceptance messages = %v, want ending in YOU_ACCEPTED_ALLIANCE", ids)
	}
	drainFrames(t, w.leader)
}

// encodeAllyTarget builds an alliance packet carrying one int: a target
// object id or an answer.
func encodeAllyTarget(opcode byte, v int32) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(v)
	return w.Bytes()
}

func encodeAllyName(opcode byte, name string) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteString(name)
	return w.Bytes()
}

func encodeAllyBare(opcode byte) []byte { return wire.NewPacketWriter(opcode).Bytes() }

func encodeSetAllyCrest(data []byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSetAllyCrest)
	w.WriteInt32(int32(len(data)))
	w.WriteBytes(data)
	return w.Bytes()
}

func encodeSay(typ int32, text string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(typ)
	return w.Bytes()
}

// allyRow is one clan_data row's alliance columns.
type allyRow struct {
	allyID        int64
	allyName      string
	allyCrestID   int64
	penaltyExpiry int64
	penaltyType   int64
}

// storedAlly reads clanID's alliance columns once the writes have landed.
func storedAlly(t *testing.T, w *clanWorld, clanID int32) allyRow {
	t.Helper()
	w.srv.FlushPersistence(t)
	var r allyRow
	var name sql.NullString
	err := w.srv.DB.QueryRowContext(context.Background(),
		`SELECT ally_id, ally_name, ally_crest_id, ally_penalty_expiry_time, ally_penalty_type FROM clan_data WHERE clan_id = ?`, clanID).
		Scan(&r.allyID, &name, &r.allyCrestID, &r.penaltyExpiry, &r.penaltyType)
	if err != nil {
		t.Fatalf("read clan %d alliance: %v", clanID, err)
	}
	r.allyName = name.String
	return r
}
