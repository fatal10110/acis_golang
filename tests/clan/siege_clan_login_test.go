package clan

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: a clan leader holds the siege skills while its clan is of
// siege level (SiegeClanMinLevel, 4): a level-up past 3 gives them
// (Clan.changeLevel), and a leader change moves them from the former
// leader to the new one (Clan.setNewLeader). EnterWorld tells a member of
// a clan whose hall lease is unpaid PAYMENT_FOR_YOUR_CLAN_HALL_HAS_NOT_
// BEEN_MADE... (1051), between the login notices and the member's own
// roster row. With ZoneTown 1 a town gives a player taking part in a
// siege neither the peace nor the town flag (TownZone.onEnter).

// shippedSiegeSkills runs the shipped skill definitions, which hold the
// siege skills.
func shippedSiegeSkills(t *testing.T) gameservertest.Option {
	t.Helper()
	defs, _ := shippedSkillData(t)
	db := sqltest.SharedDB(t)
	return gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db)))
}

// wantSiegeSkills fails unless the online player objID knows Seal of Ruler
// and Build Headquarters at level want.
func wantSiegeSkills(t *testing.T, w *clanWorld, who string, objID int32, want int) {
	t.Helper()
	for _, id := range []int{sealOfRuler, buildHQ} {
		if got := w.srv.PlayerSkillLevel(t, objID, id); got != want {
			t.Fatalf("%s's skill %d = %d, want %d", who, id, got, want)
		}
	}
}

// TestClanLevel4GivesTheLeaderSiegeSkills raises a level 3 clan to 4: its
// leader, without the siege skills before, holds them after.
func TestClanLevel4GivesTheLeaderSiegeSkills(t *testing.T) {
	t.Parallel()
	w := bootClanWorldCarrying(t, 40, 1400000, map[int32]int32{3874: 1}, clanLevelItems(), shippedSiegeSkills(t), seedClan(t, clanSeed{level: 3}))
	w.talkToMaster(t)
	wantSiegeSkills(t, w, "level 3 clan's leader", w.leaderID, 0)

	w.masterCommand(t, "increase_clan_level")
	if level, _ := storedClan(t, w); level != 4 {
		t.Fatalf("stored clan level = %d, want 4", level)
	}
	wantSiegeSkills(t, w, "level 4 clan's leader", w.leaderID, 1)
}

// TestClanLeaderTransferMovesTheSiegeSkills hands the level 5 Knights from
// the founder to the recruit: the siege skills go with the leadership.
func TestClanLeaderTransferMovesTheSiegeSkills(t *testing.T) {
	t.Parallel()
	w, _ := bootLeaderTransfer(t, shippedSiegeSkills(t))
	wantSiegeSkills(t, w, "leader", w.leaderID, 1)
	wantSiegeSkills(t, w, "recruit", w.memberID, 0)

	nominateRecruit(t, w)
	runTransfer(t, w)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	wantSiegeSkills(t, w, "former leader", w.leaderID, 0)
	wantSiegeSkills(t, w, "new leader", w.memberID, 1)
}

// TestUnpaidClanHallLeaseNoticeAtLogin logs in the leader of the clan
// owning Moonstone Hall: with the lease unpaid it is told so, with no
// lease named, right after the clan's skill list and ahead of its own
// roster row; with the lease paid it is not.
func TestUnpaidClanHallLeaseNoticeAtLogin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		paid bool
	}{{"unpaid", false}, {"paid", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			datapack.Require(t)
			halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
			if err != nil {
				t.Fatal(err)
			}
			decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
			if err != nil {
				t.Fatal(err)
			}
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 40, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithClanHalls(halls, decos),
				gameservertest.WithClanSeed(func(db *sql.DB) {
					seedStatements(t, db,
						`UPDATE characters SET clanid = `+itoa(hallClanID)+`, power_grade = 0 WHERE char_name = 'Newbie'`,
						`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
							SELECT `+itoa(hallClanID)+`, 'Hallkeepers', 4, obj_Id FROM characters WHERE char_name = 'Newbie'`,
					)
					if _, err := db.ExecContext(context.Background(), "INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (?, ?, ?, ?)",
						moonstoneHall, hallClanID, tc.paid, future()); err != nil {
						t.Fatalf("seed clan hall: %v", err)
					}
				}),
			)
			frames := startInWorld(t, srv.Client)
			i := slices.IndexFunc(frames, isPledgeSkillList)
			if i < 0 {
				t.Fatalf("login = %x, want the clan's skill list", opcodes(frames))
			}
			next := frames[i+1]
			if !tc.paid {
				if id, params := sysMsg(t, next); id != serverpackets.SystemMessageClanHallPaymentDueTomorrowS1 || len(params) != 0 {
					t.Fatalf("frame after the clan's skill list = %d %v, want 1051 with no parameter", id, params)
				}
				next = frames[i+2]
			}
			if next[0] != serverpackets.OpcodePledgeShowMemberListUpdate {
				t.Fatalf("login = %x, want the member's roster row next", opcodes(frames))
			}
			if tc.paid && slices.Contains(messages(t, frames), serverpackets.SystemMessageClanHallPaymentDueTomorrowS1) {
				t.Fatal("a paid lease was reported unpaid")
			}
		})
	}
}

// TestTownCombatRuleKeepsSiegeParticipantsOutOfPeace runs Gludio's siege
// with Knights attacking and a town under ZoneTown 1: Third, attacking
// with Knights, logs in inside the town and takes neither its peace nor
// its town flag; Loner, in no clan, logs in beside it and is at peace.
func TestTownCombatRuleKeepsSiegeParticipantsOutOfPeace(t *testing.T) {
	t.Parallel()
	townAt := location.Location{X: -40000, Y: 20000, Z: 0}
	zones := siegeClanZones(t)
	form, err := zone.NewCuboid(townAt.X-1000, townAt.X+1000, townAt.Y-1000, townAt.Y+1000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	town := zone.NewTown(21, form)
	town.CombatRule = 1
	zones.Add(town)
	inTown := func(id int32) string {
		return `UPDATE characters SET x = ` + itoa(int32(townAt.X)) + `, y = ` + itoa(int32(townAt.Y)) + `, z = 0 WHERE obj_Id = ` + itoa(id)
	}
	w := bootSiegeClanWorld(t, []castMember{loner, third}, []string{
		registerOnGludio(knightsClanID, "ATTACKER"),
		`UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(thirdID),
		inTown(thirdID), inTown(lonerID),
	}, gameservertest.WithZones(zones))
	startGludio(t, w)

	w.bringIn(t, third)
	w.bringIn(t, loner)
	for _, tc := range []struct {
		name  string
		id    int32
		peace bool
	}{{"attacker", thirdID, false}, {"clanless", lonerID, true}} {
		obj, ok := w.srv.State.Player(tc.id)
		if !ok {
			t.Fatalf("%s is not in the world", tc.name)
		}
		c, ok := network.OnlineCharacter(obj)
		if !ok {
			t.Fatalf("%s is not an online character", tc.name)
		}
		if got := c.InPeaceZone(); got != tc.peace {
			t.Fatalf("%s in the town: peace = %v, want %v", tc.name, got, tc.peace)
		}
	}
}
