package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: EffectProtectionBlessing.onStart returns false. setInUse(true)
// keeps it in use but leaves _startConditionsCorrect false, so its landing
// sends no YOU_FEEL_S1_EFFECT (EffectList.java:768-774) and its onExit
// (Playable.stopProtectionBlessing -> updateAbnormalEffect, Playable.java:
// 277-285) is skipped when it ends (AbstractEffect.java:319). A recast
// displaces it from its stack head with setInUse(false)
// (EffectList.java:756-765), which runs onExit: the player's UserInfo and
// its observers' CharInfo refresh once.

const (
	protectionBlessingSkill = 5182
	pkProtectBuffSkill      = 5183
)

// protectionBlessingSkills declares skill 5182 as the datapack does
// (5100-5199.xml: time 3600, stackType pk_protect, stackOrder 1), made
// self-castable, and a plain buff in the same stack slot as the control.
func protectionBlessingSkills() []modelskill.Definition {
	def := func(id int, name string) modelskill.Definition {
		return modelskill.Definition{
			ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 0, StaticHitTime: true, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: name, Time: 3600, Count: 1, StackType: "pk_protect", StackOrder: 1, Icon: true}},
		}
	}
	return append(killSkillDefs(), def(protectionBlessingSkill, "ProtectionBlessing"), def(pkProtectBuffSkill, "Buff"))
}

// castTwiceFrames casts skillID on the caster twice, with an observer in
// view, and returns the caster's frames from the first cast and both
// clients' frames from the recast.
func castTwiceFrames(t *testing.T, skillID int) (firstSelf, recastSelf, recastObserver [][]byte, casterID int32) {
	t.Helper()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Blessed", 5, 0), gameservertest.WithWantChars(1), gameservertest.WithSkills(combatPersistence(t, protectionBlessingSkills())))
	caster, casterID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, casterID, skillID, 1)
	startInWorld(t, caster)
	srv.SeedCharacterFor(t, "observer", "Observer", 5, 0)
	observer := srv.DialClient(t, "observer", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, caster)

	caster.Send(encodeRequestMagicSkillUse(int32(skillID), false, false))
	firstSelf = readQuiet(caster)
	drainUntilQuiet(t, observer)

	caster.Send(encodeRequestMagicSkillUse(int32(skillID), false, false))
	return firstSelf, readQuiet(caster), readQuiet(observer), casterID
}

func countSystemMessage(frames [][]byte, id int32) int {
	n := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == id {
			n++
		}
	}
	return n
}

func countCharInfo(frames [][]byte, id int32) int {
	n := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeCharInfo && indexOfCharInfo([][]byte{f}, id) == 0 {
			n++
		}
	}
	return n
}

// TestProtectionBlessingRecastRefreshesPlayerAppearanceOnce compares a
// Blessing of Protection with a plain buff in the same stack slot: its
// landing sends no felt message, and its recast adds exactly one UserInfo
// to the caster and one CharInfo to the observer.
func TestProtectionBlessingRecastRefreshesPlayerAppearanceOnce(t *testing.T) {
	t.Parallel()
	var (
		baseFirst, baseSelf, baseObserver, first, self, observer [][]byte
		baseID, id                                               int32
	)
	t.Run("plain buff", func(t *testing.T) {
		baseFirst, baseSelf, baseObserver, baseID = castTwiceFrames(t, pkProtectBuffSkill)
	})
	t.Run("protection blessing", func(t *testing.T) {
		first, self, observer, id = castTwiceFrames(t, protectionBlessingSkill)
	})

	if got := countSystemMessage(baseFirst, serverpackets.SystemMessageYouFeelS1Effect); got != 1 {
		t.Fatalf("plain buff landing felt messages = %d, want 1", got)
	}
	if got := countSystemMessage(first, serverpackets.SystemMessageYouFeelS1Effect); got != 0 {
		t.Fatalf("Blessing of Protection landing felt messages = %d, want 0", got)
	}

	baseUserInfo := countOpcode(baseSelf, len(baseSelf), serverpackets.OpcodeUserInfo)
	if got := countOpcode(self, len(self), serverpackets.OpcodeUserInfo); got != baseUserInfo+1 {
		t.Fatalf("caster UserInfo on recast = %d, want %d (plain buff %d + 1)", got, baseUserInfo+1, baseUserInfo)
	}
	baseCharInfo := countCharInfo(baseObserver, baseID)
	if got := countCharInfo(observer, id); got != baseCharInfo+1 {
		t.Fatalf("observer CharInfo on recast = %d, want %d (plain buff %d + 1)", got, baseCharInfo+1, baseCharInfo)
	}
}
