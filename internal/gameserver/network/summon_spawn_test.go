package network

import (
	"testing"

	"github.com/rs/zerolog"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
)

// TestWireSummonAIForwardsManaFieldsToOwner pins the second half of issue
// #2350's scope: a summon's OnHitResult copies ManaDamageMissed and
// ManaDrains through to the owner alongside the existing
// AttackFailed/Lethals/MagicResists fields, and still drops
// OpponentMPReduced (Manadam.java:72 gates it `creature instanceof Player`,
// and a Summon isn't one).
func TestWireSummonAIForwardsManaFieldsToOwner(t *testing.T) {
	ownerFrames := &testsupport.FrameCapture{}
	targetFrames := &testsupport.FrameCapture{}
	owner := newTestLivePlayer(t, 100, ownerFrames)
	target := newTestLivePlayer(t, 200, targetFrames)

	state := world.New()
	state.AddPlayer(owner)
	state.AddPlayer(target)

	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID:       300,
		Owner:          owner,
		NPCID:          1,
		Name:           "Servitor",
		OwnerInventory: owner.Inventory(),
		Stats:          summon.CombatStats{MaxHP: 100, MaxMP: 100},
	})
	if err != nil {
		t.Fatalf("NewServitor() error: %v", err)
	}

	l := &GameClientLink{world: state, log: zerolog.Nop()}
	aiController := l.wireSummonAI(servitor)

	aiController.OnHitResult(actorcast.EffectResult{
		ManaDamageMissed:  1,
		ManaDrains:        []handlerskill.ManaDrain{{TargetID: 200, CasterName: "Servitor", MP: 15}},
		OpponentMPReduced: []int32{999},
	})

	ownerGot := ownerFrames.Frames()
	if len(ownerGot) != 1 {
		t.Fatalf("owner frame count = %d, want 1 (MISSED_TARGET only, OpponentMPReduced must not forward)", len(ownerGot))
	}
	assertStaticSystemMessageFrame(t, ownerGot[0], serverpackets.SystemMessageMissedTarget)

	targetGot := targetFrames.Frames()
	if len(targetGot) != 1 {
		t.Fatalf("target frame count = %d, want 1 (ManaDrain)", len(targetGot))
	}
	assertSystemMessageStringNumberFrame(t, targetGot[0], serverpackets.SystemMessageS2MPHasBeenDrainedByS1, "Servitor", 15)
}

// TestWireSummonAIForwardsDodgeAndCounterattackToTarget pins issue #2353: a
// summon's OnHitResult copies Dodges and Counterattacks through to
// sendSkillHandlerResult, which resolves the target-addressed messages
// (AVOIDED_S1_ATTACK, COUNTERED_S1_ATTACK) by AttackerID/DefenderID
// independent of the owner argument (Blow.java:49-50,85-86 gate those on
// the target being a Player, not the caster). The caster-addressed halves
// (S1_DODGES_ATTACK, S1_PERFORMING_COUNTERATTACK) must still not reach the
// owner: the summon itself, not the owner, is the attacker, and it is never
// resolvable as a livePlayer.
func TestWireSummonAIForwardsDodgeAndCounterattackToTarget(t *testing.T) {
	ownerFrames := &testsupport.FrameCapture{}
	targetFrames := &testsupport.FrameCapture{}
	owner := newTestLivePlayer(t, 100, ownerFrames)
	target := newTestLivePlayer(t, 200, targetFrames)

	state := world.New()
	state.AddPlayer(owner)
	state.AddPlayer(target)

	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID:       300,
		Owner:          owner,
		NPCID:          1,
		Name:           "Servitor",
		OwnerInventory: owner.Inventory(),
		Stats:          summon.CombatStats{MaxHP: 100, MaxMP: 100},
	})
	if err != nil {
		t.Fatalf("NewServitor() error: %v", err)
	}

	l := &GameClientLink{world: state, log: zerolog.Nop()}
	aiController := l.wireSummonAI(servitor)

	aiController.OnHitResult(actorcast.EffectResult{
		Dodges: []handlerskill.Dodge{
			{AttackerID: 300, AttackerName: "Servitor", DefenderID: 200, DefenderName: "Target"},
		},
		Counterattacks: []handlerskill.Counterattack{
			{AttackerID: 300, AttackerName: "Servitor", DefenderID: 200, DefenderName: "Target"},
		},
	})

	if ownerGot := ownerFrames.Frames(); len(ownerGot) != 0 {
		t.Fatalf("owner frame count = %d, want 0 (summon is not a livePlayer attacker, caster-addressed halves must not forward)", len(ownerGot))
	}

	targetGot := targetFrames.Frames()
	if len(targetGot) != 2 {
		t.Fatalf("target frame count = %d, want 2 (AVOIDED_S1_ATTACK, COUNTERED_S1_ATTACK)", len(targetGot))
	}
	assertSystemMessageStringFrame(t, targetGot[0], serverpackets.SystemMessageCounteredS1Attack, "Servitor")
	assertSystemMessageStringFrame(t, targetGot[1], serverpackets.SystemMessageAvoidedS1Attack, "Servitor")
}

// TestWireSummonAIForwardsOnlyUnconditionalResistedToOwner pins issue #2354:
// a summon's OnHitResult must forward only the unconditional skill-level
// Resisted entries (Mdam.java:69, Blow.java:74, Manadam.java:55 —
// creature.sendPacket with no `instanceof Player` gate, reaching the owner
// via Summon.sendPacket's unconditional forwarding). The generic per-effect
// L2Skill.getEffects resist (gated `effector instanceof Player`, never true
// for a Summon) must not reach the owner.
func TestWireSummonAIForwardsOnlyUnconditionalResistedToOwner(t *testing.T) {
	ownerFrames := &testsupport.FrameCapture{}
	owner := newTestLivePlayer(t, 100, ownerFrames)

	state := world.New()
	state.AddPlayer(owner)

	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID:       300,
		Owner:          owner,
		NPCID:          1,
		Name:           "Servitor",
		OwnerInventory: owner.Inventory(),
		Stats:          summon.CombatStats{MaxHP: 100, MaxMP: 100},
	})
	if err != nil {
		t.Fatalf("NewServitor() error: %v", err)
	}

	l := &GameClientLink{world: state, log: zerolog.Nop()}
	aiController := l.wireSummonAI(servitor)

	aiController.OnHitResult(actorcast.EffectResult{
		Resisted: []handlerskill.Resisted{
			{TargetName: "Orc", SkillID: 1, SkillLevel: 1, Unconditional: true},
			{TargetName: "Orc", SkillID: 2, SkillLevel: 1, Unconditional: false},
		},
	})

	ownerGot := ownerFrames.Frames()
	if len(ownerGot) != 1 {
		t.Fatalf("owner frame count = %d, want 1 (only the unconditional Resisted entry forwards)", len(ownerGot))
	}
	assertSystemMessageStringSkillNameFrame(t, ownerGot[0], serverpackets.SystemMessageS1ResistedYourS2, "Orc", 1, 1)
}
