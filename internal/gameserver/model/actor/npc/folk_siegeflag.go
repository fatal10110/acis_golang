package npc

import (
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// siegeFlag is a siege headquarters' type: a civilian NPC a clan leader
// puts up for the clan during a siege. A player's interact does nothing on
// it, no effect lands on it, it can be neither healed nor struck by a
// lethal blow, and its stats are its template's own, with none of a
// creature's built-in stat bonuses. A hit warns its clan the base is under
// attack, at most once every headquartersWarnHold. Its clan holds it until
// it dies, which takes it out of the world at once. The anti-infantry
// Swoop Cannon siege summon neither hurts it nor sets off the warning.
const siegeFlag InstanceKind = "SiegeFlag"

// SiegeFlagNpcID is the template id a siege headquarters spawns as.
const SiegeFlagNpcID = 35062

// swoopCannonNpcID is the template id of the Swoop Cannon siege summon.
const swoopCannonNpcID = 14839

// siegeSummoner is an attacker that may be a siege summon.
type siegeSummoner interface {
	SiegeSummon() bool
	NPCID() int
}

// headquartersWarnHold is how long a siege headquarters' clan is warned of
// an attack at most once.
const headquartersWarnHold = 30 * time.Second

// The siege headquarters' template values: its HP, doubled for an
// advanced one, its defences, and its body.
const (
	siegeFlagHP         = 50000
	siegeFlagAdvancedHP = 100000
	siegeFlagDefence    = 500
	siegeFlagRadius     = 10
	siegeFlagHeight     = 80
)

// SiegeFlagTemplate is the template of a siege headquarters clanName puts
// up: the advanced one has twice the HP. Every value it does not set is the
// template default; it shows clanName as its name, and never moves.
func SiegeFlagTemplate(clanName string, advanced bool) *Template {
	hp := float64(siegeFlagHP)
	if advanced {
		hp = siegeFlagAdvancedHP
	}
	return &Template{
		ID: SiegeFlagNpcID, TemplateID: SiegeFlagNpcID,
		Name: clanName, UsingServerSideName: true,
		Type:  string(siegeFlag),
		Level: 1,

		BaseDamageRange: []int{0, 0, 80, 120},
		Race:            RaceDummy,

		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,

		HPMax: hp, HPRegen: 1.5, MPRegen: 0.9,
		PDef: siegeFlagDefence, MDef: siegeFlagDefence,
		AtkSpd: 300, CritRate: 4,

		CollisionRadius: siegeFlagRadius,
		CollisionHeight: siegeFlagHeight,

		CanBeAttacked: true,
		CorpseTime:    7,
		CanMove:       true,
	}
}

// headquarters is a siege headquarters' own state: the clan it stands for
// and, in Unix milliseconds on its queue clock, when the hold on its next
// attack warning ends.
type headquarters struct {
	clan      atomic.Pointer[clan.Clan]
	warnUntil atomic.Int64
}

// SiegeFlag reports a siege headquarters.
func (f *Folk) SiegeFlag() bool { return hostileKind(f.Instance) == siegeFlag }

// StandFor makes the siege headquarters cl's. It reports false, the
// headquarters standing for no clan, when cl already holds one.
func (f *Folk) StandFor(cl *clan.Clan) bool {
	if !f.SiegeFlag() || !cl.SetHeadquarters(f) {
		return false
	}
	f.headquarters.clan.Store(cl)
	return true
}

// HeadquartersClan returns the clan a siege headquarters stands for.
func (f *Folk) HeadquartersClan() (*clan.Clan, bool) {
	cl := f.headquarters.clan.Load()
	return cl, cl != nil
}

// RefusesEffects reports an NPC no effect lands on: a siege headquarters.
func (f *Folk) RefusesEffects() bool { return f.SiegeFlag() }

// headquartersHit is a hit on a siege headquarters, before its HP is
// touched. It reports false, the hit landing nothing, for a Swoop Cannon
// attacker; otherwise it warns the clan when no warning is on hold, and
// holds the next for headquartersWarnHold.
func (f *Folk) headquartersHit(attacker attackable.Combatant) bool {
	if s, ok := attacker.(siegeSummoner); ok && s.SiegeSummon() && s.NPCID() == swoopCannonNpcID {
		return false
	}
	now := f.now().UnixMilli()
	until := f.headquarters.warnUntil.Load()
	if now >= until && f.headquarters.warnUntil.CompareAndSwap(until, now+headquartersWarnHold.Milliseconds()) {
		f.emit(event.HeadquartersAttacked{})
	}
	return true
}

// headquartersDied drops a dead siege headquarters from its clan, which
// takes it out of the world.
func (f *Folk) headquartersDied() {
	if cl := f.headquarters.clan.Load(); cl != nil {
		cl.DropHeadquartersOf(f.ObjectID())
	}
}
