package skills

import (
	"context"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// loadingBuffID is a one-tick, ten-minute self buff whose saved row the
// tests below seed directly.
const (
	loadingBuffID     = 1204
	loadingBuffPeriod = 600
	loadingScreen     = 7 * time.Second
)

func loadingBuffDef() modelskill.Definition {
	return modelskill.Definition{
		ID: loadingBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 1, Time: loadingBuffPeriod, Icon: true}},
	}
}

// bootSavedBuff boots a character with a saved loadingBuff row elapsed
// seconds into its period, and a reuse timer a minute out.
func bootSavedBuff(t *testing.T, elapsed int32, opts ...gameservertest.Option) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
	}, opts...)...)
	if !srv.DrivesClock() {
		t.Skip("holding the loading screen for a set time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(),
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, loadingBuffID, 1, 1, elapsed, 60_000, time.Now().Add(time.Minute).UnixMilli(), 0, 0, 1); err != nil {
		t.Fatalf("seed saved buff: %v", err)
	}
	return srv, objID
}

// selectOnly sends the selection and reads its answer, stopping on the
// loading screen ahead of EnterWorld.
func selectOnly(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
}

// TestRestoredBuffTimerRunsFromSelection pins #3217: the reference restores
// a saved effect at character selection (Player.restore ->
// restoreEffects, Player.java:4140-4143), where setTime and scheduleEffect
// start its period (AbstractEffect.java:133-136, 186-194). A loading screen
// held for N seconds therefore leaves the buff N seconds less at
// EnterWorld, though its icon still lands at the burst's
// AbnormalStatusUpdate.
func TestRestoredBuffTimerRunsFromSelection(t *testing.T) {
	t.Parallel()
	const elapsed = 100
	srv, _ := bootSavedBuff(t, elapsed, gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingBuffDef()})))
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, loadingScreen)
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurstWithRestoredBuff(t, c)

	entries := readAbnormalStatusUpdateEntriesFromFrame(t, frames[3])
	if len(entries) != 1 || entries[0].SkillID != loadingBuffID {
		t.Fatalf("EnterWorld AbnormalStatusUpdate = %+v, want the restored buff alone", entries)
	}
	want := int32(loadingBuffPeriod - elapsed - loadingScreen/time.Second)
	if got := entries[0].Duration; got > want || got < want-1 {
		t.Fatalf("restored buff duration at EnterWorld = %ds, want %ds: the saved %ds left less the %v loading screen",
			got, want, loadingBuffPeriod-elapsed, loadingScreen)
	}
}

// TestRestoredBuffDropBeforeEnterWorldSavesLoadingTime pins the drop half
// of #3217: a connection lost on the loading screen saves the restored buff
// with the loading time counted (deleteMe -> store -> storeEffect,
// AbstractEffect.getTime), not as it was read back.
func TestRestoredBuffDropBeforeEnterWorldSavesLoadingTime(t *testing.T) {
	t.Parallel()
	const elapsed = 100
	srv, objID := bootSavedBuff(t, elapsed, gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingBuffDef()})))
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, loadingScreen)
	dropOnLoadingScreen(t, srv, c, objID)

	var count, curTime int32
	if err := srv.DB.QueryRowContext(context.Background(),
		`SELECT effect_count, effect_cur_time FROM character_skills_save WHERE char_obj_id = ? AND skill_id = ? AND restore_type = 0`,
		objID, loadingBuffID).Scan(&count, &curTime); err != nil {
		t.Fatalf("read the saved buff back: %v", err)
	}
	if want := int32(elapsed + loadingScreen/time.Second); count != 1 || curTime != want {
		t.Fatalf("saved buff after a drop on the loading screen = count %d time %d, want count 1 time %d", count, curTime, want)
	}
}

// TestRestoredBuffEndingOnLoadingScreenIsGone: a buff whose time runs out
// while the loading screen is held has ended by EnterWorld, so the burst
// carries no icon for it, and a drop saves only its reuse timer.
func TestRestoredBuffEndingOnLoadingScreenIsGone(t *testing.T) {
	t.Parallel()
	const elapsed = loadingBuffPeriod - 5
	t.Run("enter world", func(t *testing.T) {
		t.Parallel()
		srv, _ := bootSavedBuff(t, elapsed, gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingBuffDef()})))
		c := srv.Client
		selectOnly(t, c)
		srv.Advance(t, loadingScreen)
		c.Send(encodeEnterWorld())
		frames := readEnterWorldBurst(t, c)
		coolTimes := readSkillCoolTimeEntriesFromFrame(t, frames[len(frames)-2])
		if len(coolTimes) != 1 || coolTimes[0].SkillID != loadingBuffID {
			t.Fatalf("SkillCoolTime = %+v, want the buff's reuse timer", coolTimes)
		}
	})
	t.Run("drop", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootSavedBuff(t, elapsed, gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{loadingBuffDef()})))
		c := srv.Client
		selectOnly(t, c)
		srv.Advance(t, loadingScreen)
		dropOnLoadingScreen(t, srv, c, objID)
		if count, restoreType := skillSaveRow(t, srv, objID, loadingBuffID, 1); count != 1 || restoreType != 1 {
			t.Fatalf("saved rows after the buff ran out on the loading screen = %d (restore_type %d), want its reuse timer alone (restore_type 1)", count, restoreType)
		}
	})
}

// dropOnLoadingScreen closes c, selected but not in the world, and waits
// until its departure's saves have run. The drop happens at the current
// instant: before letting any time pass on a driven clock, it waits in wall
// time until the server has noticed the close, so the lost connection's
// detach delay runs from the drop itself, not from however much time the
// server took to see it, and a tick due a second after the drop never runs
// before its saves.
func dropOnLoadingScreen(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	if err := c.Close(); err != nil {
		t.Fatalf("close the selecting client: %v", err)
	}
	deadline := time.Now().Add(closeNoticeTimeout)
	for {
		p, ok := srv.State.Player(objID)
		if !ok || network.ClientDetached(p) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not notice the selecting client's close within %v", closeNoticeTimeout)
		}
		time.Sleep(100 * time.Microsecond)
	}
	srv.AdvanceUntil(t, "selected character out of the world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	srv.FlushPersistence(t)
}

// closeNoticeTimeout bounds the wall time the server may take to notice a
// closed client.
const closeNoticeTimeout = 10 * time.Second
