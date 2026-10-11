package ai

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The wizard scenarios' NPC: a fixture monster of level 1 (a WizardDDMagic2
// id in the reference) bound to wizardCaster, and the DD magic it lists.
const (
	wizardID     = int32(20092)
	wizardDDID   = modelskill.ID(9302)
	wizardMuteID = modelskill.ID(9303)
)

// wizardCaster is a behavior derived from Wizard, as the reference's
// DD-magic wizards are: once Wizard's hooks have run for a hit, it wants
// its DD magic cast on the creature at the top of its hate list, while its
// script memory says it is not waiting out a silence (int slot 0 is 0). It
// stands in for those children, which are ported later, so the scenarios
// see what Wizard leaves in the hate list.
func wizardCaster() script.Script {
	base := wizard.WizardHooks()
	return script.Script{Behavior: true, NPCs: []int32{wizardID}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n := e.NPC
			if n.Int(script.SlotIntAI0) != 0 {
				return
			}
			if mostHated := n.HateList().MostHated(); mostHated != nil {
				dd, _ := n.SkillByType(npc.SkillTypeWShortRangeDDMagic)
				n.AddCastDesire(mostHated, dd, 1000000)
			}
		},
	})}
}

// wizardScripts are the behavior chain down to Wizard, in scripts.xml
// order, and the caster bound to the fixture wizard.
func wizardScripts() ([]script.Listing, script.Catalog) {
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                       defaultnpc.New,
		"script.ai.individual.Monster.MonsterAI":                monster.New,
		"script.ai.individual.Monster.WizardBase.WizardBase":    wizard.Base,
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard": wizard.Wizard,
		"ai.WizardCaster": wizardCaster,
	}
	var list []script.Listing
	for _, path := range []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.WizardBase.WizardBase",
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard",
		"ai.WizardCaster",
	} {
		list = append(list, script.Listing{Path: path})
	}
	return list, catalog
}

// wizardTemplate is the fixture wizard: level 1, listing wizardDDID as its
// short range DD magic.
func wizardTemplate() *npc.Template {
	tmpl := warriorTemplate(wizardID, nil)
	tmpl.Name = "Wizard"
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypeWShortRangeDDMagic: {ID: wizardDDID, Level: 1}}
	return tmpl
}

// bootWizard boots one character in the world beside a fixture wizard 100
// away from it, every script draw 0, with the wizard's DD magic installed.
func bootWizard(t *testing.T) (*warriorWorld, *npc.Hostile) {
	t.Helper()
	list, catalog := wizardScripts()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wizardTemplate()})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{wizardID: script.KindHostile}, list, catalog),
	)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	w := &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
	h := w.spawn(t, wizardID, 100, 0)
	srv.InstallNPCCastSeam(t, h, modelskill.NewTable([]modelskill.Definition{{
		ID: wizardDDID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, Magic: true, SkillType: "MDAM",
		CastRange: 600, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1,
	}}))
	return w, h
}

// ddCastOn returns h's queued DD magic cast desire, and whether it aims at
// target.
func ddCastOn(h *npc.Hostile, target attackable.Combatant) (actorai.Desire, bool) {
	d, ok := desire(h, actorai.IntentionCast)
	return d, ok && d.Skill.ID == wizardDDID && d.FinalTarget != nil && d.FinalTarget.ObjectID() == target.ObjectID()
}

// magicSkillUses returns the MagicSkillUse frames h cast wizardDDID with,
// each as its target's object id.
func magicSkillUses(frames [][]byte, h *npc.Hostile) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, h.ObjectID()) {
		// opcode, caster, target, skill id, then the rest of the cast.
		if modelskill.ID(binary.LittleEndian.Uint32(f[9:13])) == wizardDDID {
			out = append(out, int32(binary.LittleEndian.Uint32(f[5:9])))
		}
	}
	return out
}

// TestWizardCastsItsDDMagicOnItsMostHated ports Wizard's attacked hook,
// every draw 0: a hit of 10 on the level 1 wizard puts the character in its
// empty hate list at the damage formula, 10 / (1 + 7) * 100, plus the
// opening 300; the hate its attacks follow is not touched. The caster
// derived from Wizard then wants its DD magic on the top of the hate list,
// the character, and casts it there at its next think. A second hit adds
// the formula alone.
func TestWizardCastsItsDDMagicOnItsMostHated(t *testing.T) {
	t.Parallel()
	w, h := bootWizard(t)

	w.hit(t, h, 10)
	if got := h.AI().Hates().Hate(w.player); got != 425 {
		t.Fatalf("hate list entry of the character after the first hit = %v, want 425", got)
	}
	if got := h.AggroHate(w.player); got != 0 {
		t.Fatalf("attack hate of the character = %v, want none: Wizard fills its hate list only", got)
	}
	if d, ok := ddCastOn(h, w.player); !ok || d.Weight != 1000000 {
		t.Fatalf("wizard cast desire = %+v (%v), want its DD magic on the character, weight 1000000", d, ok)
	}

	if got := magicSkillUses(w.ticks(t, 2), h); len(got) != 1 || got[0] != w.player.ObjectID() {
		t.Fatalf("wizard DD magic casts = %v, want one on the character %d", got, w.player.ObjectID())
	}

	w.hit(t, h, 10)
	if got := h.AI().Hates().Hate(w.player); got != 550 {
		t.Fatalf("hate list entry of the character after the second hit = %v, want 550", got)
	}
}

// TestSilencedWizardAnswersItsMostHatedOnceFree ports Wizard's 1003 timer,
// every draw 0. A hit on a silenced wizard sets its int slot 0 and arms the
// timer for 10 s, so the caster casts nothing. While the silence lasts the
// timer re-arms; once it is gone the timer drops the wizard's attack
// desires, clears the slot and runs the attacked hook again for its most
// hated with a damage of 100: the hate list gains 100 / 8 * 100, and the
// caster wants its DD magic on the character.
func TestSilencedWizardAnswersItsMostHatedOnceFree(t *testing.T) {
	t.Parallel()
	w, h := bootWizard(t)
	mute, err := effect.New(effect.Skill{ID: wizardMuteID, Level: 1}, modelskill.EffectTemplate{Name: "Mute", Time: 600})
	if err != nil {
		t.Fatalf("effect.New(Mute): %v", err)
	}
	mute.Effector, mute.Effected = h, h
	w.onQueue(t, h, func() { h.EffectList().Add(mute) })
	if !script.NPCOf(h).Muted() {
		t.Fatal("the wizard is not muted")
	}

	w.hit(t, h, 10)
	if got := script.NPCOf(h).Int(script.SlotIntAI0); got != 1 {
		t.Fatalf("silenced wizard int slot 0 = %d, want 1", got)
	}
	if d, ok := desire(h, actorai.IntentionCast); ok {
		t.Fatalf("silenced wizard wants %+v, want no cast", d)
	}

	w.srv.Advance(t, 10*time.Second)
	if d, ok := desire(h, actorai.IntentionCast); ok {
		t.Fatalf("wizard still silenced at its 1003 timer wants %+v, want no cast", d)
	}
	if got := h.AI().Hates().Hate(w.player); got != 425 {
		t.Fatalf("hate list entry while silenced = %v, want 425", got)
	}

	w.onQueue(t, h, func() { h.EffectList().Remove(mute) })
	w.srv.Advance(t, 10*time.Second)
	if got := script.NPCOf(h).Int(script.SlotIntAI0); got != 0 {
		t.Fatalf("freed wizard int slot 0 = %d, want 0", got)
	}
	if got := h.AI().Hates().Hate(w.player); got != 1675 {
		t.Fatalf("hate list entry after the timer's attacked call = %v, want 425 + 1250", got)
	}
	if d, ok := ddCastOn(h, w.player); !ok || d.Weight != 1000000 {
		t.Fatalf("freed wizard cast desire = %+v (%v), want its DD magic on its most hated, weight 1000000", d, ok)
	}
}

// TestWizardForgetsWhoWalksAway ports Wizard's 1002 timer: every 10 s the
// wizard's hate list drops the creatures 1000 or more away, with the cast
// desires aimed at them. A character that jumped 1200 away, still known to
// the wizard, leaves the list empty and the DD magic unwanted at the next
// run, while one 500 away stays.
func TestWizardForgetsWhoWalksAway(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		away int
		kept bool
	}{
		{"near", 500, true},
		{"far", 1200, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w, h := bootWizard(t)
			w.hit(t, h, 10)
			if _, ok := ddCastOn(h, w.player); !ok {
				t.Fatal("the wizard wants no DD magic on the character after a hit")
			}
			x, y, z := h.Position()
			w.instantTeleport(t, location.Location{X: x - row.away, Y: y, Z: z})
			if !h.Knows(w.player) {
				t.Fatal("the wizard no longer knows the character")
			}

			w.srv.Advance(t, 10*time.Second)
			_, wanted := ddCastOn(h, w.player)
			listed := h.AI().Hates().Hate(w.player) != 0
			if wanted != row.kept || listed != row.kept {
				t.Fatalf("character %d away: DD magic wanted %v, in the hate list %v; want both %v", row.away, wanted, listed, row.kept)
			}
		})
	}
}
