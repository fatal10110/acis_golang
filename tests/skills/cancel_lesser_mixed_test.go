package skills

import (
	"slices"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// speedUpBlockerDef is a same-stack debuff shaped like Block Wind Walk (1359):
// it shares Wind Walk's "speed_up" stack type at a higher order.
func speedUpBlockerDef(id modelskill.ID, duration int) modelskill.Definition {
	return modelskill.Definition{
		ID:            id,
		Level:         1,
		Activation:    modelskill.ActivationActive,
		Target:        modelskill.TargetSelf,
		HitTime:       0,
		StaticHitTime: true,
		StaticReuse:   true,
		SkillType:     "DEBUFF",
		Debuff:        true,
		IgnoreResists: true,
		Effects:       []modelskill.EffectTemplate{{Name: "Debuff", Time: duration, Icon: true, StackType: "speed_up", StackOrder: 100}},
	}.WithExtra(modelskill.Extra{EffectType: "DEBUFF", EffectPower: 100})
}

func playerEffectList(t *testing.T, srv *gameservertest.Server, objID int32) *effect.List {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	holder, ok := obj.(interface{ EffectList() *effect.List })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no EffectList", objID, obj)
	}
	return holder.EffectList()
}

// TestMixedPolarityCancelLesserDropsBuffVictim covers #4692: a buff that a
// stronger same-stack debuff cancels (Wind Walk, then Block Wind Walk) leaves
// the buff list at once. It keeps no flag, and once the debuff wears off the
// list is empty; the cancelled buff's own expiry later sends nothing.
func TestMixedPolarityCancelLesserDropsBuffVictim(t *testing.T) {
	t.Parallel()
	victim := stackedBuffDef(201, 1, 6)
	victim.Effects[0].Name = "SilentMove"
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{victim, speedUpBlockerDef(301, 2)})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 201, 1)
	seedKnownSkill(t, srv, objID, 301, 1)
	startInWorld(t, c)
	list := playerEffectList(t, srv, objID)

	if icons := buffSlotIDs(castSlotBuff(t, srv, objID, 201)); !slices.Equal(icons, []int32{201}) {
		t.Fatalf("icons after buff = %v, want [201]", icons)
	}
	if !list.IsAffected(effect.FlagSilentMove) {
		t.Fatal("buff flag not set after the buff landed")
	}

	if icons := buffSlotIDs(castSlotBuff(t, srv, objID, 301)); !slices.Equal(icons, []int32{301}) {
		t.Fatalf("icons after same-stack debuff = %v, want [301]", icons)
	}
	if ids := liveHeldSkillIDs(t, srv, objID); !slices.Equal(ids, []int32{301}) {
		t.Fatalf("held effects after same-stack debuff = %v, want buff victim dropped [301]", ids)
	}
	if list.IsAffected(effect.FlagSilentMove) {
		t.Fatal("cancelled buff victim still sets its flag")
	}

	srv.Advance(t, 2200*time.Millisecond)
	srv.TickEffects()
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1HasWornOff, 301, 1)
	if entries := readAbnormalStatusUpdateEntries(t, c); len(entries) != 0 {
		t.Fatalf("icons after debuff expiry = %v, want none", buffSlotIDs(entries))
	}
	drainUntilQuiet(t, c)
	if ids := liveHeldSkillIDs(t, srv, objID); len(ids) != 0 {
		t.Fatalf("held effects after debuff expiry = %v, want none", ids)
	}

	// Past the cancelled buff's own duration: nothing is left to expire.
	srv.Advance(t, 4*time.Second)
	srv.TickEffects()
	if frames := c.ReadQueued(); len(frames) != 0 {
		t.Fatalf("frames at the cancelled buff's own expiry = %d (first opcode %#x), want none", len(frames), frames[0][0])
	}
	if ids := liveHeldSkillIDs(t, srv, objID); len(ids) != 0 || list.Flags() != 0 {
		t.Fatalf("held effects = %v, flags = %#x after the cancelled buff's own expiry; want none", ids, list.Flags())
	}
}

// TestMixedPolarityCancelLesserVictimFreesBuffSlot covers #4692's slot
// count: a buff cancelled by a same-stack debuff no longer takes a buff slot,
// so the next buff fits at the cap without evicting the oldest one.
func TestMixedPolarityCancelLesserVictimFreesBuffSlot(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithMaxBuffsAmount(2),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			slotBuffDef(304), stackedBuffDef(305, 1, 60), speedUpBlockerDef(306, 60), slotBuffDef(308),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, id := range []int{304, 305, 306, 308} {
		seedKnownSkill(t, srv, objID, id, 1)
	}
	startInWorld(t, c)

	castSlotBuff(t, srv, objID, 304)
	castSlotBuff(t, srv, objID, 305)
	castSlotBuff(t, srv, objID, 306)
	if ids := liveHeldSkillIDs(t, srv, objID); !slices.Equal(ids, []int32{304, 306}) {
		t.Fatalf("held effects after same-stack debuff = %v, want [304 306]", ids)
	}

	castSlotBuff(t, srv, objID, 308)
	ids := liveHeldSkillIDs(t, srv, objID)
	slices.Sort(ids)
	if !slices.Equal(ids, []int32{304, 306, 308}) {
		t.Fatalf("held effects after a new buff at the cap = %v, want nothing evicted [304 306 308]", ids)
	}
}
