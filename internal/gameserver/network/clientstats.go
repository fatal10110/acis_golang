package network

import "time"

// Packet-protection thresholds, mirroring the reference's packet-queue
// protection defaults (Config.CLIENT_PACKET_QUEUE_*): a 160 packets/second
// short-flood ceiling, an 80/s average over the 5-second measure window for
// long floods, and at most 2 detected floods per sliding minute before the
// client is disconnected. The reference hardcodes these too; none are
// file-configurable.
const (
	maxPacketsPerSecond        = 160
	floodMeasureInterval       = 5
	maxAvgPacketsPerSecond     = 80
	maxFloodsPerMin            = 2
	preAuthMaxProcessedPackets = 3
)

// clientStats is one client's per-packet flood accounting. All fields are
// owned by the connection's read loop goroutine; nothing else touches them.
//
// The reference measures packets through a threaded per-client queue; this
// port counts frames at the same point in the read path instead. The
// queue-size accounting and burst cap of the reference have no counterpart
// here because there is no inbound queue to overflow or drain in batches —
// each frame is processed inline with its read.
type clientStats struct {
	packetsInSecond [floodMeasureInterval]int
	head            int
	totalCount      int
	// packetCountStart is the zero time until the first frame arrives,
	// which opens the first one-second counting window.
	packetCountStart time.Time
	floodDetected    bool
	floodStart       time.Time
	floodsInMin      int
	processedPackets int
}

func newClientStats() clientStats {
	return clientStats{head: floodMeasureInterval - 1}
}

// countIncomingPacket records one received frame and reports the drop
// decision: sendActionFailed is true exactly when this frame's arrival
// detected a new flood or a new second of an ongoing one began (the caller
// answers ActionFailed), and drop is true when the frame must not be
// dispatched. Mirrors GameClient.dropPacket + ClientStats.countPacket.
func (s *clientStats) countIncomingPacket(now time.Time) (sendActionFailed, drop bool) {
	s.processedPackets++
	if s.countPacket(now) {
		return true, true
	}
	return false, s.floodDetected
}

// floodsExceeded reports whether more than maxFloodsPerMin floods were
// detected within the sliding minute, the disconnect trigger the reference
// checks before running each received packet.
func (s *clientStats) floodsExceeded() bool {
	return s.floodsInMin > maxFloodsPerMin
}

// countPacket advances the sliding per-second ring and reports whether this
// packet begins (or continues into a new second of) a detected flood: the
// first such packet returns true, later packets of the same second return
// false so ActionFailed goes out at most once per second.
func (s *clientStats) countPacket(now time.Time) bool {
	s.totalCount++
	if now.Sub(s.packetCountStart) > time.Second {
		s.packetCountStart = now

		// Clear the flag if flooding stopped over the last seconds.
		if s.floodDetected && !s.longFloodDetected() && s.packetsInSecond[s.head] < maxPacketsPerSecond/2 {
			s.floodDetected = false
		}

		// Wrap the head around the tail.
		if s.head <= 0 {
			s.head = floodMeasureInterval
		}
		s.head--
		s.totalCount -= s.packetsInSecond[s.head]
		s.packetsInSecond[s.head] = 1
		return s.floodDetected
	}

	count := s.packetsInSecond[s.head] + 1
	s.packetsInSecond[s.head] = count
	if !s.floodDetected {
		shortFlood := count > maxPacketsPerSecond
		if !shortFlood && !s.longFloodDetected() {
			return false
		}
		s.floodDetected = true
		if now.Sub(s.floodStart) > time.Minute {
			s.floodStart = now
			s.floodsInMin = 1
		} else {
			s.floodsInMin++
		}
		return true
	}
	return false
}

// longFloodDetected reports whether the average packet rate over the whole
// measure window exceeds the long-flood ceiling.
func (s *clientStats) longFloodDetected() bool {
	return s.totalCount/floodMeasureInterval > maxAvgPacketsPerSecond
}
