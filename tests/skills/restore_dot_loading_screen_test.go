package skills

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// loadingPoisonID is a saved ten-tick, 3s-period poison: a restored effect
// whose ticks deal damage.
const (
	loadingPoisonID     = 84
	loadingPoisonTicks  = 10
	loadingPoisonPeriod = 3
	loadingPoisonDamage = 5
	// loadingPoisonHold outlasts the poison's whole 30s run.
	loadingPoisonHold = 40 * time.Second
	// loadingPoisonHP is the HP the character is saved with, above what
	// the poison's whole run takes.
	loadingPoisonHP = 60
)

func loadingPoisonDef() modelskill.Definition {
	return modelskill.Definition{
		ID: loadingPoisonID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "POISON",
		Effects: []modelskill.EffectTemplate{{
			Name: "DamOverTime", Count: loadingPoisonTicks, Time: loadingPoisonPeriod, Value: loadingPoisonDamage,
			StackType: "poison", StackOrder: 1, EffectType: "POISON", Icon: true,
		}},
	}
}

// bootSavedPoison boots a character saved with loadingPoisonHP HP and a
// saved row of def's poison, ticks left and elapsed seconds into its period.
func bootSavedPoison(t *testing.T, def modelskill.Definition, ticks, elapsed int32) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding the loading screen for a set time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	ctx := context.Background()
	if _, err := srv.DB.ExecContext(ctx, `UPDATE characters SET curHp = ? WHERE obj_Id = ?`, loadingPoisonHP, objID); err != nil {
		t.Fatalf("seed saved HP: %v", err)
	}
	if _, err := srv.DB.ExecContext(ctx,
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, def.ID, 1, ticks, elapsed, 60_000, time.Now().Add(time.Minute).UnixMilli(), 0, 0, 1); err != nil {
		t.Fatalf("seed saved poison: %v", err)
	}
	return srv, objID
}

// userInfoCurrentHP reads the current HP a UserInfo frame carries.
func userInfoCurrentHP(t *testing.T, frame []byte) int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeUserInfo, "UserInfo")
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64()
	for range 6 + 1 { // STR..MEN, max HP
		r.ReadInt32()
	}
	return r.ReadInt32()
}

// savedHP reads the HP the character row holds.
func savedHP(t *testing.T, srv *gameservertest.Server, objID int32) float64 {
	t.Helper()
	var hp float64
	if err := srv.DB.QueryRowContext(context.Background(), `SELECT curHp FROM characters WHERE obj_Id = ?`, objID).Scan(&hp); err != nil {
		t.Fatalf("read the saved HP: %v", err)
	}
	return hp
}

// savedEffectRow reads the restore_type 0 row saved for skillID; ok is
// false when there is none.
func savedEffectRow(t *testing.T, srv *gameservertest.Server, objID int32, skillID int) (count, curTime int32, ok bool) {
	t.Helper()
	err := srv.DB.QueryRowContext(context.Background(),
		`SELECT effect_count, effect_cur_time FROM character_skills_save WHERE char_obj_id = ? AND skill_id = ? AND restore_type = 0`,
		objID, skillID).Scan(&count, &curTime)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false
	}
	if err != nil {
		t.Fatalf("read the saved row of skill %d: %v", skillID, err)
	}
	return count, curTime, true
}

// TestRestoredPoisonHeldPastItsRunEntersWithItsDamage: the ticks of a
// restored damage-over-time effect that fall due on the loading screen run
// their damage at the replay (#3266). A loading screen held past the
// poison's whole run enters the world with every tick's damage taken and
// the poison gone: holding it does not clear the poison for free.
func TestRestoredPoisonHeldPastItsRunEntersWithItsDamage(t *testing.T) {
	t.Parallel()
	srv, _ := bootSavedPoison(t, loadingPoisonDef(), loadingPoisonTicks, 1)
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, loadingPoisonHold)
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurstWithRestoredBuff(t, c)

	if entries := readAbnormalStatusUpdateEntriesFromFrame(t, frames[3]); len(entries) != 0 {
		t.Fatalf("EnterWorld AbnormalStatusUpdate after a %v loading screen = %+v, want the ended poison gone", loadingPoisonHold, entries)
	}
	if hp, want := userInfoCurrentHP(t, frames[10]), int32(loadingPoisonHP-loadingPoisonTicks*loadingPoisonDamage); hp != want {
		t.Fatalf("EnterWorld UserInfo HP = %d, want %d: all %d ticks came due on the loading screen", hp, want, loadingPoisonTicks)
	}
}

// TestRestoredPoisonDropOnLoadingScreenSavesItsDamage: a drop on the
// loading screen saves the poison with the ticks due there run, their
// damage in the saved HP, never with ticks spent and no damage.
func TestRestoredPoisonDropOnLoadingScreenSavesItsDamage(t *testing.T) {
	t.Parallel()
	t.Run("mid run", func(t *testing.T) {
		t.Parallel()
		// Saved 1s into its period: ticks fall due 2s, 5s and 8s after the
		// restore, so a drop at 10s saves 7 ticks, 2s into the next period.
		srv, objID := bootSavedPoison(t, loadingPoisonDef(), loadingPoisonTicks, 1)
		c := srv.Client
		selectOnly(t, c)
		srv.Advance(t, 10*time.Second)
		dropOnLoadingScreen(t, srv, c, objID)

		count, curTime, ok := savedEffectRow(t, srv, objID, loadingPoisonID)
		if !ok || count != loadingPoisonTicks-3 || curTime != 2 {
			t.Fatalf("saved poison after a drop 10s into the loading screen = count %d time %d (row %v), want count %d time 2",
				count, curTime, ok, loadingPoisonTicks-3)
		}
		if hp, want := savedHP(t, srv, objID), float64(loadingPoisonHP-3*loadingPoisonDamage); hp != want {
			t.Fatalf("saved HP after the drop = %g, want %g: three ticks' damage", hp, want)
		}
	})
	t.Run("past its run", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootSavedPoison(t, loadingPoisonDef(), loadingPoisonTicks, 1)
		c := srv.Client
		selectOnly(t, c)
		srv.Advance(t, loadingPoisonHold)
		dropOnLoadingScreen(t, srv, c, objID)

		if count, curTime, ok := savedEffectRow(t, srv, objID, loadingPoisonID); ok {
			t.Fatalf("saved poison after a drop past its run = count %d time %d, want no effect row", count, curTime)
		}
		if hp, want := savedHP(t, srv, objID), float64(loadingPoisonHP-loadingPoisonTicks*loadingPoisonDamage); hp != want {
			t.Fatalf("saved HP after the drop = %g, want %g: every tick's damage", hp, want)
		}
	})
}
