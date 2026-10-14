package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: L2Skill.getEffects lands no effect on a SiegeFlag; and
// SiegeFlag.reduceCurrentHp returns before both the BASE_UNDER_ATTACK
// warning and the HP loss for a hit from the Swoop Cannon siege summon
// (npc 14839), while any other siege summon hurts it.

const (
	shieldSkillID     = 1040
	siegeGolemNpcID   = 14737
	swoopCannonNpcID  = 14839
	hqEffectCastSpace = 100
)

// TestSiegeFlagTakesNoEffect casts the shipped Shield buff through
// RequestMagicSkillUse on a siege headquarters and on an ordinary folk NPC
// of the same body: it lands on the folk and not on the headquarters.
func TestSiegeFlagTakesNoEffect(t *testing.T) {
	t.Parallel()
	shield := shippedSkill(t, shieldSkillID, 1)
	w := bootHQ(t, hqSetup{known: []modelskill.Definition{shield}})

	plainTmpl := *npc.SiegeFlagTemplate("Plain", false)
	plainTmpl.Type = "Folk"
	plain := w.srv.SpawnFolkNPCAt(t, &plainTmpl, location.Location{X: w.at.X - hqEffectCastSpace, Y: w.at.Y, Z: w.at.Z})
	flag := w.otherFlag(t, "Rivals", hqEffectCastSpace)
	if !flag.StandFor(w.rivals) {
		t.Fatal("the rivals' headquarters did not stand for them")
	}

	for _, tt := range []struct {
		name  string
		npc   *npc.Folk
		lands bool
	}{
		{name: "ordinary folk", npc: plain, lands: true},
		{name: "siege headquarters", npc: flag, lands: false},
	} {
		selectTarget(t, w.c, tt.npc.ObjectID())
		w.c.Send(encodeRequestMagicSkillUse(shieldSkillID, false, false))
		started := collectUntilQuiet(t, w.c)
		if !slices.Contains(opcodes(started), serverpackets.OpcodeMagicSkillUse) {
			t.Fatalf("%s: Shield cast answered %x, want it started", tt.name, opcodes(started))
		}
		// The cast lands, and its reuse delay runs out before the next one:
		// at this caster's casting speed the two take longer than 15s.
		w.srv.Advance(t, 20*time.Second)
		drainUntilQuiet(t, w.c)
		if _, landed := tt.npc.EffectList().ActiveBySkillID(shieldSkillID); landed != tt.lands {
			t.Fatalf("%s: Shield landed = %v, want %v", tt.name, landed, tt.lands)
		}
	}
}

// TestSiegeFlagShrugsOffTheSwoopCannon hits a clan's headquarters with a
// Swoop Cannon and then with a Siege Golem: the cannon's hit neither warns
// the clan nor takes HP; the golem's does both.
func TestSiegeFlagShrugsOffTheSwoopCannon(t *testing.T) {
	t.Parallel()
	w := bootHQ(t, hqSetup{})
	w.putUp(t, buildHQSkillID)
	flag := w.flags()[0]
	siegeSummon := func(npcID int) *summon.Actor {
		s, err := summon.NewServitor(summon.ServitorConfig{
			ObjectID:    w.srv.NewObjectID(),
			NPCID:       npcID,
			Level:       40,
			SiegeSummon: true,
			Stats:       summon.CombatStats{MaxHP: 500, MaxMP: 200},
		})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	warned := func() int {
		n := 0
		for _, id := range drainSystemMessageIDs(t, w.c) {
			if id == serverpackets.SystemMessageBaseUnderAttack {
				n++
			}
		}
		return n
	}

	flag.TakeDamage(10, siegeSummon(swoopCannonNpcID))
	if got := warned(); got != 0 {
		t.Fatalf("Swoop Cannon hit warned %d times, want none", got)
	}
	if got := flag.CurrentHP(); got != 50000 {
		t.Fatalf("headquarters HP after the Swoop Cannon hit = %d, want 50000", got)
	}

	flag.TakeDamage(10, siegeSummon(siegeGolemNpcID))
	if got := warned(); got != 1 {
		t.Fatalf("Siege Golem hit warned %d times, want 1", got)
	}
	if got := flag.CurrentHP(); got != 50000-10 {
		t.Fatalf("headquarters HP after the Siege Golem hit = %d, want %d", got, 50000-10)
	}
}
