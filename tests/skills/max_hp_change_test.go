package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ownVitalsStatuses returns the index and attributes of every StatusUpdate
// of objID among frames that carries its current HP.
func ownVitalsStatuses(frames [][]byte, objID int32) (at []int, attrs []map[serverpackets.StatusType]int32) {
	for i, frame := range frames {
		if frame[0] != serverpackets.OpcodeStatusUpdate {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() != objID {
			continue
		}
		got := map[serverpackets.StatusType]int32{}
		for range r.ReadInt32() {
			typ, value := r.ReadInt32(), r.ReadInt32()
			got[serverpackets.StatusType(typ)] = value
		}
		if _, ok := got[serverpackets.StatusCurrentHP]; ok {
			at, attrs = append(at, i), append(attrs, got)
		}
	}
	return at, attrs
}

// requireMaxChangeStatus requires the last UserInfo of frames to show
// current HP hp, and frames to carry exactly one own vitals StatusUpdate,
// after that UserInfo, reporting current HP hp and the player's own vitals
// set.
func requireMaxChangeStatus(t *testing.T, frames [][]byte, objID int32, hp int, when string) {
	t.Helper()
	_, userInfo := lastUserInfoCombat(t, frames, when)
	if got := userInfoCurrentHP(t, frames[userInfo]); got != int32(hp) {
		t.Fatalf("%s: UserInfo current HP = %d, want %d", when, got, hp)
	}
	at, attrs := ownVitalsStatuses(frames, objID)
	if len(at) != 1 || at[0] < userInfo {
		t.Fatalf("%s: own vitals StatusUpdates at %v, UserInfo at %d; want one after the UserInfo", when, at, userInfo)
	}
	_, mp := attrs[0][serverpackets.StatusCurrentMP]
	_, cp := attrs[0][serverpackets.StatusCurrentCP]
	_, maxCP := attrs[0][serverpackets.StatusMaxCP]
	if len(attrs[0]) != 4 || !mp || !cp || !maxCP {
		t.Fatalf("%s: StatusUpdate attributes %v, want current HP, MP, CP and max CP", when, attrs[0])
	}
	if got := attrs[0][serverpackets.StatusCurrentHP]; got != int32(hp) {
		t.Fatalf("%s: StatusUpdate current HP = %d, want %d", when, got, hp)
	}
}

// TestMaxHPBuffChangesReportStatusAndClampHP: a max-HP buff landing on a
// player at full HP reports its status once, after the UserInfo of the stat
// change, at the old HP below the raised maximum. Filled to the raised
// maximum, the player drops to the old one when the buff ends: that
// change's UserInfo already shows the old maximum as current HP, and the
// same single report follows it.
func TestMaxHPBuffChangesReportStatusAndClampHP(t *testing.T) {
	t.Parallel()
	const (
		skillID = 9310
		bonus   = 500
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		statClass(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			selfBuff(skillID, modelskill.FuncTemplate{Op: modelskill.FuncAdd, Stat: "maxHp", Value: bonus}),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)
	maxHP := srv.PlayerMaxHP(t, objID)
	if got := srv.PlayerCurrentHP(t, objID); got != maxHP {
		t.Fatalf("HP on entry = %d, want full %d", got, maxHP)
	}

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	srv.Advance(t, 700*time.Millisecond)
	requireMaxChangeStatus(t, collectUntilQuiet(t, c), objID, maxHP, "buff landed")
	if got := srv.PlayerMaxHP(t, objID); got != maxHP+bonus {
		t.Fatalf("max HP under the buff = %d, want %d", got, maxHP+bonus)
	}

	srv.AddPlayerHP(t, objID, bonus)
	if got := srv.PlayerCurrentHP(t, objID); got != maxHP+bonus {
		t.Fatalf("HP filled under the buff = %d, want %d", got, maxHP+bonus)
	}
	srv.Advance(t, 2200*time.Millisecond)
	srv.TickEffects()
	requireMaxChangeStatus(t, collectUntilQuiet(t, c), objID, maxHP, "buff ended")
	if got := srv.PlayerCurrentHP(t, objID); got != maxHP {
		t.Fatalf("HP after the buff ended = %d, want the old maximum %d", got, maxHP)
	}
}
