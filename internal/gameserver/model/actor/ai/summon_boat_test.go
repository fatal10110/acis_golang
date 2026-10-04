package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// entranceMove is a summon move controller that also arms entrance follows
// (move.Controller.MaybeStartEntranceFollow), reporting dest out of reach
// when out is set.
type entranceMove struct {
	summonMove
	dest           location.Location
	out            bool
	entranceTarget attackable.Combatant
	cancels        int
	walks          []location.Location
}

// MaybeStartEntranceFollow on the plain summon fake reports every target in
// reach: the summon neither walks nor probes a boat entrance.
func (m *summonMove) MaybeStartEntranceFollow(attackable.Combatant, int) (location.Location, bool) {
	return location.Location{}, false
}

func (m *entranceMove) MaybeStartEntranceFollow(target attackable.Combatant, offset int) (location.Location, bool) {
	m.entranceTarget = target
	m.friendlyRange = offset
	return m.dest, m.out
}

func (m *entranceMove) CancelFollow() { m.cancels++ }

func (m *entranceMove) MoveToLocation(dest location.Location) (bool, error) {
	m.walks = append(m.walks, dest)
	return true, nil
}

// recordingBoats is a BoatEntrance answering every probe with entrance,
// walk.
type recordingBoats struct {
	entrance location.Location
	walk     bool
	probes   []location.Location
	refusals int
}

func (b *recordingBoats) PassBoatEntrance(dest location.Location) (location.Location, bool) {
	b.probes = append(b.probes, dest)
	if !b.walk {
		b.refusals++
	}
	return b.entrance, b.walk
}

func (b *recordingBoats) Refuse() { b.refusals++ }

// rootedSummon is a summon that cannot move.
type rootedSummon struct{ *gateFake }

func (rootedSummon) MovementDisabled() bool { return true }

func newEntranceSummon(actor SummonActor, move *entranceMove, boats *recordingBoats) *Summon {
	brain := NewSummon(actor, move, &recordingAttack{})
	brain.SetBoatEntrance(boats)
	return brain
}

// A summon following its owner walks after it as ever: the boat-entrance
// probe is only for anyone else (SummonMove.java:94-99).
func TestSummonFollowOfOwnerSkipsBoatEntrance(t *testing.T) {
	owner := gatePlayerFake(1, 1, 0)
	move := &entranceMove{out: true}
	boats := &recordingBoats{}
	brain := newEntranceSummon(gateSummonFake(2, owner), move, boats)

	if !brain.TryToFollow(owner) {
		t.Fatal("TryToFollow(owner) = false, want accepted")
	}
	if move.friendlyTarget != owner || move.entranceTarget != nil || len(boats.probes) != 0 {
		t.Fatalf("owner follow: friendly %v, entrance %v, probes %v; want a plain friendly follow", move.friendlyTarget, move.entranceTarget, boats.probes)
	}
}

// A summon following anyone else out of reach never walks after it: with
// no boat entrance on its way the owner is answered ActionFailed and the
// summon stays, following; in reach nothing is probed.
func TestSummonFollowOfOtherProbesBoatEntrance(t *testing.T) {
	owner := gatePlayerFake(1, 1, 0)
	other := actor(200)
	move := &entranceMove{dest: location.Location{X: 500, Y: 10, Z: 20}, out: true}
	boats := &recordingBoats{}
	brain := newEntranceSummon(gateSummonFake(2, owner), move, boats)

	if !brain.TryToFollow(other) {
		t.Fatal("TryToFollow(other) = false, want accepted")
	}
	if move.entranceTarget != other || move.friendlyRange != summonFollowOffset || move.friendlyTarget != nil {
		t.Fatalf("follow of another: entrance %v at %d, friendly %v; want an entrance follow at %d", move.entranceTarget, move.friendlyRange, move.friendlyTarget, summonFollowOffset)
	}
	if len(boats.probes) != 1 || boats.probes[0] != move.dest || boats.refusals != 1 {
		t.Fatalf("probes %v, refusals %d; want one probe toward %v refused", boats.probes, boats.refusals, move.dest)
	}
	if len(move.walks) != 0 || brain.CurrentIntention() != IntentionFollow {
		t.Fatalf("walks %v, intention %v; want no walk, still following", move.walks, brain.CurrentIntention())
	}

	move.out = false
	brain.Think()
	if len(boats.probes) != 1 {
		t.Fatalf("probes in reach = %d, want none more", len(boats.probes)-1)
	}
}

// Crossing a dock's entrance on its way, the summon walks to the entrance
// instead, the walk replacing the follow (Playable.moveToBoatEntrance,
// PlayableAI.tryToMoveTo); a summon that cannot move goes idle, its owner
// answered ActionFailed (PlayableAI.thinkMoveTo).
func TestSummonFollowOfOtherWalksToBoatEntrance(t *testing.T) {
	entrance := location.Location{X: 300, Y: 40, Z: -3624}
	owner := gatePlayerFake(1, 1, 0)

	move := &entranceMove{dest: location.Location{X: 500}, out: true}
	boats := &recordingBoats{entrance: entrance, walk: true}
	brain := newEntranceSummon(gateSummonFake(2, owner), move, boats)
	brain.TryToFollow(actor(200))
	if len(move.walks) != 1 || move.walks[0] != entrance || move.cancels != 1 {
		t.Fatalf("walks %v, follow cancels %d; want one walk to %v replacing the follow", move.walks, move.cancels, entrance)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("intention = %v, want move-to", got)
	}

	rootedMove := &entranceMove{dest: location.Location{X: 500}, out: true}
	rootedBoats := &recordingBoats{entrance: entrance, walk: true}
	rooted := newEntranceSummon(rootedSummon{gateSummonFake(3, owner)}, rootedMove, rootedBoats)
	rooted.TryToFollow(actor(200))
	if len(rootedMove.walks) != 0 || rootedBoats.refusals != 1 || rooted.CurrentIntention() != IntentionIdle {
		t.Fatalf("rooted: walks %v, refusals %d, intention %v; want idle, refused", rootedMove.walks, rootedBoats.refusals, rooted.CurrentIntention())
	}
}
