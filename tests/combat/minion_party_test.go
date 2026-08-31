package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
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

	if err := minion.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := minion.AI().CurrentIntention(); got != ai.IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionFollow)
	}
}
