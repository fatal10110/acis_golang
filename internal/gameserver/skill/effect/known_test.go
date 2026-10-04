package effect

import "testing"

// referenceEffectNames are the <effect name> values the reference can build:
// every class Effect<name> in net.sf.l2j.gameserver.skills.effects with the
// (EffectTemplate, L2Skill, Creature, Creature) constructor
// EffectTemplate.java:68-84 looks up.
var referenceEffectNames = []string{
	"AbortCast",
	"Betray",
	"BigHead",
	"BlockBuff",
	"BlockDebuff",
	"Bluff",
	"Buff",
	"Cancel",
	"CancelDebuff",
	"ChameleonRest",
	"ChanceSkillTrigger",
	"CharmOfCourage",
	"CharmOfLuck",
	"ClanGate",
	"Confusion",
	"DamOverTime",
	"Debuff",
	"Distrust",
	"FakeDeath",
	"Fear",
	"Fusion",
	"Grow",
	"Heal",
	"HealOverTime",
	"ImmobileUntilAttacked",
	"ImobileBuff",
	"ImobilePetBuff",
	"IncreaseCharges",
	"Invincible",
	"ManaDamOverTime",
	"ManaHeal",
	"ManaHealOverTime",
	"Mute",
	"Negate",
	"NoblesseBless",
	"Paralyze",
	"Petrification",
	"PhoenixBless",
	"PhysicalMute",
	"PolearmTargetSingle",
	"ProtectionBlessing",
	"RandomizeHate",
	"Recovery",
	"Relax",
	"RemoveTarget",
	"Root",
	"Seed",
	"Signet",
	"SignetAntiSummon",
	"SignetMDam",
	"SignetNoise",
	"SilenceMagicPhysical",
	"SilentMove",
	"Sleep",
	"Spoil",
	"Stun",
	"StunSelf",
	"TargetMe",
	"ThrowUp",
}

// TestKnownMatchesReferenceEffects checks Known against the reference's
// effect classes: every one is known, and names the reference cannot build
// are not.
func TestKnownMatchesReferenceEffects(t *testing.T) {
	t.Parallel()
	for _, name := range referenceEffectNames {
		if !Known(name) {
			t.Errorf("Known(%q) = false, want true", name)
		}
	}
	if got, want := len(coreKinds), len(referenceEffectNames); got != want {
		t.Errorf("coreKinds holds %d names, reference has %d", got, want)
	}
	// EffectTemplate exists but has no effect constructor; class names are
	// case-sensitive; a qualified name is prefixed with "Effect" again.
	for _, name := range []string{"", "Template", "buff", "BUFF", "Bogus", "net.sf.l2j.gameserver.skills.effects.EffectBuff"} {
		if Known(name) {
			t.Errorf("Known(%q) = true, want false", name)
		}
	}
}
