package task

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// testLoop runs the queue of every fake actor built without one of its own;
// a test calls testLoop.Run after a tick to run the work it posted. No test
// in this package runs in parallel, so the tests share it.
var (
	testLoop  = sim.NewInline(time.UnixMilli(0))
	testQueue = testLoop.NewQueue("test")
)
