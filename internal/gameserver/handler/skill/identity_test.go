package skill

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var fakeObjectIDs atomic.Int32

// nextFakeObjectID hands a test double a distinct world object id from the
// same range live objects are allocated from.
func nextFakeObjectID() int32 {
	return idfactory.FirstObjectID + fakeObjectIDs.Add(1)
}

// idActor is a bare Actor carrying only an object id. Its slice field makes
// the type non-comparable, so a Go == on two idActor interface values would
// panic: the identity check must never fall back to one.
type idActor struct {
	id   int32
	tags []string
}

func (a idActor) ObjectID() int32         { return a.id }
func (idActor) Kind() actor.Kind          { return actor.KindNPC }
func (idActor) Dead() bool                { return false }
func (idActor) Position() (x, y, z int)   { return 0, 0, 0 }
func (idActor) Heading() int              { return 0 }
func (a *idActor) retag(tag string) Actor { a.tags = append(a.tags, tag); return *a }

func TestSameObjectIdentityContract(t *testing.T) {
	id := nextFakeObjectID()
	other := nextFakeObjectID()
	shared := &idActor{id: id}
	unplaced := &idActor{}

	tests := []struct {
		name string
		a, b Actor
		want bool
	}{
		{"both absent", nil, nil, true},
		{"absent vs placed", nil, shared, false},
		{"placed vs absent", shared, nil, false},
		{"same value", shared, shared, true},
		{"same id, distinct Go values", idActor{id: id}, shared.retag("wrapper"), true},
		{"same id, different Go types", idActor{id: id}, &fakeCubicSummoner{fakeActor: fakeActor{objectID: id}}, true},
		{"different ids, otherwise equal values", idActor{id: id}, idActor{id: other}, false},
		{"different ids, same Go type pointers", &fakeCubicSummoner{fakeActor: fakeActor{objectID: id}}, &fakeCubicSummoner{fakeActor: fakeActor{objectID: other}}, false},
		{"unplaced actor is not even itself", unplaced, unplaced, false},
		{"two unplaced actors", idActor{}, idActor{}, false},
		{"unplaced vs placed", idActor{}, shared, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameObject(tt.a, tt.b); got != tt.want {
				t.Fatalf("sameObject() = %v, want %v", got, tt.want)
			}
			if got := sameObject(tt.b, tt.a); got != tt.want {
				t.Fatalf("sameObject() reversed = %v, want %v", got, tt.want)
			}
		})
	}
}

// A mass cubic cast treats the target that is the caster's world object as
// the caster's own admission even when it reaches the handler as a distinct
// Go value, and every other object id as given by another player.
func TestCubicMassCastClassifiesRecipientsByObjectID(t *testing.T) {
	caster := newFakeCubicSummoner(true)
	casterView := newFakeCubicSummoner(true)
	casterView.objectID = caster.objectID
	other := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm)},
		Targets: []Actor{casterView, other},
	})

	if casterView.givenByOther[cubic.Storm] {
		t.Fatal("caster's own object reported givenByOther=true, want false")
	}
	if !other.givenByOther[cubic.Storm] {
		t.Fatal("other object reported givenByOther=false, want true")
	}
	if !result.CubicTouched || !result.CubicAdded {
		t.Fatalf("CubicTouched/CubicAdded = %v/%v, want true/true", result.CubicTouched, result.CubicAdded)
	}
	if got := result.CubicTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicTargets = %v, want other only", got)
	}
}

type identityPlayer struct {
	neutralPlayer
	world.Presence
	fakeActor
	current            world.Tracked
	attacked, targeted world.Tracked
}

func (p *identityPlayer) CurrentTarget() world.Tracked { return p.current }
func (p *identityPlayer) AttackTarget(t world.Tracked) { p.attacked = t }
func (p *identityPlayer) SetTarget(t world.Tracked)    { p.targeted = t }
func (*identityPlayer) AlikeDead() bool                { return false }
func (*identityPlayer) Kind() actor.Kind               { return actor.KindPlayer }

func newIdentityPlayer(id int32) *identityPlayer {
	return &identityPlayer{fakeActor: fakeActor{objectID: id}}
}

func TestCanBeSummonedExcludesCasterByObjectID(t *testing.T) {
	caster := newIdentityPlayer(nextFakeObjectID())
	if canBeSummoned(caster, newIdentityPlayer(caster.objectID)) {
		t.Fatal("canBeSummoned(caster, caster's own object) = true, want false")
	}
	if !canBeSummoned(caster, newIdentityPlayer(nextFakeObjectID())) {
		t.Fatal("canBeSummoned(caster, another player) = false, want true")
	}
}

// A provoked playable already targeting the caster's object attacks it; one
// targeting a different object is retargeted onto the caster.
func TestAggressionRetargetComparesObjectIDs(t *testing.T) {
	caster := newIdentityPlayer(nextFakeObjectID())

	onCaster := newIdentityPlayer(nextFakeObjectID())
	onCaster.current = newIdentityPlayer(caster.objectID)
	fireAggressionEvent(caster, onCaster, modelskill.Definition{})
	if onCaster.attacked != caster || onCaster.targeted != nil {
		t.Fatalf("target already on caster: attacked=%v targeted=%v, want attack on caster", onCaster.attacked, onCaster.targeted)
	}

	onOther := newIdentityPlayer(nextFakeObjectID())
	onOther.current = newIdentityPlayer(nextFakeObjectID())
	fireAggressionEvent(caster, onOther, modelskill.Definition{})
	if onOther.targeted != caster || onOther.attacked != nil {
		t.Fatalf("target on another object: attacked=%v targeted=%v, want retarget to caster", onOther.attacked, onOther.targeted)
	}
}
