package combat

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

type countedRewards struct{ calls atomic.Int32 }

func (r *countedRewards) CalculateRewards(attackable.Combatant) { r.calls.Add(1) }

func TestHostileDieAppliesOnceUnderConcurrency(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	rewards := &countedRewards{}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if hostile.Die(nil, rewards) {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if hostile.Die(nil, rewards) || winners.Load() != 1 || rewards.calls.Load() != 1 {
		t.Fatalf("Die results: winners=%d rewards=%d; repeat must be false", winners.Load(), rewards.calls.Load())
	}
	statuses, dies := 0, 0
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			break
		}
		if len(frame) < 5 {
			continue
		}
		if wireReader(frame[1:]).ReadInt32() != hostile.ObjectID() {
			continue
		}
		switch frame[0] {
		case serverpackets.OpcodeStatusUpdate:
			statuses++
		case serverpackets.OpcodeDie:
			dies++
		}
	}
	if statuses != 2 || dies != 1 {
		t.Fatalf("death frames: StatusUpdate=%d Die=%d, want 2 and 1", statuses, dies)
	}
}

func TestPlayerNonlethalSkillDamageSendsSelfStatus(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world")
	}
	player := obj.(interface {
		CurrentHP() int
		SetCP(float64)
		SetSpawnProtection(bool)
		ReduceHP(float64, attackable.Combatant, modelskill.Definition)
	})
	player.SetSpawnProtection(false)
	player.SetCP(0)
	beforeHP := player.CurrentHP()
	player.ReduceHP(1, nil, modelskill.Definition{})
	if got := player.CurrentHP(); got != beforeHP-1 {
		t.Fatalf("HP = %d, want %d", got, beforeHP-1)
	}
	statuses := 0
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			break
		}
		if len(frame) < 5 {
			continue
		}
		if frame[0] == serverpackets.OpcodeDie {
			t.Fatal("nonlethal skill damage sent Die")
		}
		if frame[0] == serverpackets.OpcodeStatusUpdate && wireReader(frame[1:]).ReadInt32() == objID {
			statuses++
		}
	}
	if statuses != 1 {
		t.Fatalf("self StatusUpdate count = %d, want 1", statuses)
	}
}

// TestLethalHitOrdersStatusRewardDie pins what the killer's client sees when
// its skill kills a monster, in the reference's order: the monster's zero-HP
// StatusUpdate (reduceHp's setHp), another from doDie's setHp(0), then the
// exp/SP reward, a post-reward StatusUpdate, and Die (AI DEAD).
func TestLethalHitOrdersStatusRewardDie(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithLevels(levelTableFor(t)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	hostile := spawnRewardedNPC(t, srv, 5000, 25)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(42, false, false))
	readCastStartFrames(t, c, objID, 42, 1, 500, 60_000, hostile.ObjectID())

	var order []string
	for len(order) == 0 || order[len(order)-1] != "die" {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			t.Fatalf("kill sequence stopped after %v", order)
		}
		r := wireReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeStatusUpdate:
			if r.ReadInt32() == hostile.ObjectID() {
				count := r.ReadInt32()
				foundZeroHP := false
				for range count {
					kind, value := r.ReadInt32(), r.ReadInt32()
					if kind == int32(serverpackets.StatusCurrentHP) && value == 0 {
						foundZeroHP = true
					}
				}
				if !foundZeroHP {
					t.Fatal("NPC death StatusUpdate lacked CUR_HP=0")
				}
				order = append(order, "status")
			}
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() == int32(serverpackets.SystemMessageYouEarnedS1ExpAndS2SP) {
				order = append(order, "reward")
			}
		case serverpackets.OpcodeDie:
			if r.ReadInt32() == hostile.ObjectID() {
				order = append(order, "die")
			}
		}
	}
	want := []string{"status", "status", "reward", "status", "die"}
	if !slices.Equal(order, want) {
		t.Fatalf("kill sequence = %v, want %v", order, want)
	}
}

func TestPlayerLethalSkillDamageSendsThreeStatusesBeforeDie(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world")
	}
	player := obj.(interface {
		CurrentHP() int
		SetCP(float64)
		SetSpawnProtection(bool)
		ReduceHP(float64, attackable.Combatant, modelskill.Definition)
	})
	player.SetSpawnProtection(false)
	player.SetCP(0)
	player.ReduceHP(float64(player.CurrentHP()), nil, modelskill.Definition{})

	var order []string
	for len(order) == 0 || order[len(order)-1] != "die" {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			t.Fatalf("player death sequence stopped after %v", order)
		}
		r := wireReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeStatusUpdate:
			if r.ReadInt32() == objID {
				count := r.ReadInt32()
				foundZeroHP := false
				for range count {
					kind, value := r.ReadInt32(), r.ReadInt32()
					if kind == int32(serverpackets.StatusCurrentHP) && value == 0 {
						foundZeroHP = true
					}
				}
				if !foundZeroHP {
					t.Fatal("player death StatusUpdate lacked CUR_HP=0")
				}
				order = append(order, "status")
			}
		case serverpackets.OpcodeDie:
			if r.ReadInt32() == objID {
				order = append(order, "die")
			}
		}
	}
	want := []string{"status", "status", "status", "die"}
	if !slices.Equal(order, want) {
		t.Fatalf("player death sequence = %v, want %v", order, want)
	}
}
