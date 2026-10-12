package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// killStanding is a killer's stored PvP kills, PK kills and karma.
type killStanding struct{ pvp, pk, karma int64 }

func storedKillStanding(t *testing.T, w *clanWorld, id int32) killStanding {
	t.Helper()
	return killStanding{
		pvp:   queryInt(t, w, `SELECT pvpkills FROM characters WHERE obj_Id = ?`, id),
		pk:    queryInt(t, w, `SELECT pkkills FROM characters WHERE obj_Id = ?`, id),
		karma: queryInt(t, w, `SELECT karma FROM characters WHERE obj_Id = ?`, id),
	}
}

// TestKillAwardBetweenClans has a Rivals member kill a karma-free,
// unflagged player (Player.onKillUpdatePvPKarma, Player.java:2764-2830).
// A kill between clans at war with each other, neither side in its
// academy, is a PvP kill: the killer gains a PvP point and its UserInfo,
// and no karma. Any other kill of an innocent is a PK: a PK kill and 240
// karma. Nothing is awarded when both players stand in a PvP zone.
func TestKillAwardBetweenClans(t *testing.T) {
	t.Parallel()
	mutual := []string{warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID)}
	pvp, pk, none := killStanding{pvp: 1}, killStanding{pk: 1, karma: 240}, killStanding{}
	for _, tt := range []struct {
		name          string
		wars          []string
		byPupil       bool
		academyVictim bool
		killerInPvP   bool
		victimInPvP   bool
		want          killStanding
	}{
		{name: "no war", want: pk},
		{name: "mutual war", wars: mutual, want: pvp},
		{name: "killer's clan declared only", wars: []string{warStmt(rivalsClanID, knightsClanID)}, want: pk},
		{name: "victim's clan declared only", wars: []string{warStmt(knightsClanID, rivalsClanID)}, want: pk},
		{name: "academy killer", wars: mutual, byPupil: true, want: pk},
		{name: "academy victim", wars: mutual, academyVictim: true, want: pk},
		{name: "both in a pvp zone", killerInPvP: true, victimInPvP: true, want: none},
		{name: "both in a pvp zone at war", wars: mutual, killerInPvP: true, victimInPvP: true, want: none},
		{name: "only the victim in a pvp zone", victimInPvP: true, want: pk},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pupilClanID := rivalsClanID
			if tt.academyVictim {
				pupilClanID = knightsClanID
			}
			stmts := append([]string{
				`UPDATE characters SET clanid = ` + itoa(pupilClanID) + `, subpledge = -1, power_grade = 9, lvl_joined_academy = 10 WHERE obj_Id = ` + itoa(pupilID),
			}, tt.wars...)
			w := bootAllianceCast(t, []castMember{{pupilID, "player3", "Pupil"}}, stmts)
			killerID, killerClient, victimID := w.memberID, w.member, w.leaderID
			if tt.byPupil || tt.academyVictim {
				pupil := w.srv.DialClient(t, "player3", 1)
				startInWorld(t, pupil)
				if tt.byPupil {
					killerID, killerClient = pupilID, pupil
				} else {
					victimID = pupilID
				}
				drainFrames(t, w.leader)
				drainFrames(t, w.member)
				drainFrames(t, pupil)
			}
			setInPvPZone(t, w, killerID, tt.killerInPvP)
			setInPvPZone(t, w, victimID, tt.victimInPvP)

			killPlayer(t, w, victimID, killerID)
			frames := drainFrames(t, killerClient)
			told := slices.Contains(messages(t, frames), serverpackets.SystemMessageYourKarmaHasBeenChangedToS1)
			if told != (tt.want.karma != 0) {
				t.Fatalf("killer told its karma changed = %v, want %v", told, tt.want.karma != 0)
			}
			if _, ok := firstOpcode(frames, serverpackets.OpcodeUserInfo); ok != (tt.want != none) {
				t.Fatalf("killer sent its UserInfo = %v, want %v: %x", ok, tt.want != none, opcodes(frames))
			}

			w.leaveWorld(t, killerClient)
			if got := storedKillStanding(t, w, killerID); got != tt.want {
				t.Fatalf("killer's stored standing = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// setInPvPZone records whether the player id stands in a PvP zone.
func setInPvPZone(t *testing.T, w *clanWorld, id int32, inside bool) {
	t.Helper()
	if !inside {
		return
	}
	c := onlineCharacter(t, w.srv, id)
	onPlayerQueue(t, w.srv, id, func() { c.SetInPvPZone(true) })
}
