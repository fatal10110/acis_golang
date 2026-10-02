package bbs

import (
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestUnstoredSendsCountForADay pins a delivered mail too wide to store
// counting against its sender's daily limit for one day, then no more.
func TestUnstoredSendsCountForADay(t *testing.T) {
	box := NewMailbox(nil, nil, zerolog.Nop())
	sender := Sender{ID: 1}
	to := []Recipient{{Name: "b", ID: 2}}
	long := strings.Repeat("m", messageColumnWidth+1)
	start := time.Now()
	for i := range dailySendLimit {
		if _, copied := box.Send(sender, to, "b", "s", long, start.Add(time.Duration(i)*time.Minute)); copied {
			t.Fatalf("send %d filed a sent-box copy, want none for an unstored mail", i+1)
		}
	}
	if got := box.CheckSend(sender.ID, false, []string{"b"}, start.Add(time.Hour)); got != SendDailyLimit {
		t.Fatalf("check after %d unstored sends = %d, want SendDailyLimit", dailySendLimit, got)
	}
	if got := box.CheckSend(sender.ID, false, []string{"b"}, start.Add(24*time.Hour+time.Minute)); got != SendAllowed {
		t.Fatalf("check a day after the first send = %d, want SendAllowed", got)
	}
	// A refused send (no recipient took it) does not count.
	box2 := NewMailbox(nil, nil, zerolog.Nop())
	for range dailySendLimit {
		box2.Send(sender, []Recipient{{Name: "x"}}, "x", "s", long, start)
	}
	if got := box2.CheckSend(sender.ID, false, []string{"b"}, start); got != SendAllowed {
		t.Fatalf("check after undelivered sends = %d, want SendAllowed", got)
	}
}
