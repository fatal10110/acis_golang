package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func partyParams(partyType int) *commons.StatSet {
	set := commons.NewStatSet()
	set.Set("Party_Type", partyType)
	return set
}

func TestMinionAssistsMasterOnCombatDamage(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	master := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	minion := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	attacker := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 80, Y: hostileY, Z: hostileZ})
	master.Instance.Template.AIParams = partyParams(2)
	minion.Instance.Template.AIParams = partyParams(1)
	minion.Instance.Template.CanMove = true
	master.AddMinion(minion)
	minion.SetMaster(master)

	master.TakeDamage(30, attacker)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire after master damage = (%v, %+v), want attack on attacker", ok, d)
	}
	if !d.MoveToTarget {
		t.Fatal("moving party assist MoveToTarget = false, want true")
	}
}

func TestPartyPrivateFollowsMasterWhenIdle(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	master := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	minion := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 200, Y: hostileY, Z: hostileZ})
	master.Instance.Template.AIParams = partyParams(2)
	minion.Instance.Template.AIParams = partyParams(1)
	minion.Instance.Template.CanMove = true
	master.AddMinion(minion)
	minion.SetMaster(master)

	tickThinkWander(t, minion)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionFollow)
	}
}

func partyParamsMoving(partyType, movingAttack int) *commons.StatSet {
	set := partyParams(partyType)
	set.Set("MovingAttack", movingAttack)
	return set
}

func liveCombatant(t *testing.T, srv *gameservertest.Server) attackable.Combatant {
	t.Helper()
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("live player missing from world")
	}
	combatant, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("live player %T does not implement Combatant", obj)
	}
	return combatant
}

func TestStationaryMinionHoldsAttackWhenPlayerInRange(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	player := liveCombatant(t, srv)
	master := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	minion := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	master.Instance.Template.AIParams = partyParams(2)
	minion.Instance.Template.AIParams = partyParamsMoving(1, 0)
	minion.Instance.Template.AggroRange = 500
	minion.Instance.Template.CanMove = true
	master.AddMinion(minion)
	minion.SetMaster(master)

	master.TakeDamage(30, player)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget.ObjectID() != player.ObjectID() {
		t.Fatalf("minion desire after master damage = (%v, %+v), want hold attack on player", ok, d)
	}
	if d.MoveToTarget {
		t.Fatal("stationary party assist MoveToTarget = true, want false")
	}
	if got := d.Weight; got != 30 {
		t.Fatalf("hold attack weight = %v, want 30", got)
	}
}

func TestStationaryMinionDropsAttackWhenPlayerOutOfRange(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	player := liveCombatant(t, srv)
	master := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	minion := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	master.Instance.Template.AIParams = partyParams(2)
	minion.Instance.Template.AIParams = partyParamsMoving(1, 0)
	minion.Instance.Template.AggroRange = 10
	minion.Instance.Template.CanMove = true
	master.AddMinion(minion)
	minion.SetMaster(master)

	minion.AddCombatDamageHate(player, 10)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() before assist = %v, want Attack so the player is top desire", got)
	}

	master.TakeDamage(30, player)

	if got := minion.AI().Desires().Len(); got != 0 {
		t.Fatalf("desires after out-of-range hold assist = %d, want 0", got)
	}
}
