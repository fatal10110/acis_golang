package skills

import (
	"context"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// lethalPoisonHP is the HP a character is saved with under a lethal poison:
// the poison's 5 HP ticks take it to 7, then 2, and the third kills.
const (
	lethalPoisonHP = 12
	// lethalPoisonKill is how long after the restore the killing tick comes
	// due: a poison saved 1s into its 3s period ticks at 2s, 5s and 8s.
	lethalPoisonKill = 8 * time.Second
)

// lethalPoisonDef is loadingPoisonDef shaped like Poison of Death (4082):
// its damage-over-time may kill.
func lethalPoisonDef() modelskill.Definition {
	def := loadingPoisonDef()
	def.ID = 4082
	def.Debuff = true
	def.KillByDOT = true
	return def
}

// bootSavedLethalPoison boots a character saved with lethalPoisonHP HP
// under a saved lethal poison, 1s into its period.
func bootSavedLethalPoison(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	srv, objID := bootSavedPoison(t, lethalPoisonDef(), loadingPoisonTicks, 1)
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE characters SET curHp = ? WHERE obj_Id = ?`, lethalPoisonHP, objID); err != nil {
		t.Fatalf("seed saved HP: %v", err)
	}
	return srv, objID
}

// TestRestoredLethalPoisonKillsOnTheLoadingScreen: a restored poison whose
// damage-over-time may kill runs its ticks from the restore, as every other
// restored effect does (#3394). A loading screen held past its killing tick
// sends nothing; the death it caused there is shown by the EnterWorld
// burst: no poison icon left, zero HP in UserInfo and Die after
// ShortCutInit.
func TestRestoredLethalPoisonKillsOnTheLoadingScreen(t *testing.T) {
	t.Parallel()
	srv, objID := bootSavedLethalPoison(t)
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, lethalPoisonKill+2*time.Second)
	if queued := c.ReadQueued(); len(queued) != 0 {
		t.Fatalf("frame %#x sent on the loading screen, want nothing before the EnterWorld burst", queued[0][0])
	}
	c.Send(encodeEnterWorld())
	frames := readFrameSequence(t, c, []byte{
		serverpackets.OpcodeSendMacroList,
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeAbnormalStatusUpdate,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeDie,
		serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeActionFailed,
	})

	if entries := readAbnormalStatusUpdateEntriesFromFrame(t, frames[3]); len(entries) != 0 {
		t.Fatalf("EnterWorld AbnormalStatusUpdate after the poison's death = %+v, want the poison gone with the death", entries)
	}
	if hp := userInfoCurrentHP(t, frames[10]); hp != 0 {
		t.Fatalf("EnterWorld UserInfo HP = %d, want 0: the poison killed on the loading screen", hp)
	}
	if !srv.PlayerDead(t, objID) {
		t.Fatal("character alive after EnterWorld, want it dead")
	}
}

// TestRestoredLethalPoisonTicksBeforeItsKillOnlyHurt: a loading screen left
// before the killing tick enters the world alive, with the ticks due by
// then taken from its HP.
func TestRestoredLethalPoisonTicksBeforeItsKillOnlyHurt(t *testing.T) {
	t.Parallel()
	srv, objID := bootSavedLethalPoison(t)
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, lethalPoisonKill-time.Second)
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurstWithRestoredBuff(t, c)

	if hp, want := userInfoCurrentHP(t, frames[10]), int32(lethalPoisonHP-2*loadingPoisonDamage); hp != want {
		t.Fatalf("EnterWorld UserInfo HP = %d, want %d: two ticks came due on the loading screen", hp, want)
	}
	if srv.PlayerDead(t, objID) {
		t.Fatal("character dead after EnterWorld, want it alive")
	}
}

// TestRestoredLethalPoisonDropAfterItsKillSavesTheDeath: a drop on the
// loading screen past the killing tick saves the character dead, with the
// poison gone with the death.
func TestRestoredLethalPoisonDropAfterItsKillSavesTheDeath(t *testing.T) {
	t.Parallel()
	srv, objID := bootSavedLethalPoison(t)
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, lethalPoisonKill+2*time.Second)
	dropOnLoadingScreen(t, srv, c, objID)

	if hp := savedHP(t, srv, objID); hp != 0 {
		t.Fatalf("saved HP after the drop = %g, want 0: the poison killed on the loading screen", hp)
	}
	if count, curTime, ok := savedEffectRow(t, srv, objID, int(lethalPoisonDef().ID)); ok {
		t.Fatalf("saved poison after its kill = count %d time %d, want no effect row", count, curTime)
	}
}
