package pets

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	groupTargetServitorSkill = 1113
	groupTargetServitorNPCID = 12602
	groupTargetPartyHeal     = 1114
	groupTargetServitorHeal  = 1115
	groupTargetHealPower     = 40
)

// bootGroupTargetOwner boots an owner who knows a servitor summon, a
// PARTY-targeted heal and a SUMMON-targeted heal, then summons the servitor.
func bootGroupTargetOwner(t *testing.T) (*servitorOwner, *summon.Actor) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: groupTargetServitorSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", NpcID: groupTargetServitorNPCID, SummonTotalLifeTime: 1_200_000,
			StaticHitTime: true, StaticReuse: true,
		},
		{
			ID: groupTargetPartyHeal, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetParty,
			Radius: 1000, SkillType: "HEAL", Power: groupTargetHealPower, StaticHitTime: true, StaticReuse: true,
		},
		{
			ID: groupTargetServitorHeal, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSummon,
			CastRange: 600, SkillType: "HEAL", Power: groupTargetHealPower, StaticHitTime: true, StaticReuse: true,
		},
	}), gamesql.NewCharacterSkillStore(db))
	cat := &npc.Template{
		ID: groupTargetServitorNPCID, TemplateID: groupTargetServitorNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20, CorpseTime: 60,
	}
	srv := bootPets(t, gameservertest.WithNPCs(npc.NewTable([]*npc.Template{cat})), gameservertest.WithSkills(skills))
	ownerID := srv.SoleObjectID(t)
	for _, id := range []int{groupTargetServitorSkill, groupTargetPartyHeal, groupTargetServitorHeal} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	startInWorld(t, srv.Client)
	o := &servitorOwner{srv: srv, client: srv.Client, id: ownerID}
	o.client.Send(encodeRequestMagicSkillUse(groupTargetServitorSkill))
	readUntilOpcode(t, o.client, serverpackets.OpcodePetInfo, "servitor PetInfo")
	drainUntilQuiet(t, o.client)
	obj, ok := srv.State.Summon(ownerID)
	if !ok {
		t.Fatal("no servitor in world state after its PetInfo")
	}
	return o, obj.(*summon.Actor)
}

// hurtServitor takes damage HP off the servitor on its owner's queue.
func (o *servitorOwner) hurtServitor(t *testing.T, servitor *summon.Actor, damage float64) {
	t.Helper()
	owner, _ := o.srv.State.Player(o.id)
	runOn(t, o.srv.PlayerQueue(t, o.id), func() {
		servitor.ReduceHP(damage, owner.(attackable.Combatant), modelskill.Definition{})
	})
	drainUntilQuiet(t, o.client)
}

// healAmount is what one of the owner's group heals restores: the skill
// power plus the square root of the caster's (whole) M.Atk.
func (o *servitorOwner) healAmount(t *testing.T) float64 {
	t.Helper()
	owner, _ := o.srv.State.Player(o.id)
	caster, ok := owner.(interface{ MAtk() float64 })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no MAtk", o.id, owner)
	}
	return groupTargetHealPower + math.Sqrt(caster.MAtk())
}

// launchedTargets decodes the target ids of a MagicSkillLaunched frame.
func launchedTargets(t *testing.T, frame []byte) []int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // caster
	r.ReadInt32() // skill id
	r.ReadInt32() // level
	n := r.ReadInt32()
	ids := make([]int32, 0, n)
	for range n {
		ids = append(ids, r.ReadInt32())
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read MagicSkillLaunched: %v", err)
	}
	return ids
}

// TestPartySkillCoversOwnersServitor casts a PARTY heal with no party: the
// affected set is the caster followed by its own servitor, which the heal
// restores.
func TestPartySkillCoversOwnersServitor(t *testing.T) {
	t.Parallel()
	o, servitor := bootGroupTargetOwner(t)
	o.hurtServitor(t, servitor, 100)
	before := servitor.HP()

	o.client.Send(encodeRequestMagicSkillUse(groupTargetPartyHeal))
	frames := readUntilOpcode(t, o.client, serverpackets.OpcodeMagicSkillLaunched, "party heal MagicSkillLaunched")
	if got, want := launchedTargets(t, frames[len(frames)-1]), []int32{o.id, servitor.ObjectID()}; !slices.Equal(got, want) {
		t.Fatalf("party heal targets = %v, want owner then servitor %v", got, want)
	}
	o.srv.AdvanceUntil(t, "servitor healed", func() bool { return servitor.HP() > before })
	if got, want := servitor.HP(), before+o.healAmount(t); got != want {
		t.Fatalf("servitor HP after party heal = %v, want %v", got, want)
	}
	drainUntilQuiet(t, o.client)
}

// TestSummonSkillHealsOwnServitor casts a SUMMON-targeted heal: the cast
// aims at the owner's servitor and restores it.
func TestSummonSkillHealsOwnServitor(t *testing.T) {
	t.Parallel()
	o, servitor := bootGroupTargetOwner(t)
	o.hurtServitor(t, servitor, 100)
	before := servitor.HP()

	o.client.Send(encodeRequestMagicSkillUse(groupTargetServitorHeal))
	frames := readUntilOpcode(t, o.client, serverpackets.OpcodeMagicSkillUse, "servitor heal MagicSkillUse")
	r := wire.NewReader(frames[len(frames)-1][1:])
	if caster, target := r.ReadInt32(), r.ReadInt32(); caster != o.id || target != servitor.ObjectID() {
		t.Fatalf("MagicSkillUse caster/target = %d/%d, want %d/%d", caster, target, o.id, servitor.ObjectID())
	}
	frames = readUntilOpcode(t, o.client, serverpackets.OpcodeMagicSkillLaunched, "servitor heal MagicSkillLaunched")
	if got, want := launchedTargets(t, frames[len(frames)-1]), []int32{servitor.ObjectID()}; !slices.Equal(got, want) {
		t.Fatalf("servitor heal targets = %v, want %v", got, want)
	}
	o.srv.AdvanceUntil(t, "servitor healed", func() bool { return servitor.HP() > before })
	if got, want := servitor.HP(), before+o.healAmount(t); got != want {
		t.Fatalf("servitor HP after servitor heal = %v, want %v", got, want)
	}
	drainUntilQuiet(t, o.client)
}

// TestSummonSkillOnDeadServitorIsInvalidTarget casts a SUMMON-targeted heal
// while the servitor lies dead: the owner reads INVALID_TARGET and nothing
// is cast.
func TestSummonSkillOnDeadServitorIsInvalidTarget(t *testing.T) {
	t.Parallel()
	o, servitor := bootGroupTargetOwner(t)
	o.hurtServitor(t, servitor, servitor.HP()+100)
	if !servitor.Dead() {
		t.Fatal("servitor alive after a lethal hit")
	}

	o.client.Send(encodeRequestMagicSkillUse(groupTargetServitorHeal))
	frames := drainFrames(t, o.client)
	var sawInvalid bool
	for _, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeMagicSkillLaunched:
			t.Fatalf("dead servitor heal was cast: frames %x", frameOpcodes(frames))
		case serverpackets.OpcodeSystemMessage:
			if wire.NewReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageInvalidTarget {
				sawInvalid = true
			}
		}
	}
	if !sawInvalid {
		t.Fatalf("dead servitor heal frames %x carry no INVALID_TARGET", frameOpcodes(frames))
	}
}
