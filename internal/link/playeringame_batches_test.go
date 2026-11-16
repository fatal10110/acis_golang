package link

import (
	"slices"
	"strings"
	"testing"
)

// TestEncodePlayerInGameBatchesSplitsByBytes splits a roster too large for
// one payload: every batch fits, the batches decode back to the roster in
// order, and a batch is cut only where the next account would not fit.
func TestEncodePlayerInGameBatchesSplitsByBytes(t *testing.T) {
	const maxPayload = 100
	roster := []string{"alice", "bob", strings.Repeat("c", 14), "d\U0001F600", strings.Repeat("e", 20), "f", "g"}

	batches, err := EncodePlayerInGameBatches(roster, maxPayload)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) < 2 {
		t.Fatalf("batches = %d, want the roster split", len(batches))
	}
	var got []string
	for i, b := range batches {
		if len(b) > maxPayload {
			t.Fatalf("batch %d is %d bytes, over %d", i, len(b), maxPayload)
		}
		accounts, err := DecodePlayerInGame(b)
		if err != nil {
			t.Fatal(err)
		}
		if i+1 < len(batches) {
			next, _ := DecodePlayerInGame(batches[i+1])
			if len(b)+encodedStringSize(next[0]) <= maxPayload {
				t.Fatalf("batch %d (%d bytes) cut before %q, which fits", i, len(b), next[0])
			}
		}
		got = append(got, accounts...)
	}
	if !slices.Equal(got, roster) {
		t.Fatalf("decoded roster = %v, want %v", got, roster)
	}
}

// TestEncodePlayerInGameBatchesFittingRosterIsOnePayload keeps a roster
// that fits as the single payload EncodePlayerInGame builds.
func TestEncodePlayerInGameBatchesFittingRosterIsOnePayload(t *testing.T) {
	roster := []string{"alice", "bob"}
	want, _ := EncodePlayerInGame(roster)
	batches, err := EncodePlayerInGameBatches(roster, len(want))
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || !slices.Equal(batches[0], want) {
		t.Fatalf("batches = %x, want one %x", batches, want)
	}
	if batches, err := EncodePlayerInGameBatches(nil, len(want)); err != nil || len(batches) != 0 {
		t.Fatalf("empty roster = %d batches, %v; want none", len(batches), err)
	}
}

// TestEncodePlayerInGameBatchesRejectsOversizedAccount fails on an account
// that fits no payload, building nothing.
func TestEncodePlayerInGameBatchesRejectsOversizedAccount(t *testing.T) {
	if batches, err := EncodePlayerInGameBatches([]string{"ok", strings.Repeat("x", 100)}, 64); err == nil {
		t.Fatalf("batches = %d, want an error", len(batches))
	}
}
