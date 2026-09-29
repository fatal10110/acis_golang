package skills

import (
	"testing"
	"time"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	// npcBreakSkill is the casting monster's ten-second magic self buff.
	npcBreakSkill = modelskill.ID(9103)
	// npcStrikeSkill is the player's quick physical strike on the monster.
	npcStrikeSkill = 43
)

var npcBreakRef = modelskill.Ref{ID: npcBreakSkill, Level: 1}

// castingNPC is a player in the world next to a monster wired with the
// production AI-cast seam, the monster already targeted and mid-way through
// its ten-second magic self buff.
type castingNPC struct {
	srv     *gameservertest.Server
	c       *testsupport.ScriptedClient
	objID   int32
	hostile *npc.Hostile
	maxHP   int
}

// bootCastingNPC boots castingNPC. raid marks the monster raid-related and
// roll fixes its combat rolls, the cast-break roll among them, before the
// cast starts.
func bootCastingNPC(t *testing.T, raid bool, roll int) *castingNPC {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: npcStrikeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Offensive: true, CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: "PDAM", Power: 100,
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, npcStrikeSkill, 1)
	startInWorld(t, c)
	hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1_000_000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{{
		ID: npcBreakSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		Magic: true, SkillType: "BUFF", HitTime: 10_000, StaticHitTime: true, StaticReuse: true,
	}}))
	onNPCQueue(t, hostile, func() {
		hostile.SetRaidRelated(raid)
		hostile.SetRollSource(func(int) int { return roll })
	})
	drainUntilQuiet(t, c)
	maxHP := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	startNPCCast(t, c, hostile, aiCtl)
	return &castingNPC{srv: srv, c: c, objID: objID, hostile: hostile, maxHP: maxHP}
}

// startNPCCast starts the monster's buff on its own queue, as its AI loop
// does, and reads the MagicSkillUse the player sees.
func startNPCCast(t *testing.T, c *testsupport.ScriptedClient, hostile *npc.Hostile, aiCtl *actorcast.AIController) {
	t.Helper()
	onNPCQueue(t, hostile, func() { aiCtl.Cast(hostile, npcBreakRef) })
	use := readUntil(t, c, serverpackets.OpcodeMagicSkillUse)
	if caster := wireReader(use[1:]).ReadInt32(); caster != hostile.ObjectID() {
		t.Fatalf("MagicSkillUse caster = %d, want monster %d", caster, hostile.ObjectID())
	}
	if !hostile.CastingNow() {
		t.Fatal("CastingNow() = false after the monster's MagicSkillUse, want its buff in flight")
	}
}

// npcCanceled reads the player's frames until the stream goes quiet and
// reports whether any was a MagicSkillCanceled for the monster.
func (w *castingNPC) npcCanceled(t *testing.T) bool {
	t.Helper()
	canceled := false
	for {
		frame := w.c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return canceled
		}
		if frame[0] == serverpackets.OpcodeMagicSkillCanceled && wireReader(frame[1:]).ReadInt32() == w.hostile.ObjectID() {
			canceled = true
		}
	}
}

// landOnNPC applies the named real effect to the monster on its own queue,
// with the player as its effector.
func (w *castingNPC) landOnNPC(t *testing.T, name string) {
	t.Helper()
	var effector *player.Character
	onPlayerQueue(t, w.srv, w.objID, func(pc *player.Character) { effector = pc })
	e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: name, Time: 30})
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	e.Effector, e.Effected = effector, w.hostile
	onNPCQueue(t, w.hostile, func() { w.hostile.EffectList().Add(e) })
}

// TestPlayerSkillHitBreaksNPCMagicCast drives a player's physical skill into
// a monster mid-way through a magic cast and pins Formulas.calcCastBreak
// (Formulas.java:725-750) for an NPC target: a roll under the clamped rate
// breaks the cast (observers see MagicSkillCanceled), a roll over it leaves
// the cast running, and a raid-related monster is never broken.
func TestPlayerSkillHitBreaksNPCMagicCast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		raid  bool
		roll  int
		broke bool
	}{
		{name: "roll under the rate breaks", roll: 0, broke: true},
		{name: "roll over the rate keeps casting", roll: 99},
		{name: "raid-related monster never breaks", raid: true, roll: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootCastingNPC(t, tt.raid, tt.roll)

			w.c.Send(encodeRequestMagicSkillUse(npcStrikeSkill, false, false))
			w.srv.AdvanceUntil(t, "strike landing on the monster", func() bool { return w.hostile.CurrentHP() < w.maxHP })

			if got := w.npcCanceled(t); got != tt.broke {
				t.Fatalf("monster MagicSkillCanceled = %v, want %v", got, tt.broke)
			}
			if got := w.hostile.CastingNow(); got == tt.broke {
				t.Fatalf("monster CastingNow() = %v after the hit, want %v", got, !tt.broke)
			}
		})
	}
}

// TestNPCAutoAttackHitBreaksNPCMagicCast lands another monster's melee hit
// on the casting monster: the auto-attack path rolls the same cast break.
func TestNPCAutoAttackHitBreaksNPCMagicCast(t *testing.T) {
	t.Parallel()
	w := bootCastingNPC(t, false, 0)
	attacker := w.srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX + 20, Y: hostileY, Z: hostileZ})
	attacker.DoAttack(t, w.hostile)

	if !w.npcCanceled(t) {
		t.Fatal("monster MagicSkillCanceled = false after a melee hit under a breaking roll, want the cast broken")
	}
	if w.hostile.CastingNow() {
		t.Fatal("monster CastingNow() = true after the breaking hit, want its cast aborted")
	}
}

// TestAbortCastInterruptsNPCCast lands AbortCast on a casting monster. It
// interrupts the cast of any creature but a raid-related one
// (EffectAbortCast.onStart), and the monster's observers see
// MagicSkillCanceled.
func TestAbortCastInterruptsNPCCast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		raid  bool
		broke bool
	}{
		{name: "monster", broke: true},
		{name: "raid-related monster", raid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootCastingNPC(t, tt.raid, 99)
			w.landOnNPC(t, "AbortCast")

			if got := w.npcCanceled(t); got != tt.broke {
				t.Fatalf("monster MagicSkillCanceled = %v, want %v", got, tt.broke)
			}
			if got := w.hostile.CastingNow(); got == tt.broke {
				t.Fatalf("monster CastingNow() = %v after AbortCast, want %v", got, !tt.broke)
			}
		})
	}
}

// TestRemoveTargetStopsNPCCast lands RemoveTarget on a casting monster: it
// stops the cast unconditionally (EffectRemoveTarget.onStart ->
// CreatureCast.stop), which observers see as MagicSkillCanceled.
func TestRemoveTargetStopsNPCCast(t *testing.T) {
	t.Parallel()
	w := bootCastingNPC(t, false, 99)
	w.landOnNPC(t, "RemoveTarget")

	if !w.npcCanceled(t) {
		t.Fatal("monster MagicSkillCanceled = false after RemoveTarget, want the cast stopped")
	}
	if w.hostile.CastingNow() {
		t.Fatal("monster CastingNow() = true after RemoveTarget, want its cast stopped")
	}
}
