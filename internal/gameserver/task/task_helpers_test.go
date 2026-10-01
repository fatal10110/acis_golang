package task

import (
	"fmt"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// testLoop runs the queue of every fake actor built without one of its own;
// a test calls testLoop.Run after a tick to run the work it posted. No test
// in this package runs in parallel, so the tests share it.
var (
	testLoop  = sim.NewInline(time.UnixMilli(0))
	testQueue = testLoop.NewQueue("test")
)

// ---- from shadowitem_test.go ----
type shadowItemFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *shadowItemFakeEffects) ManaThreshold(actorID int32, inst *item.Instance, secondsLeft int) {
	e.record(fmt.Sprintf("%d threshold %d %d", actorID, inst.ObjectID, secondsLeft))
}

func (e *shadowItemFakeEffects) Expire(actorID int32, inst *item.Instance) {
	e.record(fmt.Sprintf("%d expire %d", actorID, inst.ObjectID))
}

func (e *shadowItemFakeEffects) record(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, s)
}

func (e *shadowItemFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}
