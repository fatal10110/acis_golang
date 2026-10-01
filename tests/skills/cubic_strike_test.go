package skills

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// stormStrikePower is the Storm Cubic's MDAM power in this fixture.
const stormStrikePower = 37

// TestStormCubicStrikeUsesCubicFormulaAndKeepsOwnerShot drives a Storm
// Cubic's MDAM proc end to end. Cubic.useMdamSkill deals
// Formulas.calcMagicDam(Cubic) damage, 91 / M.Def * power, with no owner
// M.Atk or shot term: the monster loses exactly int(91 / its M.Def * 37)
// HP, although the owner's own M.Atk and charged blessed spiritshot would
// multiply a player cast of the same skill. The owner is told the damage,
// and its blessed spiritshot stays charged: a cubic never spends it.
func TestStormCubicStrikeUsesCubicFormulaAndKeepsOwnerShot(t *testing.T) {
	t.Parallel()
	var nowMS atomic.Int64
	defs := cubicSummonSkills(900*time.Second, 900*time.Second)
	for i := range defs {
		if defs[i].ID == summonStormCubicSkill {
			defs[i].CubicActivationChance = 100
		}
	}
	defs = append(defs, modelskill.Definition{
		ID: stormCubicFireSkill, Level: 1, SkillType: "MDAM", Power: stormStrikePower,
		Target: modelskill.TargetOne, Offensive: true,
	})
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
		gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, summonLifeCubicSkill, 1)
	seedKnownSkill(t, srv, objID, summonStormCubicSkill, 1)
	sword := srv.GiveItem(t, objID, approachSwordID, 1)
	startInWorld(t, c)
	// A shot charge rides on the active weapon.
	c.Send(encodeSignetUseItem(sword))
	srv.Settle(t)
	drainUntilQuiet(t, c)
	// Every owner roll is its best: the cubic always acts, the owner never
	// scores a magic critical, and the magic-success check always passes.
	setPlayerRollSource(t, srv, objID, func(n int) int { return n - 1 })
	summonStormCubic(t, srv, c, objID)

	monster := srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 100000, MDef: 91,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, monster.ObjectID())
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.SetChargedShot(item.ShotBlessedSpirit, true)
		if !pc.BlessedSpiritshotCharged() {
			t.Error("no blessed spiritshot charge on the owner's weapon")
		}
	})
	// Enter attack stance without swinging, so the cubic is the only
	// source of damage.
	obj, _ := srv.State.Player(objID)
	srv.AttackStance.Add(obj.(task.AttackStanceActor))

	srv.AdvanceUntil(t, "the cubic strike", func() bool { return monster.CurrentHP() < monster.MaxHP() })
	mDef := monster.MDef()
	want := int(91 / mDef * stormStrikePower)
	if lost := monster.MaxHP() - monster.CurrentHP(); lost != want {
		t.Fatalf("monster lost %v HP to the cubic, want %d (91 / M.Def %v * power %d)", lost, want, mDef, stormStrikePower)
	}

	dealt := readFrameLog(c).index(func(frame []byte) bool {
		r, ok := systemMessage(frame, serverpackets.SystemMessageYouDidS1Dmg)
		return ok && r.ReadInt32() == 1 && r.ReadInt32() == serverpackets.SystemMessageParamNumber && r.ReadInt32() == int32(want)
	})
	if dealt < 0 {
		t.Fatalf("owner got no YOU_DID_S1_DMG of %d", want)
	}
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if !pc.BlessedSpiritshotCharged() {
			t.Error("the cubic strike spent the owner's blessed spiritshot")
		}
	})
}
