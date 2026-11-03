package quest

import (
	"context"
	"maps"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// startQ001 starts Q001 at condition 1 for objID, on its own queue, and
// returns its character.
func startQ001(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	var char *player.Character
	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		st := q.NewState(c, sc)
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		char = c
	})
	srv.ReadQueued(t, srv.Client)
	return char
}

// detachDuringHook sends the player objID back to character select while a
// hook raised off its queue is held between its writes: hook runs on its
// own goroutine, calling hold once it has made its first write, and hold
// returns once the detach has stopped for the hook, or has finished. It
// reports whether the detach went on past marking the player while the hook
// was held.
func detachDuringHook(t *testing.T, srv *gameservertest.Server, objID int32, hook func(sc *script.Script, p *script.Player, hold func())) (pastMark bool) {
	t.Helper()
	invoke := srv.ScriptOffQueue(t, objID, q001)
	detaching := srv.PlayerDetaching(t, objID)
	held, release := make(chan struct{}), make(chan struct{})
	returned := make(chan bool, 1)
	go func() {
		returned <- invoke(func(sc *script.Script, p *script.Player) {
			hook(sc, p, func() {
				close(held)
				<-release
			})
		})
	}()
	<-held

	var restarted, past atomic.Bool
	go func() {
		defer close(release)
		for !restarted.Load() {
			// A stop before the mark is set is some other step of the
			// detach waiting briefly: keep looking.
			if !detaching() {
				runtime.Gosched()
				continue
			}
			if parked, inMark := parkedDetach(); parked {
				past.Store(!inMark)
				return
			}
			runtime.Gosched()
		}
	}()
	restart(t, srv)
	restarted.Store(true)
	if !<-returned {
		t.Fatal("the hook panicked")
	}
	return past.Load()
}

// TestScriptHookFromAnotherQueueRacingDetachLandsWhole raises, off the
// player's queue, a hook that gives an item and then moves the quest on,
// and sends the player back to character select between the two. The
// detach must stop inside MarkDetaching until the hook has ended, so both
// writes are in its final saves: the item and the new condition are in the
// rows and on reselect. A detach that sealed the journal at once would keep
// the item and refuse the condition, so the player could claim the step's
// reward again.
//
// It runs alone, not in parallel: the detach it waits for is found among
// the process's goroutines, and no other suite's may be there.
func TestScriptHookFromAnotherQueueRacingDetachLandsWhole(t *testing.T) {
	srv, objID := bootHelpers(t)
	startQ001(t, srv, objID)

	pastMark := detachDuringHook(t, srv, objID, func(sc *script.Script, p *script.Player, hold func()) {
		st := sc.QuestState(p, q001)
		sc.GiveItems(p, dropItem, 1)
		hold()
		st.SetCond(2)
	})
	if pastMark {
		t.Error("the detach went past marking the player while a hook that wrote it was running")
	}
	srv.FlushItems(t)
	srv.FlushPersistence(t)

	if got := itemRows(t, srv, objID); !maps.Equal(got, map[int32]int{dropItem: 1}) {
		t.Errorf("item rows after the detach = %v, want the given item", got)
	}
	wantQuest := map[string]string{"<state>": "STARTED", "<cond>": "2"}
	if got := questRows(t, srv, objID, q001); !maps.Equal(got, wantQuest) {
		t.Errorf("quest rows after the detach = %v, want %v", got, wantQuest)
	}
	enterWorld(t, srv)
	if got := srv.PlayerItemCount(t, objID, dropItem); got != 1 {
		t.Errorf("reselected holding %d of the given item, want 1", got)
	}
}

// progressionRow reads objID's saved exp, sp and PK count.
func progressionRow(t *testing.T, srv *gameservertest.Server, objID int32) (exp int64, sp, pk int) {
	t.Helper()
	err := srv.DB.QueryRowContext(context.Background(), "SELECT exp, sp, pkkills FROM characters WHERE obj_Id = ?", objID).Scan(&exp, &sp, &pk)
	if err != nil {
		t.Fatal(err)
	}
	return exp, sp, pk
}

// TestScriptExpAndPkFromAnotherQueueRacingDetachAreSaved raises, off the
// player's queue, a hook that rewards exp and sp, then sets the PK count
// and rewards again, and sends the player back to character select between
// the reward and the rest. Every write lands before the detach's character
// save: the row holds what the character held when it left. A hook raised
// once the player has left changes nothing, not even in memory.
//
// It runs alone, not in parallel: the detach it waits for is found among
// the process's goroutines, and no other suite's may be there.
func TestScriptExpAndPkFromAnotherQueueRacingDetachAreSaved(t *testing.T) {
	srv, objID := bootHelpers(t)
	char := startQ001(t, srv, objID)
	later := srv.ScriptOffQueue(t, objID, q001)

	pastMark := detachDuringHook(t, srv, objID, func(sc *script.Script, p *script.Player, hold func()) {
		sc.RewardExpAndSp(p, 1000, 100)
		hold()
		p.SetPkKills(7)
		sc.RewardExpAndSp(p, 500, 50)
	})
	if pastMark {
		t.Error("the detach went past marking the player while a hook that wrote it was running")
	}
	srv.FlushPersistence(t)

	left := char.ProgressionValues()
	exp, sp, pk := progressionRow(t, srv, objID)
	if sp != 150 || pk != 7 {
		t.Errorf("saved sp, pk = %d, %d, want 150, 7", sp, pk)
	}
	if exp != left.Exp || sp != left.SP || pk != left.PKKills {
		t.Errorf("saved exp, sp, pk = %d, %d, %d, the character left with %d, %d, %d", exp, sp, pk, left.Exp, left.SP, left.PKKills)
	}

	if !later(func(sc *script.Script, p *script.Player) {
		sc.RewardExpAndSp(p, 1000, 100)
		p.SetPkKills(9)
	}) {
		t.Fatal("the hook raised after the detach panicked")
	}
	if after := char.ProgressionValues(); after.Exp != left.Exp || after.SP != left.SP || after.PKKills != left.PKKills {
		t.Errorf("a hook raised after the detach changed the character: exp, sp, pk = %d, %d, %d", after.Exp, after.SP, after.PKKills)
	}
}
