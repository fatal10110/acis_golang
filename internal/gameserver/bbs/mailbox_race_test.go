package bbs

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// inlineWriter runs every write job at once, on the caller.
type inlineWriter struct{}

func (inlineWriter) Enqueue(_ int32, job func()) bool {
	job()
	return true
}

// recordingStore records the ids of the rows inserted.
type recordingStore struct {
	mu       sync.Mutex
	inserted []int32
}

func (s *recordingStore) InsertMail(_ context.Context, m Mail) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inserted = append(s.inserted, m.ID)
	return nil
}

func (*recordingStore) DeleteMail(context.Context, int32) error       { return nil }
func (*recordingStore) MarkMailRead(context.Context, int32) error     { return nil }
func (*recordingStore) MoveMail(context.Context, int32, Folder) error { return nil }

// TestMailboxConcurrentSendsKeepInboxCap drives many senders' queues
// filing mail into one inbox that holds 99 mails while its owner's queue
// reads, marks, moves and deletes in it. The inbox never holds more than
// inboxCapacity mails, every id is given once, and every delivery the cap
// refused is accounted for.
func TestMailboxConcurrentSendsKeepInboxCap(t *testing.T) {
	const (
		owner         = int32(1)
		senders       = 16
		sendsEach     = 5
		ownerRounds   = 20
		seededInbox   = inboxCapacity - 1
		firstSenderID = int32(1000)
	)
	store := &recordingStore{}
	box := NewMailbox(store, inlineWriter{}, zerolog.Nop())
	now := time.Now()
	seed := make([]Mail, seededInbox)
	for i := range seed {
		seed[i] = Mail{ID: int32(i + 1), ReceiverID: owner, SenderID: 2, Folder: Inbox, Subject: "s", Message: "m", Sent: now, Unread: true}
	}
	box.Restore(seed)

	inboxOf := func(mails []Mail) int {
		n := 0
		for _, m := range mails {
			if m.Folder == Inbox {
				n++
			}
		}
		return n
	}

	var (
		wg              sync.WaitGroup
		mu              sync.Mutex
		delivered, full int
		sentCopies      int
	)
	start := make(chan struct{})
	for s := range senders {
		wg.Go(func() {
			<-start
			sender := Sender{ID: firstSenderID + int32(s)}
			for range sendsEach {
				deliveries, copied := box.Send(sender, []Recipient{{Name: "owner", ID: owner}}, "owner", "s", "m", now)
				mu.Lock()
				for _, d := range deliveries {
					switch d.Result {
					case Delivered:
						delivered++
					case DeliveryInboxFull:
						full++
					default:
						t.Errorf("delivery result = %d, want delivered or inbox full", d.Result)
					}
				}
				if copied {
					sentCopies++
				}
				mu.Unlock()
			}
		})
	}

	moved, deleted := 0, 0
	wg.Go(func() {
		<-start
		for round := range ownerRounds {
			mails := box.Box(owner)
			if n := inboxOf(mails); n > inboxCapacity {
				t.Errorf("round %d: inbox holds %d mails, want at most %d", round, n, inboxCapacity)
			}
			for _, m := range mails {
				if m.Folder != Inbox {
					continue
				}
				box.MarkRead(owner, m.ID)
				if round%2 == 0 {
					box.Move(owner, m.ID, Archive)
					moved++
				} else {
					box.Delete(owner, m.ID)
					deleted++
				}
				break
			}
		}
	})
	close(start)
	wg.Wait()

	if got := delivered + full; got != senders*sendsEach {
		t.Fatalf("delivered %d + refused full %d = %d, want %d attempts", delivered, full, got, senders*sendsEach)
	}
	if sentCopies != delivered {
		t.Fatalf("sent-box copies = %d, want one per delivered mail (%d)", sentCopies, delivered)
	}
	inbox := box.Box(owner)
	if got, want := inboxOf(inbox), seededInbox+delivered-moved-deleted; got != want {
		t.Fatalf("final inbox = %d mails, want %d seeded + %d delivered - %d moved - %d deleted = %d",
			got, seededInbox, delivered, moved, deleted, want)
	}
	if n := inboxOf(inbox); n > inboxCapacity {
		t.Fatalf("final inbox holds %d mails, want at most %d", n, inboxCapacity)
	}

	seen := map[int32]bool{}
	all := append([]Mail(nil), inbox...)
	for s := range senders {
		all = append(all, box.Box(firstSenderID+int32(s))...)
	}
	for _, m := range all {
		if seen[m.ID] {
			t.Fatalf("mail id %d filed twice", m.ID)
		}
		seen[m.ID] = true
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if got, want := len(store.inserted), 2*delivered; got != want {
		t.Fatalf("rows inserted = %d, want %d (a recipient copy and a sent copy per delivery)", got, want)
	}
	for _, id := range store.inserted {
		if id <= seededInbox {
			t.Fatalf("inserted row id %d reuses a seeded id", id)
		}
	}
}
