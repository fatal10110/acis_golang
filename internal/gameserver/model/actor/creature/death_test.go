package creature

import (
	"sync"
	"testing"
)

type deathTestActor struct {
	id   int32
	mu   sync.Mutex
	dead bool
}

func (a *deathTestActor) ObjectID() int32 { return a.id }

func (a *deathTestActor) MarkDead() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dead {
		return false
	}
	a.dead = true
	return true
}

type recordingRewarder struct {
	calls []DeathActor
}

func (r *recordingRewarder) CalculateRewards(killer DeathActor) {
	r.calls = append(r.calls, killer)
}

func TestDieAppliesOnce(t *testing.T) {
	actor := &deathTestActor{id: 1}
	killer := &deathTestActor{id: 2}
	rewards := &recordingRewarder{}

	if !Die(actor, killer, rewards) {
		t.Fatal("Die() = false, want true on first kill")
	}
	if len(rewards.calls) != 1 || rewards.calls[0] != killer {
		t.Fatalf("rewards.calls = %v, want one call with killer", rewards.calls)
	}

	if Die(actor, killer, rewards) {
		t.Fatal("Die() = true, want false on repeat kill")
	}
	if len(rewards.calls) != 1 {
		t.Fatalf("rewards.calls after repeat = %v, want unchanged", rewards.calls)
	}
}

func TestDieNilRewarderIsNoOp(t *testing.T) {
	actor := &deathTestActor{id: 1}
	if !Die(actor, nil, nil) {
		t.Fatal("Die() = false, want true with nil killer/rewards")
	}
}

func TestDieConcurrentOnlyOneWinner(t *testing.T) {
	actor := &deathTestActor{id: 1}
	rewards := &recordingRewarder{}

	const attempts = 50
	results := make(chan bool, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- Die(actor, nil, rewards)
		}()
	}
	wg.Wait()
	close(results)

	wins := 0
	for r := range results {
		if r {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want exactly 1", wins)
	}
}
