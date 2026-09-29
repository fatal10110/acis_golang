package item

import (
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
)

// The live player and summon types must satisfy the gate's optional
// interfaces; otherwise a dead player or pet silently falls through to
// ResurrectionScrollAllowed.
var (
	_ resurrectionPlayerTarget = (*player.Character)(nil)
	_ petOwnerHolder           = (*summon.Actor)(nil)
)

// scrollTarget is a selected creature for ResurrectionScrollGate. The
// embedded Actor is nil: the gate only calls the methods overridden here.
type scrollTarget struct {
	skilltarget.Actor
	kind  actor.Kind
	dead  bool
	isPet bool
}

func (t *scrollTarget) Kind() actor.Kind { return t.kind }
func (t *scrollTarget) Dead() bool       { return t.dead }
func (t *scrollTarget) IsPet() bool      { return t.isPet }

// scrollPlayer is a selected player with its siege, festival and revive
// offer state.
type scrollPlayer struct {
	scrollTarget
	inSiege, festival, offer, offerForPet bool
}

func (p *scrollPlayer) InSiegeZone() bool         { return p.inSiege }
func (p *scrollPlayer) FestivalParticipant() bool { return p.festival }
func (p *scrollPlayer) ReviveOffer() (bool, bool) { return p.offer, p.offerForPet }
func newScrollPlayer(dead bool) *scrollPlayer {
	return &scrollPlayer{scrollTarget: scrollTarget{kind: actor.KindPlayer, dead: dead}}
}

func (p *scrollPlayer) withOffer(forPet bool) *scrollPlayer {
	p.offer, p.offerForPet = true, forPet
	return p
}

// ownerSurface names summon.Owner for embedding: the interface has its
// own Owner method, so it cannot be embedded under its own name.
type ownerSurface = summon.Owner

// scrollOwner is a pet owner; the embedded owner is nil and only
// ObjectID and ReviveOffer are called.
type scrollOwner struct {
	ownerSurface
	id                 int32
	offer, offerForPet bool
}

func (o *scrollOwner) ObjectID() int32           { return o.id }
func (o *scrollOwner) ReviveOffer() (bool, bool) { return o.offer, o.offerForPet }

// scrollSummon is a selected pet or servitor with its owner.
type scrollSummon struct {
	scrollTarget
	owner summon.Owner
}

func (s *scrollSummon) SummonOwner() summon.Owner { return s.owner }

func newScrollSummon(dead, isPet bool, owner summon.Owner) *scrollSummon {
	return &scrollSummon{scrollTarget: scrollTarget{kind: actor.KindSummon, dead: dead, isPet: isPet}, owner: owner}
}

func TestResurrectionScrollGate(t *testing.T) {
	const user int32 = 1
	const otherOwner int32 = 2

	tests := []struct {
		name     string
		selected any
		want     ResurrectionScrollRefusal
	}{
		{name: "nothing selected", selected: nil, want: ResurrectionScrollInvalidTarget},
		{name: "non-actor selection", selected: struct{}{}, want: ResurrectionScrollInvalidTarget},
		{name: "living player", selected: newScrollPlayer(false).withOffer(false), want: ResurrectionScrollAllowed},
		{name: "living pet of another owner with an offer open", selected: newScrollSummon(false, true, &scrollOwner{id: otherOwner, offer: true}), want: ResurrectionScrollAllowed},
		{name: "living npc", selected: &scrollTarget{kind: actor.KindNPC}, want: ResurrectionScrollAllowed},
		{name: "dead npc is left to the skill target type", selected: &scrollTarget{kind: actor.KindNPC, dead: true}, want: ResurrectionScrollAllowed},
		{name: "dead player", selected: newScrollPlayer(true), want: ResurrectionScrollAllowed},
		{
			name: "dead player in a siege zone wins over festival and an open offer",
			selected: func() *scrollPlayer {
				p := newScrollPlayer(true).withOffer(true)
				p.inSiege, p.festival = true, true
				return p
			}(),
			want: ResurrectionScrollSiege,
		},
		{
			name: "dead festival participant wins over an open offer",
			selected: func() *scrollPlayer {
				p := newScrollPlayer(true).withOffer(false)
				p.festival = true
				return p
			}(),
			want: ResurrectionScrollFestival,
		},
		{name: "dead player with an offer open for its pet", selected: newScrollPlayer(true).withOffer(true), want: ResurrectionScrollPetOfferOpen},
		{name: "dead player with an offer open for itself", selected: newScrollPlayer(true).withOffer(false), want: ResurrectionScrollAlreadyProposed},
		{name: "dead servitor", selected: newScrollSummon(true, false, &scrollOwner{id: otherOwner, offer: true}), want: ResurrectionScrollAllowed},
		{name: "own dead pet with the owner's offer open", selected: newScrollSummon(true, true, &scrollOwner{id: user, offer: true}), want: ResurrectionScrollAllowed},
		{name: "foreign dead pet, owner has no offer", selected: newScrollSummon(true, true, &scrollOwner{id: otherOwner}), want: ResurrectionScrollAllowed},
		{name: "foreign dead pet, owner offer open for the pet", selected: newScrollSummon(true, true, &scrollOwner{id: otherOwner, offer: true, offerForPet: true}), want: ResurrectionScrollAlreadyProposed},
		{name: "foreign dead pet, owner offer open for the owner", selected: newScrollSummon(true, true, &scrollOwner{id: otherOwner, offer: true}), want: ResurrectionScrollOwnerOfferOpen},
		{name: "dead pet without an owner", selected: newScrollSummon(true, true, nil), want: ResurrectionScrollAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResurrectionScrollGate(user, tt.selected); got != tt.want {
				t.Fatalf("ResurrectionScrollGate = %d, want %d", got, tt.want)
			}
		})
	}
}
