package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// emptyAuraSelfBuff is a physical aura too narrow to reach anyone, carrying
// a self buff.
const emptyAuraSelfBuff = modelskill.ID(9321)

func emptyAuraSelfBuffDef() modelskill.Definition {
	return modelskill.Definition{
		ID: emptyAuraSelfBuff, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetAura,
		Radius: 10, Offensive: true, SkillType: "PDAM", Power: 1, HitTime: 500, StaticHitTime: true,
		SelfEffects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, Self: true}},
	}
}

// TestPlayerAuraOnNobodyRunsItsHandler (CreatureCast.onMagicHitTimer,
// callSkill; Pdam.useSkill): a player's aura whose launch found no one still
// runs its skill handler with no targets, so its self buff lands on the
// caster and its soulshot is spent.
func TestPlayerAuraOnNobodyRunsItsHandler(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootArmedWith(t, []modelskill.Definition{emptyAuraSelfBuffDef()},
		procWeaponCatalog(t, func(*item.WeaponDetail) {}), func(int) int { return 0 },
		[]int32{fixtureSwordID}, int(emptyAuraSelfBuff))
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetChargedShot(item.ShotSoul, true) })

	c.Send(encodeRequestMagicSkillUse(int32(emptyAuraSelfBuff), false, false))
	readUntil(t, c, serverpackets.OpcodeMagicSkillUse)
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)

	if held := liveHeldSkillIDs(t, srv, objID); !slices.Contains(held, int32(emptyAuraSelfBuff)) {
		t.Fatalf("caster effects = %v, want the aura's self buff %d", held, emptyAuraSelfBuff)
	}
	var charged bool
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { charged = pc.ChargedShot(item.ShotSoul) })
	if charged {
		t.Fatal("soulshot still charged after the aura hit no one, want it spent")
	}
}

// TestNPCAuraOnNobodyRunsItsHandler is the same for a monster's AI cast: its
// aura on no one lands its self buff on the monster.
func TestNPCAuraOnNobodyRunsItsHandler(t *testing.T) {
	t.Parallel()
	srv, c, _ := bootHitPhaseCaster(t, modelskill.Definition{
		ID: npcStrikeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "DUMMY",
	})
	hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1_000_000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{emptyAuraSelfBuffDef()}))
	drainUntilQuiet(t, c)

	onNPCQueue(t, hostile, func() { aiCtl.Cast(hostile, modelskill.Ref{ID: emptyAuraSelfBuff, Level: 1}) })
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)

	var held []modelskill.ID
	onNPCQueue(t, hostile, func() {
		for _, e := range hostile.EffectList().All() {
			held = append(held, e.Skill.ID)
		}
	})
	if !slices.Contains(held, emptyAuraSelfBuff) {
		t.Fatalf("monster effects = %v, want the aura's self buff %d", held, emptyAuraSelfBuff)
	}
}
