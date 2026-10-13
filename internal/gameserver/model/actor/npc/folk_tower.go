package npc

import (
	"sync/atomic"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
)

// The control tower types. A castle's control towers stand where its
// castles.xml entry places them from boot on, for the castle's whole life:
// dead outside a siege, shown with ServerObjectInfo as their own template,
// and working while a siege is fought, shown with NpcInfo as their working
// form. Only a player, or a player's summon, of a clan attacking the
// castle's siege may attack a working tower; brought down to 1 HP, it
// falls dead again. A player's interact does nothing on a tower.
const (
	lifeTower  InstanceKind = "LifeTower"
	flameTower InstanceKind = "FlameTower"
)

// The template ids a control tower is shown as: its working form, and its
// own dead template.
const (
	LifeTowerWorkingNpcID  = 13002
	LifeTowerDeadNpcID     = 13003
	FlameTowerWorkingNpcID = 13004
	FlameTowerDeadNpcID    = 13005
)

// A control tower's body.
const (
	towerRadius = 7
	towerHeight = 35
)

// TowerTemplate is the template a castle control tower spawns from: the
// dead life tower's id and name when life is set, the dead flame tower's
// otherwise, with hp, pDef and mDef from castles.xml, no MP and no attack,
// standing still for good, and never brought under 1 HP. Every value it
// does not set is the template default.
func TowerTemplate(life bool, hp, pDef, mDef float64) *Template {
	id, kind, name := FlameTowerDeadNpcID, flameTower, "Flame Control Tower"
	if life {
		id, kind, name = LifeTowerDeadNpcID, lifeTower, "Life Control Tower"
	}
	return &Template{
		ID: id, TemplateID: id,
		Name: name,
		Type: string(kind), Level: 1,

		BaseDamageRange: []int{0, 0, 80, 120},
		Race:            RaceDummy,

		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,

		HPMax: hp, HPRegen: 1.5, MPRegen: 0.9,
		PDef: pDef, MDef: mDef,
		AtkSpd: 300, CritRate: 4,

		CollisionRadius: towerRadius,
		CollisionHeight: towerHeight,

		Undying:       true,
		CanBeAttacked: true,
		CorpseTime:    7,
		CanMove:       true,
	}
}

// TowerWorkingNpcID is the template id of inst's working form, false when
// inst is no control tower.
func TowerWorkingNpcID(inst *Instance) (int, bool) {
	switch hostileKind(inst) {
	case lifeTower:
		return LifeTowerWorkingNpcID, true
	case flameTower:
		return FlameTowerWorkingNpcID, true
	}
	return 0, false
}

// TowerSiege is the siege of the castle a control tower stands for.
type TowerSiege interface {
	GuardedSiege
	// InProgress reports whether the siege is being fought.
	InProgress() bool
	// LifeTowerDown is a life tower of the castle falling dead while the
	// siege is fought.
	LifeTowerDown()
}

// TowerRuntime is what a control tower works with: the siege of its castle,
// nil for none, which leaves it dead and unattackable, and its working
// form, nil when no template defines it, which leaves it dead.
type TowerRuntime struct {
	Siege   TowerSiege
	Working *Template
}

// tower is a control tower's own state. form is its working form while it
// works, nil while it stands dead.
type tower struct {
	siege   TowerSiege
	working *Template
	form    atomic.Pointer[Template]
}

// Tower reports a castle control tower.
func (f *Folk) Tower() bool {
	k := hostileKind(f.Instance)
	return k == lifeTower || k == flameTower
}

// TowerLife reports a life control tower.
func (f *Folk) TowerLife() bool { return hostileKind(f.Instance) == lifeTower }

// TowerWorking reports a control tower in its working form.
func (f *Folk) TowerWorking() bool { return f.towerState.form.Load() != nil }

// PolymorphTower puts a control tower in its working form, as a siege
// starts: the players around it see it go and come back as it now is. A
// tower with no working form stays as it is.
func (f *Folk) PolymorphTower() {
	if !f.Tower() || f.towerState.working == nil {
		return
	}
	f.towerState.form.Store(f.towerState.working)
	f.reappear()
}

// UnpolymorphTower stands a control tower dead at full HP, as a siege
// ends: its HP is filled, then the players around it see it go and come
// back dead, whatever it was.
func (f *Folk) UnpolymorphTower() {
	if !f.Tower() {
		return
	}
	f.SetHP(f.MaxHPValue())
	f.towerState.form.Store(nil)
	f.reappear()
}

// TowerMidVictory is a control tower's turn when its castle changes hands
// while the siege is fought: a working flame tower falls dead, its HP as
// it is; a dead life tower is filled and works again for the new
// defenders. Any other tower stays as it is.
func (f *Folk) TowerMidVictory() {
	switch {
	case !f.Tower():
	case f.TowerLife() && !f.TowerWorking():
		f.SetHP(f.MaxHPValue())
		f.PolymorphTower()
	case !f.TowerLife() && f.TowerWorking():
		f.towerState.form.Store(nil)
		f.reappear()
	}
}

// reappear takes the NPC off the grid and puts it back where it stands:
// the players around it forget it, then see it again as it now is.
func (f *Folk) reappear() {
	if f.world == nil {
		return
	}
	f.world.Leave(f)
	f.world.Rejoin(f)
}

// towerHit is a hit on a control tower, after its HP is touched. A working
// tower its castle's siege is fought around falls dead once at 1 HP or
// less; a life tower falling so tells the siege.
func (f *Folk) towerHit() {
	s := f.towerState.siege
	if s == nil || !s.InProgress() {
		return
	}
	form := f.towerState.form.Load()
	if form == nil || f.HP() > folkMinHP || !f.towerState.form.CompareAndSwap(form, nil) {
		return
	}
	f.reappear()
	if f.TowerLife() {
		s.LifeTowerDown()
	}
}

// towerAttackableBy reports whether attacker may attack a control tower: a
// working one, by a player, or a player's summon, whose clan attacks the
// siege being fought around the tower's castle.
func (f *Folk) towerAttackableBy(attacker skilltarget.Actor) bool {
	if f.towerState.siege == nil || !f.TowerWorking() {
		return false
	}
	clanID, ok := actingClanID(attacker)
	return ok && f.towerState.siege.Attacking(clanID)
}

// actingClanID is the clan of the player acting through attacker: the
// player itself, or a summon's owner. ok is false for any other attacker.
func actingClanID(attacker skilltarget.Actor) (int32, bool) {
	var acting any = attacker
	switch attacker.Kind() {
	case actor.KindPlayer:
	case actor.KindSummon:
		owner, ok := attacker.Owner()
		if !ok || owner == nil {
			return 0, false
		}
		acting = owner
	default:
		return 0, false
	}
	member, ok := acting.(interface{ ClanID() int32 })
	if !ok {
		return 0, false
	}
	return member.ClanID(), true
}
