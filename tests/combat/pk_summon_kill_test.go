package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// pkServitorSummonSkill summons pkServitorNPCID.
	pkServitorSummonSkill = 1111
	pkServitorNPCID       = 12077
	// pkServitorStrikeAction is the servitor special-skill shortcut that
	// casts pkServitorStrikeSkill on the owner's target.
	pkServitorStrikeAction = int32(1003)
	pkServitorStrikeSkill  = 4710
	// pkServitorAttackAction is the summon attack shortcut.
	pkServitorAttackAction = int32(16)
)

// pkServitorTemplate is the servitor pkServitorSummonSkill summons, knowing
// the lethal strike.
func pkServitorTemplate() *npc.Template {
	return &npc.Template{
		ID: pkServitorNPCID, TemplateID: pkServitorNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, BaseAttackRange: 40,
		CollisionRadius: 8, CollisionHeight: 20, CorpseTime: 7,
		Skills: map[int]int{pkServitorStrikeSkill: 1},
	}
}

// bootServitorPKKillScene boots the PK kill scene with the killer knowing
// the servitor summon of cat, summons the servitor and waits for every
// client to go quiet. The owner starts karma-free and flagged.
func bootServitorPKKillScene(t *testing.T, cat *npc.Template) (*pkKillScene, *summon.Actor) {
	t.Helper()
	s := bootPKKillSceneWith(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{cat})),
	}, pkServitorSkills()...)
	s.c.Send(encodeRequestMagicSkillUse(pkServitorSummonSkill, false, false))
	var servitor *summon.Actor
	s.srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := s.srv.State.Summon(s.objID)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	drainUntilQuiet(t, s.c)
	drainUntilQuiet(t, s.vc)
	if s.karma() != 0 || s.killer.PvPFlagState() == task.PvPFlagNone {
		t.Fatal("owner must start karma-free and flagged")
	}
	// The karma change also reports the owner's relation to its servitor
	// to the owner (broadcastRelations).
	s.karmaTail = []byte{serverpackets.OpcodeUserInfo, serverpackets.OpcodeRelationChanged}
	return s, servitor
}

// pkServitorSkills are the servitor summon the killer knows and the lethal
// strike its servitor knows.
func pkServitorSkills() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: pkServitorSummonSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", NpcID: pkServitorNPCID, SummonTotalLifeTime: 1_200_000,
			StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{
			ID: pkServitorStrikeSkill, Level: 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetOne, Offensive: true, SkillType: "PDAM",
			CastRange: 900, HitTime: 500, ReuseDelay: 60_000,
			StaticHitTime: true, StaticReuse: true, Power: 1_000_000,
		},
	}
}

// TestServitorPKKillTakesTheOwnersKnifeOffBeforeItsDamageMessage has a
// PvP-flagged player wearing the shipped PK-free knife command its servitor
// to kill an innocent player with a damaging skill. The kill makes the
// owner a PKer inside the servitor's killing blow (Playable.doDie →
// onKillUpdatePvPKarma of the acting player): after the karma change the
// knife comes off (S1_DISARMED) and the flag resets (UserInfo), and only
// then does the owner read the servitor's damage message, which the skill's
// handler sends after reduceCurrentHp. The servitor's offensive skill flags
// its owner once its effects have run, so the owner ends with the karma and
// the flag, the knife off.
func TestServitorPKKillTakesTheOwnersKnifeOffBeforeItsDamageMessage(t *testing.T) {
	t.Parallel()
	s, servitor := bootServitorPKKillScene(t, pkServitorTemplate())

	selectPlayerTarget(t, s.c, s.victimID)
	s.c.Send(encodeRequestActionUse(pkServitorStrikeAction, true))
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames, aborted := s.assertKnifeTakenOff(t)
	flagReset := indexOf(frames, aborted+1, serverpackets.OpcodeUserInfo, -1)
	damage := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageSummonGaveDamageS1)
	if flagReset < 0 || damage < flagReset {
		t.Fatalf("frames after the karma change = %x, want S1_DISARMED and the flag reset's UserInfo before SUMMON_GAVE_DAMAGE_S1", opcodesOf(frames))
	}
	if reflag := indexOf(frames, damage+1, serverpackets.OpcodeUserInfo, -1); reflag < 0 {
		t.Fatalf("frames after the karma change = %x, want the re-flag's UserInfo after SUMMON_GAVE_DAMAGE_S1", opcodesOf(frames))
	}
	if servitor.Dead() {
		t.Fatal("servitor died in the scenario")
	}
	if karma := s.karma(); karma != 240 {
		t.Fatalf("owner karma = %d after the servitor's PK kill, want 240", karma)
	}
	if state := s.killer.PvPFlagState(); state == task.PvPFlagNone {
		t.Fatal("owner unflagged after the servitor's skill PK kill, want the skill's flag")
	}
	if n := s.flags.Len(); n != 1 {
		t.Fatalf("PvP flag task tracks %d players after the servitor's PK kill, want the owner", n)
	}
}

// TestServitorHitPKKillTakesTheOwnersKnifeOffBeforeItsAbsorbedHP has the
// same owner command its wounded servitor's auto-attack on the innocent
// player, the servitor absorbing half of its damage. The hit flags the
// owner before its damage (CreatureAttack.onHitTimer); the kill then makes
// the owner a PKer inside the damage: the knife comes off and the flag
// resets before the rest of the hit, so the owner reads the servitor's
// absorbed HP (PetStatusUpdate) only after the flag reset's UserInfo, and
// ends with karma and no flag.
func TestServitorHitPKKillTakesTheOwnersKnifeOffBeforeItsAbsorbedHP(t *testing.T) {
	t.Parallel()
	cat := pkServitorTemplate()
	cat.PAtk = 100_000
	s, servitor := bootServitorPKKillScene(t, cat)
	onPlayerQueue(t, s.srv, s.objID, func(*player.Character) {
		// Every swing lands without a critical: hit and critical rolls
		// alternate within a swing.
		calls := 0
		servitor.SetRollSource(func(int) int {
			calls++
			if calls%2 == 1 {
				return 0
			}
			return 999
		})
		servitor.AddStatFuncs([]effect.Mod{{Stat: stat.AbsorbDamagePercent, Op: effect.OpAdd, Value: 50}})
		servitor.SetHP(1)
	})
	drainUntilQuiet(t, s.c)

	selectPlayerTarget(t, s.c, s.victimID)
	s.c.Send(encodeRequestActionUse(pkServitorAttackAction, true))
	assertKarmaChangeFrames(t, s.c, s.objID, 240)
	s.srv.Settle(t)

	frames, aborted := s.assertKnifeTakenOff(t)
	flagReset := indexOf(frames, aborted+1, serverpackets.OpcodeUserInfo, -1)
	absorbed := indexOf(frames, 0, serverpackets.OpcodePetStatusUpdate, -1)
	if flagReset < 0 || absorbed < flagReset {
		t.Fatalf("frames after the karma change = %x, want S1_DISARMED and the flag reset's UserInfo before the absorbed HP's PetStatusUpdate", opcodesOf(frames))
	}
	if hp := servitor.HP(); hp <= 1 {
		t.Fatalf("servitor HP = %v after the lethal hit, want the absorbed share", hp)
	}
	if karma := s.karma(); karma != 240 {
		t.Fatalf("owner karma = %d after the servitor's PK kill, want 240", karma)
	}
	if state := s.killer.PvPFlagState(); state != task.PvPFlagNone {
		t.Fatalf("owner PvP flag = %v after the servitor's lethal hit, want none", state)
	}
	if n := s.flags.Len(); n != 0 {
		t.Fatalf("PvP flag task tracks %d players after the servitor's lethal hit, want none", n)
	}
}
