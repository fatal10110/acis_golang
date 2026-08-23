package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// threatFixture boots a server and builds one owner threat table plus two
// attacker combatants, mirroring a monster with two players on its hate list.
type threatFixture struct {
	owner    *attackable.ThreatTable
	first    *hostileHandle
	second   *hostileHandle
	attacker attackable.Combatant
}

func newThreatFixture(t *testing.T) *threatFixture {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	first := srv.SpawnHostileNPCAt(t, firstThreatSpot)
	second := srv.SpawnHostileNPCAt(t, secondThreatSpot)
	return &threatFixture{
		owner:    attackable.NewThreatTable(first),
		first:    first,
		second:   second,
		attacker: second,
	}
}

var (
	firstThreatSpot  = location.Location{X: 160, Y: 20, Z: 30}
	secondThreatSpot = location.Location{X: 260, Y: 20, Z: 30}
)

// TestThreatOrdersAttackersByHateNotDamage pins the ordering rule: the most
// hated attacker is the one with the highest hate, not the highest damage.
func TestThreatOrdersAttackersByHateNotDamage(t *testing.T) {
	f := newThreatFixture(t)
	f.owner.AddDamage(f.first, 100, 10)
	f.owner.AddDamage(f.second, 5, 50)

	most, ok := f.owner.MostHated()
	if !ok || most.Attacker.ObjectID() != f.second.ObjectID() {
		t.Fatalf("most hated = %v (hate %v), want the lower-damage attacker with higher hate", most.Attacker, most.Hate)
	}
}

// TestThreatMostHatedSkipsNonPositiveHate pins that stopped or zeroed hate
// keeps its entry but can no longer win MostHated.
func TestThreatMostHatedSkipsNonPositiveHate(t *testing.T) {
	f := newThreatFixture(t)
	f.owner.AddDamage(f.first, 50, 50)
	f.owner.AddDamage(f.second, 10, 10)

	f.owner.StopHate(f.first)
	if _, ok := f.owner.MostHated(); !ok {
		t.Fatal("MostHated found nothing while another attacker still holds hate")
	}
	most, _ := f.owner.MostHated()
	if most.Attacker.ObjectID() != f.second.ObjectID() {
		t.Fatalf("most hated after stop-hate = %v, want the remaining attacker", most.Attacker)
	}
	if e, ok := f.owner.Get(f.first); !ok || e.Hate != 0 {
		t.Fatalf("stopped entry = %+v ok=%v, want an entry kept at zero hate", e, ok)
	}
}

// TestThreatDecayReducesAllHate pins the decay primitive: one reduction pass
// lowers every attacker's hate by the decay amount.
func TestThreatDecayReducesAllHate(t *testing.T) {
	f := newThreatFixture(t)
	f.owner.AddDamage(f.first, 100, 100)
	f.owner.AddDamage(f.second, 40, 60)

	f.owner.ReduceAllHate(25)

	first, _ := f.owner.Get(f.first)
	second, _ := f.owner.Get(f.second)
	if first.Hate != 75 || second.Hate != 35 {
		t.Fatalf("hates after decay = %v/%v, want 75/35", first.Hate, second.Hate)
	}
	most, ok := f.owner.MostHated()
	if !ok || most.Attacker.ObjectID() != f.first.ObjectID() {
		t.Fatalf("most hated after decay = %v, want the still-most-hated attacker", most.Attacker)
	}
}

// TestThreatRefreshDropsLostAttackers pins the visibility sweep: attackers
// that are no longer visible leave the table; visible ones stay.
func TestThreatRefreshDropsLostAttackers(t *testing.T) {
	f := newThreatFixture(t)
	f.owner.AddDamage(f.first, 100, 100)
	f.owner.AddDamage(f.second, 40, 60)

	visible := map[int32]bool{f.second.ObjectID(): true}
	f.owner.Refresh(func(c attackable.Combatant) bool { return visible[c.ObjectID()] })

	if _, ok := f.owner.Get(f.first); ok {
		t.Fatal("lost attacker kept in threat table after refresh")
	}
	if _, ok := f.owner.Get(f.second); !ok {
		t.Fatal("visible attacker dropped from threat table by refresh")
	}
}
