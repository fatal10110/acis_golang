package skills

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	deletedCastNPC    = int32(20071)
	deletedCastDebuff = modelskill.ID(9311)
	deletedCastNuke   = modelskill.ID(9312)
)

// deletedCastDebuffSkill always lands its debuff on its one target; its
// launch comes 600 ms into the cast and its hit 400 ms later.
func deletedCastDebuffSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: deletedCastDebuff, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, Debuff: true, SkillType: "DEBUFF",
		CastRange: 900, HitTime: 1000, StaticHitTime: true, StaticReuse: true,
		IgnoreResists: true, EffectPower: 100,
		Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}},
	}
}

// deletedCastNukeSkill is a magic damage skill with the same timing.
func deletedCastNukeSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: deletedCastNuke, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, Magic: true, SkillType: "MDAM",
		CastRange: 900, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1_000_000,
	}
}

// deletedCastWorld is a player next to a monster bound to a behavior that
// records its skill-finished hooks.
type deletedCastWorld struct {
	srv    *gameservertest.Server
	c      *testsupport.ScriptedClient
	objID  int32
	caster *npc.Hostile
	cast   func(id modelskill.ID)
	mu     sync.Mutex
	lines  []string
}

func (w *deletedCastWorld) finished() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.lines)
}

func bootDeletedCast(t *testing.T, defs ...modelskill.Definition) *deletedCastWorld {
	t.Helper()
	w := &deletedCastWorld{}
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{deletedCastNPC}, Hooks: script.Hooks{
			OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
				line := fmt.Sprintf("skill=%d target=%d success=%v", e.Skill.ID, e.Creature.ObjectID(), e.Success)
				w.mu.Lock()
				w.lines = append(w.lines, line)
				w.mu.Unlock()
			},
		}}
	}
	w.srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{deletedCastNPC: script.KindHostile},
			[]script.Listing{{Path: "ai.Recorder"}}, script.Catalog{"ai.Recorder": behavior}),
	)
	w.c = w.srv.Client
	startInWorld(t, w.c)
	tmpl := realDamageCasterTemplate(int(deletedCastNPC))
	tmpl.MAtk = 1000
	caster, aiCtl := w.srv.SpawnCastingHostileNPC(t, tmpl, modelskill.NewTable(defs))
	w.caster = caster
	drainUntilQuiet(t, w.c)
	w.objID = w.srv.SoleObjectID(t)
	player := worldCombatant(t, w.srv, w.objID)
	w.cast = func(id modelskill.ID) {
		t.Helper()
		onNPCQueue(t, caster, func() { aiCtl.Cast(player, modelskill.Ref{ID: id, Level: 1}) })
	}
	return w
}

// TestDeletedNPCLaunchedCastStillLands (Npc.deleteMe never stops the cast;
// NpcCast.onMagicHitTimer runs callSkill on the targets of its launch): a
// monster deleted at once between its debuff's launch and its hit still
// lands the debuff on the player, then finishes the cast and raises its
// skill-finished hook with success. Nothing more of the cast reaches the
// player, who no longer knows the monster.
func TestDeletedNPCLaunchedCastStillLands(t *testing.T) {
	t.Parallel()
	w := bootDeletedCast(t, deletedCastDebuffSkill())

	w.cast(deletedCastDebuff)
	w.srv.Advance(t, 700*time.Millisecond)
	if log := readFrameLog(w.c); log.index(objectFrame(serverpackets.OpcodeMagicSkillLaunched, w.caster.ObjectID())) < 0 {
		t.Fatal("the monster's debuff did not launch before the delete")
	}
	w.caster.DeleteNow()
	w.srv.AdvanceUntil(t, "the deleted monster's cast finishing", func() bool { return len(w.finished()) > 0 })
	w.srv.Advance(t, time.Second)

	if !slices.Contains(liveHeldSkillIDs(t, w.srv, w.objID), int32(deletedCastDebuff)) {
		t.Fatal("the launched debuff of the deleted monster did not land on the player")
	}
	want := []string{fmt.Sprintf("skill=%d target=%d success=true", deletedCastDebuff, w.objID)}
	if got := w.finished(); !slices.Equal(got, want) {
		t.Fatalf("skill-finished hooks = %q, want %q", got, want)
	}
	log := readFrameLog(w.c)
	for _, op := range []byte{serverpackets.OpcodeMagicSkillLaunched, serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeMagicSkillUse} {
		if log.index(objectFrame(op, w.caster.ObjectID())) >= 0 {
			t.Fatalf("the player saw cast frame %#x of the deleted monster", op)
		}
	}
}

// TestDeletedNPCLaunchedNukeDealsNoDamage (Mdam, like Pdam, Blow, Manadam
// and Drain, returns on a dead caster, and a deleted NPC is dead): the
// magic nuke that kills the player when its monster lives reaches its hit
// and finishes when the monster is deleted after its launch, but takes no
// HP from the player.
func TestDeletedNPCLaunchedNukeDealsNoDamage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		deleted bool
	}{{"live", false}, {"deleted", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootDeletedCast(t, deletedCastNukeSkill())
			hp := w.srv.PlayerCurrentHP(t, w.objID)

			w.cast(deletedCastNuke)
			w.srv.Advance(t, 700*time.Millisecond)
			if tc.deleted {
				w.caster.DeleteNow()
			}
			w.srv.AdvanceUntil(t, "the monster's cast finishing", func() bool { return len(w.finished()) > 0 })

			if got := w.srv.PlayerCurrentHP(t, w.objID); tc.deleted && got != hp {
				t.Fatalf("player HP %d after the deleted monster's nuke, want %d", got, hp)
			} else if !tc.deleted && got != 0 {
				t.Fatalf("player HP %d after the live monster's nuke, want 0", got)
			}
			want := []string{fmt.Sprintf("skill=%d target=%d success=true", deletedCastNuke, w.objID)}
			if got := w.finished(); !slices.Equal(got, want) {
				t.Fatalf("skill-finished hooks = %q, want %q", got, want)
			}
		})
	}
}

// TestDeletedNPCCastStopsAtItsLaunch (CreatureCast.onMagicLaunch: the
// deleted caster knows no one, so its target is lost): a monster deleted
// before its debuff launches stops the cast there. The debuff never lands
// and the skill-finished hook reports no success.
func TestDeletedNPCCastStopsAtItsLaunch(t *testing.T) {
	t.Parallel()
	w := bootDeletedCast(t, deletedCastDebuffSkill())

	w.cast(deletedCastDebuff)
	w.srv.Advance(t, 300*time.Millisecond)
	w.caster.DeleteNow()
	w.srv.AdvanceUntil(t, "the deleted monster's cast stopping", func() bool { return len(w.finished()) > 0 })
	w.srv.Advance(t, time.Second)

	if slices.Contains(liveHeldSkillIDs(t, w.srv, w.objID), int32(deletedCastDebuff)) {
		t.Fatal("the debuff of a monster deleted before its launch landed")
	}
	want := []string{fmt.Sprintf("skill=%d target=%d success=false", deletedCastDebuff, w.objID)}
	if got := w.finished(); !slices.Equal(got, want) {
		t.Fatalf("skill-finished hooks = %q, want %q", got, want)
	}
	if w.caster.CastingNow() {
		t.Fatal("the deleted monster is still casting")
	}
}
