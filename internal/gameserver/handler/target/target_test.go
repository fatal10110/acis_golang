package target

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func mustHandler(t *testing.T, r *Registry, target modelskill.Target) Handler {
	t.Helper()
	h, ok := r.Handler(target)
	if !ok {
		t.Fatalf("Handler(%s) missing", target)
	}
	return h
}

func ids(creatures []Creature) []int32 {
	out := make([]int32, 0, len(creatures))
	for _, creature := range creatures {
		out = append(out, creature.ObjectID())
	}
	return out
}

type targetActor struct {
	id       int32
	x, y, z  int
	heading  int
	dead     bool
	category Category

	see                    map[int32]bool
	attackableBy           bool
	attackableWithoutForce bool
	summon                 Creature
	owner                  Creature
	holy                   bool
	unlockable             bool
	undead                 bool
	peace                  bool
	corpse                 bool
	corpseDeadline         time.Time
	corpseTime             time.Duration
	spoiled                bool
	seeded                 bool

	sameParty    map[int32]bool
	sameClan     map[int32]bool
	sameAlly     map[int32]bool
	partyMembers map[int32]bool
	inParty      bool
	olympiad     bool
	duelID       int32
	duelTeam     int
	hasClan      bool
	mageClass    bool
	clanGroups   []string
	pet          bool
}

type targetCreature struct {
	id       int32
	x, y, z  int
	heading  int
	dead     bool
	category Category
}

func (c targetCreature) ObjectID() int32 { return c.id }

func (c targetCreature) Position() (int, int, int) { return c.x, c.y, c.z }

func (c targetCreature) Heading() int { return c.heading }

func (c targetCreature) Dead() bool { return c.dead }

func (c targetCreature) Category() Category { return c.category }

func (a *targetActor) ObjectID() int32 { return a.id }

func (a *targetActor) Position() (int, int, int) { return a.x, a.y, a.z }

func (a *targetActor) Heading() int { return a.heading }

func (a *targetActor) Dead() bool { return a.dead }

func (a *targetActor) Category() Category { return a.category }

func (a *targetActor) CanSeeTarget(target Creature) bool {
	if a.see == nil {
		return true
	}
	visible, ok := a.see[target.ObjectID()]
	return !ok || visible
}

func (a *targetActor) AttackableBy(Creature) bool { return a.attackableBy }

func (a *targetActor) AttackableWithoutForceBy(Creature) bool { return a.attackableWithoutForce }

func (a *targetActor) Summon() (Creature, bool) { return a.summon, a.summon != nil }

func (a *targetActor) Owner() (Creature, bool) { return a.owner, a.owner != nil }

func (a *targetActor) IsPet() bool { return a.pet }

func (a *targetActor) Holy() bool { return a.holy }

func (a *targetActor) Unlockable() bool { return a.unlockable }

func (a *targetActor) Undead() bool { return a.undead }

func (a *targetActor) InPeaceZone() bool { return a.peace }

func (a *targetActor) HasCorpse() bool { return a.corpse }

func (a *targetActor) CorpseDeadline() (time.Time, bool) {
	return a.corpseDeadline, !a.corpseDeadline.IsZero()
}

func (a *targetActor) CorpseTime() time.Duration { return a.corpseTime }

func (a *targetActor) Spoiled() bool { return a.spoiled }

func (a *targetActor) Seeded() bool { return a.seeded }

func (a *targetActor) IsInSameParty(other Creature) bool {
	return actorInSet(a.sameParty, other)
}

func (a *targetActor) IsInSameClan(other Creature) bool {
	return actorInSet(a.sameClan, other)
}

func (a *targetActor) IsInSameAlly(other Creature) bool {
	return actorInSet(a.sameAlly, other)
}

func (a *targetActor) IsInParty() bool { return a.inParty }

func (a *targetActor) PartyContains(other Creature) bool {
	return actorInSet(a.partyMembers, other)
}

func (a *targetActor) OlympiadMode() bool { return a.olympiad }

func (a *targetActor) DuelID() int32 { return a.duelID }

func (a *targetActor) DuelTeam() int { return a.duelTeam }

func (a *targetActor) HasClan() bool { return a.hasClan }

func (a *targetActor) MageClass() bool { return a.mageClass }

func (a *targetActor) ClanGroups() []string { return a.clanGroups }

func actorInSet(set map[int32]bool, c Creature) bool {
	if set == nil || c == nil {
		return false
	}
	return set[c.ObjectID()]
}

type knownList []*targetActor

func (k knownList) ForEachKnownCreatureInRadius(anchor Creature, radius int, fn func(Creature)) {
	ax, ay, az := anchor.Position()
	for _, actor := range k {
		if actor.ObjectID() == anchor.ObjectID() {
			continue
		}
		if radius != -1 {
			if !location.In3DRange(actor.x, actor.y, actor.z, ax, ay, az, radius) {
				continue
			}
		}
		fn(actor)
	}
}
