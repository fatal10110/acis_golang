package network

import (
	"testing"
	"time"
)

// addPackets feeds n packets all stamped within the same instant.
func addPackets(s *clientStats, at time.Time, n int) {
	for i := 0; i < n; i++ {
		s.countIncomingPacket(at)
	}
}

func TestClientStatsAllowsNormalPacketRates(t *testing.T) {
	s := newClientStats()
	start := time.Unix(0, 0)
	addPackets(&s, start, maxPacketsPerSecond)
	if s.floodDetected {
		t.Fatal("flood detected at the ceiling packets/second, want none")
	}
}

func TestClientStatsShortFloodDropsWithOneActionFailedPerSecond(t *testing.T) {
	s := newClientStats()
	start := time.Unix(0, 0)

	// Fill the first one-second window exactly to the ceiling.
	addPackets(&s, start, maxPacketsPerSecond)

	// The packet pushing past the ceiling detects the flood and answers
	// ActionFailed once.
	actionFailed, drop := s.countIncomingPacket(start)
	if !actionFailed || !drop {
		t.Fatalf("flood-onset packet: actionFailed=%v drop=%v, want both true", actionFailed, drop)
	}
	if !s.floodDetected {
		t.Fatal("flood not flagged after exceeding the per-second ceiling")
	}

	// Later packets of the same second are dropped silently.
	for i := 0; i < 10; i++ {
		if actionFailed, drop := s.countIncomingPacket(start); actionFailed || !drop {
			t.Fatalf("flood packet %d: actionFailed=%v drop=%v, want silent drop", i, actionFailed, drop)
		}
	}
}

func TestClientStatsActionFailedOncePerSecondDuringOngoingFlood(t *testing.T) {
	s := newClientStats()
	start := time.Unix(0, 0)

	addPackets(&s, start, maxPacketsPerSecond+1)
	if !s.floodDetected {
		t.Fatal("flood not detected after burst above the ceiling")
	}

	// A new one-second window during an ongoing flood answers ActionFailed
	// exactly once, then drops silently again.
	next := start.Add(1500 * time.Millisecond)
	if actionFailed, drop := s.countIncomingPacket(next); !actionFailed || !drop {
		t.Fatalf("first packet of next window: actionFailed=%v drop=%v, want ActionFailed and drop", actionFailed, drop)
	}
	if actionFailed, drop := s.countIncomingPacket(next); actionFailed || !drop {
		t.Fatalf("second packet of next window: actionFailed=%v drop=%v, want silent drop", actionFailed, drop)
	}
}

func TestClientStatsFloodClearsAfterRateFalls(t *testing.T) {
	s := newClientStats()
	start := time.Unix(0, 0)

	addPackets(&s, start, maxPacketsPerSecond+1)
	if !s.floodDetected {
		t.Fatal("flood not detected after burst above the ceiling")
	}

	// Walk the measure window forward with a near-silent rate: once the
	// flooded second-slot retires and the window average falls back under
	// the long-flood line, the flag clears.
	now := start
	cleared := false
	for i := 0; i < 3*floodMeasureInterval && !cleared; i++ {
		now = now.Add(1100 * time.Millisecond)
		s.countIncomingPacket(now)
		cleared = !s.floodDetected
	}
	if !cleared {
		t.Fatal("flood still flagged after several quiet seconds")
	}
	if actionFailed, drop := s.countIncomingPacket(now); actionFailed || drop {
		t.Fatalf("post-flood packet: actionFailed=%v drop=%v, want both false", actionFailed, drop)
	}
}

func TestClientStatsLongFloodDetectsSustainedAverage(t *testing.T) {
	s := newClientStats()
	start := time.Unix(0, 0)

	// Sustain 100 packets/second: never trips the 160/s short ceiling but
	// pushes the measure-window average past the 80/s long-flood line once
	// several second-slots are populated.
	now := start
	detected := false
	for i := 0; i < 6*floodMeasureInterval && !detected; i++ {
		addPackets(&s, now, 100)
		for p := 0; p < 10; p++ {
			if _, drop := s.countIncomingPacket(now); drop {
				detected = true
				break
			}
		}
		now = now.Add(time.Second)
	}
	if !detected {
		t.Fatal("long flood never detected at a sustained 100 packets/second average")
	}
}

// burstThenQuiet drives one full flood onset (burst above the ceiling,
// ActionFailed observed) followed by enough quiet seconds for the flood
// flag to clear, so the next burst counts as a distinct flood.
func burstThenQuiet(s *clientStats, start time.Time) {
	addPackets(s, start, maxPacketsPerSecond+1)
	now := start
	for i := 0; i < 2*floodMeasureInterval && s.floodDetected; i++ {
		now = now.Add(1100 * time.Millisecond)
		s.countIncomingPacket(now)
	}
	if s.floodDetected {
		panic("flood flag failed to clear during quiet gap")
	}
}

func TestClientStatsDisconnectsAfterTooManyFloodsPerMinute(t *testing.T) {
	s := newClientStats()
	start := time.Unix(0, 0)

	burstThenQuiet(&s, start)
	burstThenQuiet(&s, start.Add(5*time.Second))
	if s.floodsExceeded() {
		t.Fatal("disconnect triggered after two floods, want tolerance up to two")
	}

	// A third flood inside the same minute crosses the disconnect line;
	// its onset also reports ActionFailed before the caller closes the
	// connection.
	addPackets(&s, start.Add(10*time.Second), maxPacketsPerSecond)
	actionFailed, _ := s.countIncomingPacket(start.Add(10 * time.Second))
	if !actionFailed {
		t.Fatal("third flood onset did not report ActionFailed")
	}
	if !s.floodsExceeded() {
		t.Fatal("disconnect not triggered after three floods within a minute")
	}
}

func TestPreAuthCapThreshold(t *testing.T) {
	s := newClientStats()
	now := time.Now()
	for i := 0; i < preAuthMaxProcessedPackets; i++ {
		s.countIncomingPacket(now)
	}
	if s.processedPackets != preAuthMaxProcessedPackets {
		t.Fatalf("processedPackets = %d after %d packets, want %d", s.processedPackets, preAuthMaxProcessedPackets, preAuthMaxProcessedPackets)
	}
	s.countIncomingPacket(now)
	if s.processedPackets <= preAuthMaxProcessedPackets {
		t.Fatalf("processedPackets = %d, want above the pre-auth cap", s.processedPackets)
	}
}
