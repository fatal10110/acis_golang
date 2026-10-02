package olympiad

import (
	"database/sql"
	"encoding/binary"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// cmdOlympiadStat is the user command id of /olympiadstat.
const cmdOlympiadStat = 109

func olympiadStat(t *testing.T, c *testsupport.ScriptedClient) [][]string {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUserCommand)
	w.WriteInt32(cmdOlympiadStat)
	c.Send(w.Bytes())
	return sysMsgs(t, drainFrames(t, c))
}

func msg(id int, params ...int) []string {
	out := []string{strconv.Itoa(id)}
	for _, p := range params {
		out = append(out, strconv.Itoa(p))
	}
	return out
}

// TestOlympiadStatCommand pins /olympiadstat: a character who is not a
// noble is told only nobles may ask; a noble is told its record for the
// running cycle (matches, wins, defeats, points), all zeros while it has
// none.
func TestOlympiadStatCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		stmts []string
		want  []string
	}{
		{"not a noble", nil, msg(serverpackets.SystemMessageNoblesseOnly)},
		{"noble without a record", []string{
			`UPDATE characters SET nobless = 1 WHERE char_name = 'Stat'`,
		}, msg(serverpackets.SystemMessageOlympiadRecordS1MatchesS2WinsS3Defeats, 0, 0, 0, 0)},
		{"noble with a record", []string{
			`UPDATE characters SET nobless = 1 WHERE char_name = 'Stat'`,
			`INSERT INTO olympiad_nobles SELECT obj_Id, 88, 42, 9, 5, 3, 1, 0 FROM characters WHERE char_name = 'Stat'`,
		}, msg(serverpackets.SystemMessageOlympiadRecordS1MatchesS2WinsS3Defeats, 9, 5, 3, 42)},
		{"record of a character no longer noble", []string{
			`INSERT INTO olympiad_nobles SELECT obj_Id, 88, 42, 9, 5, 3, 1, 0 FROM characters WHERE char_name = 'Stat'`,
		}, msg(serverpackets.SystemMessageNoblesseOnly)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := bootNamed(t, "Stat", tc.stmts)
			if got := olympiadStat(t, c); !slices.EqualFunc(got, [][]string{tc.want}, slices.Equal[[]string]) {
				t.Fatalf("/olympiadstat = %v, want %v", got, [][]string{tc.want})
			}
		})
	}
}

// TestNobleEntersWorld pins what noble status changes in the login burst of
// the leader of a level-1 clan: UserInfo's noble byte, the clan rank lifted
// from the leader's 1 to 5, and the eight noble skills at level 1 in the
// SkillList. A character who is not a noble has none of them.
func TestNobleEntersWorld(t *testing.T) {
	t.Parallel()
	nobleSkills := []int32{325, 326, 327, 1323, 1324, 1325, 1326, 1327}
	for _, noble := range []bool{false, true} {
		t.Run("noble="+strconv.FormatBool(noble), func(t *testing.T) {
			t.Parallel()
			var stmts []string
			if noble {
				stmts = []string{`UPDATE characters SET nobless = 1 WHERE char_name = 'Login'`}
			}
			opts := append(skillOptions(t), gameservertest.WithClanSeed(func(db *sql.DB) {
				for _, stmt := range []string{
					`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT 500, 'Nobles', 1, obj_Id FROM characters WHERE char_name = 'Login'`,
					`UPDATE characters SET clanid = 500 WHERE char_name = 'Login'`,
				} {
					if _, err := db.Exec(stmt); err != nil {
						t.Fatalf("seed %q: %v", stmt, err)
					}
				}
			}))
			_, burst := bootNamed(t, "Login", stmts, opts...)

			info := firstFrame(t, burst, serverpackets.OpcodeUserInfo)
			n := len(info)
			// The tail after the noble byte: hero, fishing, fishing stance
			// x/y/z, name color, running, pledge class, pledge type, title
			// color, cursed weapon stage.
			wantByte, wantClass := byte(0), int32(1)
			if noble {
				wantByte, wantClass = 1, 5
			}
			if got := info[n-36]; got != wantByte {
				t.Errorf("UserInfo noble byte = %d, want %d", got, wantByte)
			}
			if got := int32(binary.LittleEndian.Uint32(info[n-16:])); got != wantClass {
				t.Errorf("UserInfo pledge class = %d, want %d", got, wantClass)
			}

			skills := skillListLevels(t, firstFrame(t, burst, serverpackets.OpcodeSkillList))
			for _, id := range nobleSkills {
				level, ok := skills[id]
				switch {
				case noble && (!ok || level != 1):
					t.Errorf("SkillList skill %d = level %d (known %v), want level 1", id, level, ok)
				case !noble && ok:
					t.Errorf("SkillList of a character who is not a noble has noble skill %d", id)
				}
			}
		})
	}
}
