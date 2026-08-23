package combat

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// scriptedClient aliases the wired client handle every scenario drives.
type scriptedClient = testsupport.ScriptedClient

// hostileHandle is the fixture monster handle suites assert against.
type hostileHandle = npc.Hostile

// hostileX/Y/Z mirrors the fixture NPC spawn point every scenario targets.
const (
	hostileX = 60
	hostileY = 20
	hostileZ = 30

	// readQuietWindow is how long a stream must stay silent before a
	// drainUntilQuiet-style wait accepts quietness.
	readQuietWindow = 700 * time.Millisecond
)

// mustRead reads the next frame or fails the test.
func mustRead(t *testing.T, c *scriptedClient, what string) []byte {
	t.Helper()
	frame := c.Read()
	if frame == nil {
		t.Fatalf("%s never arrived", what)
	}
	return frame
}

// combatPersistence builds the skill layer over the shared MariaDB, exactly
// like production wiring.
func combatPersistence(t testing.TB, defs []modelskill.Definition) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))
}

// seedKnownSkill persists a known skill level for objID before the client
// selects the character.
func seedKnownSkill(t *testing.T, srv *gameservertest.Server, objID int32, skillID, level int) {
	t.Helper()
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, skillID, level); err != nil {
		t.Fatalf("seed known skill %d: %v", skillID, err)
	}
}

// spawnRewardedNPC spawns the fixture monster and points its template's
// kill rewards at the given amounts, so the death chain pays real exp/SP.
func spawnRewardedNPC(t *testing.T, srv *gameservertest.Server, rewardExp, rewardSp float64) *hostileHandle {
	t.Helper()
	hostile := srv.SpawnHostileNPC(t)
	hostile.Instance.Template.RewardExp = rewardExp
	hostile.Instance.Template.RewardSp = rewardSp
	return hostile
}

// targetHostile clicks the fixture monster and consumes the click's reply
// sequence: ValidateLocation, MyTargetSelected, and the monster's full-health
// StatusUpdate. It returns the monster's max HP as reported.
func targetHostile(t *testing.T, c *scriptedClient, hostileID int32) int32 {
	t.Helper()
	c.Send(encodeAction(hostileID, hostileX, hostileY, hostileZ, false))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeValidateLocation, "click ValidateLocation")
	frame = c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	if got := wireReader(frame[1:]).ReadInt32(); got != hostileID {
		t.Fatalf("MyTargetSelected object id = %d, want %d", got, hostileID)
	}
	frame = c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "target StatusUpdate")
	r := wireReader(frame[1:])
	if got := r.ReadInt32(); got != hostileID {
		t.Fatalf("StatusUpdate object id = %d, want %d", got, hostileID)
	}
	if count := r.ReadInt32(); count != 2 {
		t.Fatalf("StatusUpdate count = %d, want 2", count)
	}
	maxHP := int32(0)
	for range 2 {
		typ, val := r.ReadInt32(), r.ReadInt32()
		switch typ {
		case int32(serverpackets.StatusMaxHP):
			maxHP = val
		case int32(serverpackets.StatusCurrentHP):
			if val != maxHP && maxHP != 0 {
				t.Fatalf("StatusUpdate CUR_HP = %d before any damage, want MAX_HP %d", val, maxHP)
			}
		default:
			t.Fatalf("StatusUpdate type = %d, want MAX_HP/CUR_HP", typ)
		}
	}
	return maxHP
}

// assertTargetHPStatus asserts a StatusUpdate reporting the target's MAX_HP
// and a CUR_HP strictly below it — the visible proof a swing landed.
func assertTargetHPStatus(t *testing.T, frame []byte, objectID, maxHP int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "target StatusUpdate")
	r := wireReader(frame[1:])
	if got := r.ReadInt32(); got != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", got, objectID)
	}
	count := r.ReadInt32()
	var sawCur bool
	curHP := int32(-1)
	for i := int32(0); i < count; i++ {
		typ, val := r.ReadInt32(), r.ReadInt32()
		switch typ {
		case int32(serverpackets.StatusMaxHP):
			if val != maxHP {
				t.Fatalf("StatusUpdate MAX_HP = %d, want %d", val, maxHP)
			}
		case int32(serverpackets.StatusCurrentHP):
			sawCur = true
			curHP = val
		}
	}
	if !sawCur || curHP >= maxHP {
		t.Fatalf("StatusUpdate current HP = %d, want less than max HP %d after a landed hit", curHP, maxHP)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
}

// assertAutoAttackStart asserts the next frame is AutoAttackStart naming the
// given attacker.
func assertAutoAttackStart(t *testing.T, c *scriptedClient, attackerID int32) {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeAutoAttackStart, "AutoAttackStart")
	if got := wireReader(frame[1:]).ReadInt32(); got != attackerID {
		t.Fatalf("AutoAttackStart object id = %d, want %d", got, attackerID)
	}
}

// assertAttackBy asserts the next frame is an Attack whose attacker is the
// given object id.
func assertAttackBy(t *testing.T, c *scriptedClient, attackerID int32) []byte {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeAttack, "Attack")
	r := wireReader(frame[1:])
	if got := r.ReadInt32(); got != attackerID {
		t.Fatalf("Attack attacker id = %d, want %d", got, attackerID)
	}
	return frame
}
