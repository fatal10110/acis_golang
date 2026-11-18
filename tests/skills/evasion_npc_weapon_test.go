package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPhysicalSkillFromArmedMonsterIsNeverEvaded pins the caster side of
// the physical-skill evasion gate: only an equipped weapon item lets the
// target roll P_SKILL_EVASION, and a monster's template right-hand weapon is
// no item. A monster holding a sword casts a PDAM at a player who evades
// every physical skill; the hit lands and the player is not told it avoided
// the attack.
func TestPhysicalSkillFromArmedMonsterIsNeverEvaded(t *testing.T) {
	t.Parallel()
	const skillID = modelskill.ID(9107)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	tmpl := realDamageCasterTemplate(100)
	tmpl.RightHand = fixtureSwordID
	caster, aiCtl := srv.SpawnCastingHostileNPC(t, tmpl, modelskill.NewTable([]modelskill.Definition{{
		ID: skillID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, SkillType: "PDAM",
		CastRange: 150, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		Power: 1_000_000,
	}}))
	if got := caster.AttackType(); got != item.WeaponSword {
		t.Fatalf("monster attack type = %v, want the sword it holds", got)
	}
	objID := srv.SoleObjectID(t)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.AddStatFuncs([]effect.Mod{{Stat: stat.PSkillEvasion, Op: effect.OpSet, Value: 100}})
	})
	drainUntilQuiet(t, srv.Client)
	maxHP := srv.PlayerCurrentHP(t, objID)

	target := worldCombatant(t, srv, objID)
	if !caster.Queue().Post(func() { aiCtl.Cast(target, modelskill.Ref{ID: skillID, Level: 1}) }) {
		t.Fatal("post npc cast: queue closed")
	}
	srv.Advance(t, 2*time.Second)
	srv.Settle(t)
	if hp := srv.PlayerCurrentHP(t, objID); hp >= maxHP {
		t.Fatalf("player HP after the monster's PDAM = %d of %d, want the hit to land", hp, maxHP)
	}
	assertNoSystemMessage(t, srv.Client.ReadQueued(), serverpackets.SystemMessageAvoidedS1Attack)
}
