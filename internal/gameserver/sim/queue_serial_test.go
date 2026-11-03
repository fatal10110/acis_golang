package sim

import (
	"testing"
	"time"
)

// TestQueueSerialNamesTheQueue: a queue given a serial is named by its id,
// a dash and the serial; one without by its id alone.
func TestQueueSerialNamesTheQueue(t *testing.T) {
	in := NewInline(time.Unix(0, 0))
	for _, tc := range []struct {
		q    *Queue
		want string
	}{
		{in.NewQueue("npc").Serial(268435457), "sim: state of queue npc-268435457 touched off its queue"},
		{in.NewQueue("makers"), "sim: state of queue makers touched off its queue"},
	} {
		func() {
			defer func() {
				if got := recover(); got != tc.want {
					t.Fatalf("AssertOwner panic = %v, want %q", got, tc.want)
				}
			}()
			AssertOwner(tc.q)
		}()
	}
}
