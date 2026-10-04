package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// quickPoisonID is a saved ten-tick, 1s-period poison.
const (
	quickPoisonID     = 85
	quickPoisonTicks  = 10
	quickPoisonDamage = 5
)

func quickPoisonDef() modelskill.Definition {
	def := loadingPoisonDef()
	def.ID = quickPoisonID
	def.Effects[0].Count = quickPoisonTicks
	def.Effects[0].Time = 1
	def.Effects[0].Value = quickPoisonDamage
	return def
}

// TestRestoredPoisonTicksOnTheLoadingScreen pins #3266: the reference
// restores a saved effect at character selection and schedules it there
// (Player.restore -> restoreEffects, Player.java:4140-4143, 4701-4764;
// AbstractEffect.scheduleEffect, AbstractEffect.java:272-320), so every
// tick due on the loading screen runs onActionTime() on the character
// before EnterWorld. A 1s poison held 3s on the loading screen has dealt
// three ticks' damage by the EnterWorld UserInfo, and its icon shows the
// seven ticks left. Nothing is sent before the burst, and the damage sends
// no StatusUpdate into it: the burst reads exactly its own frames.
func TestRestoredPoisonTicksOnTheLoadingScreen(t *testing.T) {
	t.Parallel()
	srv, _ := bootSavedPoison(t, quickPoisonDef(), quickPoisonTicks, 0)
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, 3*time.Second)
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurstWithRestoredBuff(t, c)

	entries := readAbnormalStatusUpdateEntriesFromFrame(t, frames[3])
	if len(entries) != 1 || entries[0].SkillID != quickPoisonID {
		t.Fatalf("EnterWorld AbnormalStatusUpdate = %+v, want the restored poison", entries)
	}
	if want := int32(quickPoisonTicks - 3); entries[0].Duration > want || entries[0].Duration < want-1 {
		t.Fatalf("restored poison duration at EnterWorld = %ds, want %ds: three of its ticks ran on the loading screen", entries[0].Duration, want)
	}
	if hp, want := userInfoCurrentHP(t, frames[10]), int32(loadingPoisonHP-3*quickPoisonDamage); hp != want {
		t.Fatalf("EnterWorld UserInfo HP = %d, want %d: three ticks' damage", hp, want)
	}
}
