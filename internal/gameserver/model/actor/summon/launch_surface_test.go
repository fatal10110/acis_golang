package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type summonPeaceZoneQuery struct {
	result               bool
	regionX, regionY     int
	x, y, z, effectRange int
}

func (q *summonPeaceZoneQuery) EffectRangeInPeaceZone(regionX, regionY, x, y, z, effectRange int) bool {
	q.regionX, q.regionY = regionX, regionY
	q.x, q.y, q.z, q.effectRange = x, y, z, effectRange
	return q.result
}

func TestActorLaunchSurfacesUseTemplateRadiusAndOwnRegion(t *testing.T) {
	actor := NewPet(PetConfig{ObjectID: 1, CollisionRadius: 12.5})
	state := world.New()
	state.Spawn(actor, 100, 200, 300, 0)
	zones := &summonPeaceZoneQuery{result: true}
	actor.SetZones(zones)

	if got := actor.CollisionRadius(); got != 12.5 {
		t.Fatalf("CollisionRadius() = %v, want 12.5", got)
	}
	if !actor.EffectRangeInPeaceZone(10, 20, 30, 40) {
		t.Fatal("EffectRangeInPeaceZone() = false, want zone result")
	}
	if zones.regionX != 100 || zones.regionY != 200 {
		t.Fatalf("region anchor = (%d,%d), want own position (100,200)", zones.regionX, zones.regionY)
	}
	if zones.x != 10 || zones.y != 20 || zones.z != 30 || zones.effectRange != 40 {
		t.Fatalf("query = (%d,%d,%d,%d), want (10,20,30,40)", zones.x, zones.y, zones.z, zones.effectRange)
	}
}
