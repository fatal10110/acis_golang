package gameservertest

import "testing"

// cleanup registers fn to run when tb's test completes, and lets go of fn
// once it has run. testing keeps a finished parallel test until its parent
// ends, along with every cleanup it registered, even those already run; a
// cleanup that closes over a booted stack would hold its whole world in
// memory until the package's last parallel test, and a large package then
// runs out of memory. Use it for every cleanup that reaches the stack.
func cleanup(tb testing.TB, fn func()) {
	tb.Cleanup(func() {
		run := fn
		fn = nil
		run()
	})
}
