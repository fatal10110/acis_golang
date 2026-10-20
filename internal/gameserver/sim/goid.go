package sim

import (
	"bytes"
	"runtime"
	"strconv"
)

// goid parses the calling goroutine's id from its stack header,
// "goroutine 123 [running]:". Ids start at 1 and are never reused.
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
