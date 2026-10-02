package target

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// partySweeper is a player caster that shares loot with the players in
// group: its party or command channel.
type partySweeper struct {
	*targetActor
	group map[int32]bool
}

func (p partySweeper) IsLooterOrInLooterParty(ownerID int32) bool {
	return ownerID == p.id || p.group[ownerID]
}

// Reference: PlayerCast.canCast, case SWEEP, gates on
// Player.isLooterOrInLooterParty(spoilerId): a member of the spoiler's
// party or command channel may sweep its spoil, anyone else may not.
func TestSweepAdmitsTheSpoilersParty(t *testing.T) {
	sweep := &modelskill.Definition{SkillType: "SWEEP"}
	spoiled := &targetActor{id: 11, kind: actor.KindNPC, corpse: true, monster: true, spoiled: true, spoiler: 1}
	member := partySweeper{targetActor: &targetActor{id: 2, kind: actor.KindPlayer}, group: map[int32]bool{1: true}}
	stranger := partySweeper{targetActor: &targetActor{id: 3, kind: actor.KindPlayer}, group: map[int32]bool{4: true}}

	if got := CastRejectionFor(modelskill.TargetCorpseMob, member, spoiled, sweep, false); got != CastRejectNone {
		t.Fatalf("spoiler's party member: CastRejectionFor = %v, want none", got)
	}
	if got := CastRejectionFor(modelskill.TargetCorpseMob, stranger, spoiled, sweep, false); got != CastRejectSweepNotAllowed {
		t.Fatalf("outside the spoiler's party: CastRejectionFor = %v, want sweep not allowed", got)
	}
}
