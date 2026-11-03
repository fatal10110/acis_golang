package cast

import (
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// spellActor is a skill target known by its object id, with an optional
// summon; every other method panics.
type spellActor struct {
	skilltarget.Actor
	id     int32
	summon skilltarget.Actor
}

func (a *spellActor) ObjectID() int32 { return a.id }

func (a *spellActor) Summon() (skilltarget.Actor, bool) { return a.summon, a.summon != nil }

// spellNPC is an NPC known by its object id; every other method panics.
type spellNPC struct {
	attackable.Combatant
	id int32
}

func (n *spellNPC) ObjectID() int32 { return n.id }

// TestSpellUnseen pins the reference's skip rule for the NPCs around a
// player's skill (CreatureCast.callSkill, Weapon.castSkillOnMagic): a skill
// on one target goes unseen when that target is the player's summon, and,
// by the target NPC itself, when it is neither offensive nor a debuff. A
// skill on several targets is seen by everyone.
func TestSpellUnseen(t *testing.T) {
	summon := &spellActor{id: 2}
	player := &spellActor{id: 1, summon: summon}
	npcA, npcB := &spellActor{id: 10}, &spellNPC{id: 11}
	selfA := &spellNPC{id: 10}
	good := modelskill.Definition{}
	offensive := modelskill.Definition{Offensive: true}
	debuff := modelskill.Definition{Debuff: true}
	for _, tc := range []struct {
		name    string
		def     modelskill.Definition
		targets []skilltarget.Actor
		npc     *spellNPC
		unseen  bool
	}{
		{"good on the summon", good, []skilltarget.Actor{summon}, npcB, true},
		{"offensive on the summon", offensive, []skilltarget.Actor{summon}, npcB, true},
		{"good on the npc, by itself", good, []skilltarget.Actor{npcA}, selfA, true},
		{"good on the npc, by another", good, []skilltarget.Actor{npcA}, npcB, false},
		{"offensive on the npc, by itself", offensive, []skilltarget.Actor{npcA}, selfA, false},
		{"debuff on the npc, by itself", debuff, []skilltarget.Actor{npcA}, selfA, false},
		{"good on the player", good, []skilltarget.Actor{player}, selfA, false},
		{"good on two, the summon among them", good, []skilltarget.Actor{summon, npcA}, selfA, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := spellUnseen(player, tc.def, tc.targets, tc.npc); got != tc.unseen {
				t.Fatalf("spellUnseen = %v, want %v", got, tc.unseen)
			}
		})
	}
	t.Run("no summon", func(t *testing.T) {
		alone := &spellActor{id: 1}
		if spellUnseen(alone, offensive, []skilltarget.Actor{npcA}, npcB) {
			t.Fatal("an offensive skill on an npc went unseen by another npc")
		}
	})
}
