package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestMovingHostileRefDoesNotExposeHostileWorldMethods(t *testing.T) {
	var ref any = &movingHostileLocatedRef{}
	if _, ok := ref.(interface {
		Knows(attackable.Combatant) bool
	}); ok {
		t.Fatal("moving hostile ref exposes Knows, unlike production locatedRef")
	}
	if _, ok := ref.(world.Tracked); ok {
		t.Fatal("moving hostile ref is world-tracked, unlike production locatedRef")
	}
}
