//go:build integration

package network

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestGameClientLinkEnterWorldRestoresPersistedSkillState(t *testing.T) {
	store := newMemorySkillSaveStore()
	now := time.Now().Truncate(time.Millisecond)
	p := skillstate.NewPersistenceWithClock(store, skillTable(
		modelskill.Definition{ID: 1040, Level: 3, Effects: []modelskill.EffectTemplate{{Name: "Buff"}}},
	), func() time.Time { return now })
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, p, nil, 0)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := sqlSoleObjectID(t, chars)
	store.seed(objID, 0, []effect.SaveRow{{
		Skill:         skillRef(1040, 3),
		EffectCount:   2,
		EffectCurTime: 15,
		ReuseDelay:    60_000,
		SystemTime:    now.Add(60 * time.Second).UnixMilli(),
		RestoreType:   effect.RestoreTypeEffect,
		BuffIndex:     1,
	}})

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())

	// ReplayEffects adds the restored buff to the live effect list between
	// HennaInfo and EtcStatusUpdate (mirroring EnterWorld.java:100's
	// player.updateEffectIcons() call), so List.Add's notifyAbnormalUpdate
	// hook fires one AbnormalStatusUpdate frame there that a plain
	// readEnterWorldBurst call doesn't expect.
	want := []byte{
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeAbnormalStatusUpdate,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeActionFailed,
	}
	for i, opcode := range want {
		frame := c.read()
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
	}

	live, ok := state.Player(objID)
	if !ok {
		t.Fatalf("player %d not found in world state after EnterWorld", objID)
	}
	char := live.(*livePlayer).Character
	if !char.SkillDisabled(1040*256 + 3) {
		t.Fatal("EnterWorld did not restore the reuse timer")
	}
	// ReplayEffects moves every staged entry onto the live effect list and
	// clears the registry: Save now reads that live list, so nothing should
	// linger in the registry to go stale.
	if effects := char.ActiveSkillEffects(); len(effects) != 0 {
		t.Fatalf("registry after ReplayEffects = %+v, want cleared", effects)
	}
	if got := store.rowsFor(objID, 0); len(got) != 0 {
		t.Fatalf("persisted rows after EnterWorld = %+v, want consumed", got)
	}
	if level, ok := char.EffectList().ActiveBySkillID(1040); !ok || level != 3 {
		t.Fatalf("EnterWorld restored effect in live effect list = level %d, ok %v, want level 3", level, ok)
	}
}

func TestGameClientLinkEnterWorldSendsKnownSkillList(t *testing.T) {
	store := newMemorySkillSaveStore()
	p := skillstate.NewPersistence(store, skillTable(
		modelskill.Definition{ID: 248, Level: 1, Activation: modelskill.ActivationPassive},
	), store)
	c, chars, _, _, _, _ := newLinkedSQLGameClient(t, p, nil, 0)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := sqlSoleObjectID(t, chars)
	store.seedKnown(objID, 0, player.SkillLevels{248: 1})

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c, false)

	skillList := frames[5]
	if skillList[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("EnterWorld skill frame opcode = %#x, want SkillList", skillList[0])
	}
	r := wire.NewReader(skillList[1:])
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("SkillList count = %d, want 1", count)
	}
	if passive, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); passive != 1 || level != 1 || id != 248 {
		t.Fatalf("SkillList entry = passive %d level %d id %d, want passive 1 level 1 id 248", passive, level, id)
	}
}

// TestGameClientLinkLogoutPersistsSkillState proves a buff applied during
// the live session — cast for real through the client protocol, the exact
// path issue #1234 found dead — survives logout. Before the fix, Save read
// only a registry that RestoreSkillEffect was the sole writer of, so a live
// cast like this one (which lands via effect.List.Add, never through
// RestoreSkillEffect) never persisted anything.
func TestGameClientLinkLogoutPersistsSkillState(t *testing.T) {
	store := newMemorySkillSaveStore()
	def := modelskill.Definition{
		ID: 1204, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 45_000, StaticHitTime: true, StaticReuse: true,
		MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 30}},
	}
	p := skillstate.NewPersistence(store, skillTable(def), store)
	var objID int32
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, p, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{1204: 2})
	}, 1)

	beforeCast := time.Now()
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(1204, false, false))
	c.read() // MagicSkillUse
	c.read() // SystemMessage
	c.read() // SetupGauge
	c.read() // MagicSkillLaunched
	c.read() // StatusUpdate
	afterCast := time.Now()

	c.send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	c.read() // LeaveWorld
	c.read() // AbnormalStatusUpdate, from the buff landing's List.Add
	// detachLivePlayer's Stop() now reaches the cast controller
	// (Player.cleanup -> abortAll(true) -> _cast.stop(), Creature.java:1298-1302),
	// and PlayerCast.stop() sends clientActionFailed unconditionally, cast or
	// no cast in flight (PlayerCast.java:382-387).
	if reply := c.read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("post-logout opcode = %#x, want ActionFailed from detach's unconditional cast-stop ack (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	c.expectClosed()

	got := store.rowsFor(objID, 0)
	if len(got) != 1 {
		t.Fatalf("saved rows = %+v, want exactly one row for the live-cast buff", got)
	}
	row := got[0]
	row.SystemTime = 0    // reuse timestamp depends on real cast wall-clock time; checked separately below
	row.EffectCurTime = 0 // elapsed-since-cast seconds depend on real wall-clock time; checked separately below
	want := effect.SaveRow{
		Skill:         skillRef(1204, 2),
		EffectCount:   2,
		EffectCurTime: 0,
		ReuseDelay:    45_000,
		RestoreType:   effect.RestoreTypeEffect,
		ClassIndex:    0,
		BuffIndex:     1,
	}
	if minSystem, maxSystem := beforeCast.Add(45*time.Second).UnixMilli(), afterCast.Add(45*time.Second).UnixMilli(); got[0].SystemTime < minSystem || got[0].SystemTime > maxSystem {
		t.Fatalf("saved reuse SystemTime = %d, want within [%d, %d] (cast time + 45s)", got[0].SystemTime, minSystem, maxSystem)
	}
	// The buff's 30s period barely started (cast completed a moment before
	// logout), so its saved elapsed time must still be small.
	if got[0].EffectCurTime < 0 || got[0].EffectCurTime > 5 {
		t.Fatalf("saved EffectCurTime = %d, want a small elapsed value near 0", got[0].EffectCurTime)
	}
	if !reflect.DeepEqual(row, want) {
		t.Fatalf("Logout saved rows = %+v, want %+v", got, want)
	}
}
