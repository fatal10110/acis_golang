package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// strifeSkills holds the Seal of Strife skills as aCis_datapack defines
// them (5000-5099.xml): passives multiplying the maximum CP by 1.1 and 0.9.
func strifeSkills(t *testing.T) gameservertest.Option {
	t.Helper()
	passive := func(ref modelskill.Ref, mul float64) modelskill.Definition {
		return modelskill.Definition{
			ID: ref.ID, Level: ref.Level, Activation: modelskill.ActivationPassive,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "maxCp", Value: mul}},
		}
	}
	table := modelskill.NewTable([]modelskill.Definition{
		passive(modelskill.TheVictorOfWar, 1.1),
		passive(modelskill.TheVanquishedOfWar, 0.9),
	})
	db := sqltest.SharedDB(t)
	return gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db)))
}

// strifeStatus is a competition Dawn leads, with the Seal of Strife owned
// by Dawn, in period.
func strifeStatus(period sevensigns.Period) func(*sevensigns.StatusRow) {
	return func(row *sevensigns.StatusRow) {
		row.Period = period
		row.DawnStoneScore = 100
		row.SealOwners = [3]sevensigns.Cabal{sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dawn}
	}
}

func assertStrifeSkills(t *testing.T, w *sevenSignsWorld, when string, victor, vanquished int) {
	t.Helper()
	c := w.online(t)
	if got := [2]int{c.SkillLevel(int(modelskill.TheVictorOfWar.ID)), c.SkillLevel(int(modelskill.TheVanquishedOfWar.ID))}; got != [2]int{victor, vanquished} {
		t.Fatalf("%s: victor, vanquished levels = %v, want [%d %d]", when, got, victor, vanquished)
	}
	var stored int
	if err := w.srv.DB.QueryRow("SELECT COUNT(*) FROM character_skills WHERE char_obj_id = ? AND skill_id IN (5074, 5075)", w.objID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("%s: %d Seal of Strife rows in character_skills, want none", when, stored)
	}
}

// TestSealOfStrifeSkillsFollowThePeriodChange pins giveSosEffect and
// removeSosEffect (SevenSignsManager.SevenSignsPeriodChange): as results
// end, a Dawn member online, its cabal owning the Seal of Strife, gets The
// Victor of War unstored and without a skill list, its new maximum CP shown
// in a UserInfo ahead of the period's sound; as seal validation ends it
// loses it again, after the period's end is announced.
func TestSealOfStrifeSkillsFollowThePeriodChange(t *testing.T) {
	t.Parallel()
	w := bootSevenSigns(t, sevenSignsSetup{status: strifeStatus(sevensigns.Results), cabal: sevensigns.Dawn}, strifeSkills(t))
	w.enter(t)
	assertStrifeSkills(t, w, "results", 0, 0)

	frames := w.fire(t)
	info, sound := firstOpcode(frames, serverpackets.OpcodeUserInfo), firstOpcode(frames, serverpackets.OpcodePlaySound)
	if info < 0 || sound < 0 || info > sound || firstOpcode(frames, serverpackets.OpcodeSkillList) >= 0 {
		t.Fatalf("results end frames = %x, want UserInfo ahead of PlaySound and no SkillList", opcodes(frames))
	}
	assertStrifeSkills(t, w, "seal validation", 1, 0)

	frames = w.fire(t)
	ended := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == serverpackets.SystemMessageSealValidationPeriodEnded {
			ended = i
		}
	}
	info = firstOpcode(frames, serverpackets.OpcodeUserInfo)
	if ended < 0 || info < ended || firstOpcode(frames, serverpackets.OpcodeSkillList) >= 0 {
		t.Fatalf("seal validation end frames = %x, want UserInfo after SEAL_VALIDATION_PERIOD_ENDED and no SkillList", opcodes(frames))
	}
	assertStrifeSkills(t, w, "recruiting", 0, 0)
}

// TestSealOfStrifeSkillAtEnterWorld pins EnterWorld's Seal of Strife check:
// during seal validation with the seal owned, a member of the owning cabal
// enters with The Victor of War and one of the other cabal with The
// Vanquished of War, both shown by the burst's own SkillList and neither
// stored; the burst carries no extra frame. In any other period nobody
// gets one.
func TestSealOfStrifeSkillAtEnterWorld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		period             sevensigns.Period
		cabal              sevensigns.Cabal
		victor, vanquished int32
	}{
		{"owner in validation", sevensigns.SealValidation, sevensigns.Dawn, 1, 0},
		{"rival in validation", sevensigns.SealValidation, sevensigns.Dusk, 0, 1},
		{"no cabal in validation", sevensigns.SealValidation, sevensigns.NoCabal, 0, 0},
		{"owner in competition", sevensigns.Competition, sevensigns.Dawn, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootSevenSigns(t, sevenSignsSetup{status: strifeStatus(tc.period), cabal: tc.cabal}, strifeSkills(t))
			c := w.srv.Client
			c.Send(encodeRequestGameStart(0))
			c.Read() // SSQInfo
			c.Read() // CharSelected
			c.Send(encodeEnterWorld())
			burst := readEnterWorldBurst(t, c)
			levels := map[int32]int32{}
			r := wire.NewReader(burst[7][1:])
			for range int(r.ReadInt32()) {
				r.ReadInt32() // passive
				level := r.ReadInt32()
				levels[r.ReadInt32()] = level
				r.ReadUint8() // disabled
			}
			if got := [2]int32{levels[5074], levels[5075]}; got != [2]int32{tc.victor, tc.vanquished} {
				t.Fatalf("burst SkillList victor, vanquished levels = %v, want [%d %d]", got, tc.victor, tc.vanquished)
			}
			assertStrifeSkills(t, w, "entered", int(tc.victor), int(tc.vanquished))
		})
	}
}
