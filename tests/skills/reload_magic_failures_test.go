package skills

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestReloadConfigMagicFailuresReachesNextCast: a server booted with
// MagicFailures off rolls no resist (TestBootMagicFailuresOffSkipsResistRoll);
// once //reload config reads it on, the next MDAM cast rolls magic success
// again, as the reference reads the setting on each cast. The first roll
// fails and the second succeeds, so the cast lands at half damage and the
// caster reads ATTACK_FAILED.
func TestReloadConfigMagicFailuresReachesNextCast(t *testing.T) {
	t.Parallel()
	const gmLevel = 7
	adminData, err := admin.NewData([]admin.AccessLevel{{Level: 0}, {Level: gmLevel, IsGM: true, GiveDamage: true}},
		[]admin.Command{{Name: "admin_reload", AccessLevel: gmLevel}})
	if err != nil {
		t.Fatal(err)
	}
	reloaded := network.PlayerConfig{MagicFailures: true, RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithMagicFailures(false),
		gameservertest.WithAdmin(adminData),
		gameservertest.WithDataReloads(network.DataReloads{Config: func() (network.PlayerConfig, error) { return reloaded, nil }}),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 45, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				SkillType: "MDAM", Power: 1_000_000,
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", gmLevel, objID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	seedKnownSkill(t, srv, objID, 45, 1)
	startInWorld(t, c)

	worldObj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	caster, ok := worldObj.(interface{ SetRollSource(func(int) int) })
	if !ok {
		t.Fatalf("world player %d = %T, want SetRollSource", objID, worldObj)
	}
	magicRolls := 0
	caster.SetRollSource(func(n int) int {
		if n == 10000 {
			magicRolls++
			if magicRolls == 1 {
				return 0
			}
			return 500
		}
		if n <= 0 {
			return 0
		}
		return n - 1
	})

	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	maxHP := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	reloadConfigAndWait(t, c)

	c.Send(encodeRequestMagicSkillUse(45, false, false))
	readCastStartFrames(t, c, objID, 45, 1, 500, 60_000, hostile.ObjectID())
	srv.AdvanceUntil(t, "MDAM damage", func() bool { return hostile.CurrentHP() < maxHP })
	findSystemMessage(t, c, int32(serverpackets.SystemMessageAttackFailed))
	if magicRolls != 2 {
		t.Fatalf("magic-success rolls = %d, want 2 with MagicFailures reloaded on", magicRolls)
	}
	drainUntilQuiet(t, c)
}

// reloadConfigAndWait runs //reload config and reads up to a manor-list
// barrier sent after it, so the reload is in force when it returns.
func reloadConfigAndWait(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	frames := testsupport.SyncBarrierFrames(t, c, func() {
		cmd := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
		cmd.WriteString("reload config")
		c.Send(cmd.Bytes())
		barrier := wire.NewPacketWriter(clientpackets.OpcodeExtended)
		barrier.WriteUint16(clientpackets.OpcodeRequestManorList)
		c.Send(barrier.Bytes())
	}, serverpackets.OpcodeExtended)
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != int32(serverpackets.SystemMessageS1) || r.ReadInt32() != 1 || r.ReadInt32() != int32(serverpackets.SystemMessageParamText) {
			continue
		}
		if r.ReadString() == "Configs files have been reloaded." {
			return
		}
	}
	t.Fatalf("//reload config frames = %x, want its reloaded text", testsupport.FrameOpcodes(frames))
}
