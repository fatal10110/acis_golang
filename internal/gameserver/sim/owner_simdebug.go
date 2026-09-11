//go:build simdebug

package sim

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
)

// drainers maps a goroutine id to the queue it is draining.
var drainers sync.Map

func enterDrain(q *Queue) { drainers.Store(goid(), q) }

func exitDrain() { drainers.Delete(goid()) }

func assertDrainer(q *Queue) {
	if owner, _ := drainers.Load(goid()); owner != q {
		panic("sim: state of queue " + q.id + " touched while another goroutine drains it")
	}
}

// goid parses the calling goroutine's id from its stack header,
// "goroutine 123 [running]:".
func goid() uint64 {
	var buf [64]byte
	b := bytes.TrimPrefix(buf[:runtime.Stack(buf[:], false)], []byte("goroutine "))
	if i := bytes.IndexByte(b, ' '); i >= 0 {
		b = b[:i]
	}
	id, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil {
		panic("sim: unparseable goroutine header: " + err.Error())
	}
	return id
}
