package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/rs/zerolog"
)

// rejectLog is a frameRejectReporter on a hand-driven clock whose
// diagnostics a test reads back.
type rejectLog struct {
	reporter *frameRejectReporter
	now      time.Time
	mu       sync.Mutex
	buf      bytes.Buffer
}

func newRejectLog() *rejectLog {
	r := &rejectLog{now: time.Unix(1_000_000, 0)}
	r.reporter = newFrameRejectReporter(func() time.Time { return r.now })
	r.reporter.setLog(zerolog.New(r))
	return r
}

func (r *rejectLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

type rejectLine struct {
	Level      string `json:"level"`
	Error      string `json:"error"`
	Boundary   string `json:"boundary"`
	Suppressed int    `json:"suppressed"`
}

func (r *rejectLog) lines(t *testing.T) []rejectLine {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []rejectLine
	for _, raw := range strings.Split(strings.TrimSpace(r.buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line rejectLine
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("diagnostic %q is not JSON: %v", raw, err)
		}
		out = append(out, line)
	}
	return out
}

// TestFrameRejectReporterBoundsDiagnostics pins that rejected frames log at
// most once per frameRejectInterval, the next line counting the ones
// swallowed in between.
func TestFrameRejectReporterBoundsDiagnostics(t *testing.T) {
	r := newRejectLog()
	first := errors.New("first cause")
	r.reporter.report("broadcast", first)
	for range 5 {
		r.reporter.report("session send", errors.New("swallowed"))
	}
	r.now = r.now.Add(frameRejectInterval - time.Nanosecond)
	r.reporter.report("session send", errors.New("still swallowed"))
	r.now = r.now.Add(time.Nanosecond)
	r.reporter.report("connection send", errors.New("second cause"))

	want := []rejectLine{
		{Level: "error", Error: "first cause", Boundary: "broadcast", Suppressed: 0},
		{Level: "error", Error: "second cause", Boundary: "connection send", Suppressed: 6},
	}
	if got := r.lines(t); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("diagnostics = %+v, want %+v", got, want)
	}
}

// countingReceiver counts the copies a broadcast hands it.
type countingReceiver struct{ got int }

func (c *countingReceiver) BroadcastFrame(frame wire.Frame) bool {
	c.got++
	frame.Release()
	return true
}

// TestBroadcastFanoutReportsRejectedSourceOnce pins #1819 at the broadcast
// boundary: a source frame no copy can be made of — too short for its
// header, or invalid with its own Err — is reported once per broadcast, not
// per recipient, with the source's cause; no recipient gets anything; and the
// source is still released exactly once, whether built lazily or handed in
// already built.
func TestBroadcastFanoutReportsRejectedSourceOnce(t *testing.T) {
	oversized := func(released *int) wire.Frame {
		w := wire.NewFrameWriter(4)
		w.WriteBytes(make([]byte, wire.MaxFrameLength))
		return wire.OwnedFrame(w.Frame(), w, func(*wire.Writer) { *released++ })
	}
	cases := []struct {
		name  string
		frame func(released *int) wire.Frame
		cause string
	}{
		{
			name:  "short source",
			frame: func(*int) wire.Frame { return wire.BorrowedFrame([]byte{1}) },
			cause: wire.ShortFrameError(1).Error(),
		},
		{
			name:  "invalid source",
			frame: oversized,
			cause: "wire: copy invalid frame: " + oversized(new(int)).Err().Error(),
		},
	}
	for _, tc := range cases {
		for _, eager := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s eager=%t", tc.name, eager), func(t *testing.T) {
				r := newRejectLog()
				released, builds := 0, 0
				var f fanout
				if eager {
					f = fanout{frame: tc.frame(&released), built: true, rejects: r.reporter}
				} else {
					f = fanout{build: func() wire.Frame { builds++; return tc.frame(&released) }, rejects: r.reporter}
				}
				receivers := []*countingReceiver{{}, {}, {}}
				for _, receiver := range receivers {
					f.send(receiver)
				}
				f.release()

				for i, receiver := range receivers {
					if receiver.got != 0 {
						t.Errorf("receiver %d got %d frames, want 0", i, receiver.got)
					}
				}
				if !eager && builds != 1 {
					t.Errorf("source built %d times, want 1", builds)
				}
				if tc.name == "invalid source" && released != 1 {
					t.Errorf("source released %d times, want 1", released)
				}
				lines := r.lines(t)
				if len(lines) != 1 || lines[0].Boundary != "broadcast" || lines[0].Error != tc.cause {
					t.Fatalf("diagnostics = %+v, want one broadcast line with cause %q", lines, tc.cause)
				}
			})
		}
	}
}

// TestBroadcastFanoutCopiesValidSourceToEachRecipient pins the success path
// of the shared fan-out: one build, one copy per recipient, no diagnostic.
func TestBroadcastFanoutCopiesValidSourceToEachRecipient(t *testing.T) {
	r := newRejectLog()
	builds := 0
	f := fanout{build: func() wire.Frame { builds++; return wire.BorrowedFrame([]byte{3, 0, 0x2f}) }, rejects: r.reporter}
	receivers := []*countingReceiver{{}, {}, {}}
	for _, receiver := range receivers {
		f.send(receiver)
	}
	f.release()
	for i, receiver := range receivers {
		if receiver.got != 1 {
			t.Errorf("receiver %d got %d frames, want 1", i, receiver.got)
		}
	}
	if builds != 1 {
		t.Errorf("source built %d times, want 1", builds)
	}
	if lines := r.lines(t); len(lines) != 0 {
		t.Fatalf("diagnostics = %+v, want none", lines)
	}

	// Never reaching a recipient, a lazy fan-out never builds.
	idle := fanout{build: func() wire.Frame { t.Fatal("built with no recipient"); return wire.Frame{} }, rejects: r.reporter}
	idle.release()
}

// TestSendRejectsInvalidFrameObservably pins #1819 at the session and
// connection send boundaries: a too-short frame and a frame carrying its
// own Err are each refused, released, and reported with their cause.
func TestSendRejectsInvalidFrameObservably(t *testing.T) {
	cause := errors.New("frame build failed")
	cases := []struct {
		name     string
		send     func(*Session, wire.Frame) bool
		frame    func(released *int) wire.Frame
		boundary string
		cause    string
	}{
		{
			name:     "session short frame",
			send:     (*Session).SendFrame,
			frame:    func(*int) wire.Frame { return wire.BorrowedFrame([]byte{1}) },
			boundary: "session send",
			cause:    wire.ShortFrameError(1).Error(),
		},
		{
			name: "session invalid frame",
			send: (*Session).SendFrame,
			frame: func(released *int) wire.Frame {
				w := wire.NewFrameWriter(4)
				w.WriteBytes(make([]byte, wire.MaxFrameLength))
				return wire.OwnedFrame(w.Frame(), w, func(*wire.Writer) { *released++ })
			},
			boundary: "session send",
			cause:    fmt.Sprintf("wire: frame length %d exceeds the %d-byte maximum", wire.MaxFrameLength+wire.FrameHeaderSize, wire.MaxFrameLength),
		},
		{
			name:     "session last invalid frame",
			send:     func(s *Session, f wire.Frame) bool { s.sendLast(f); return false },
			frame:    func(*int) wire.Frame { return wire.InvalidFrame(cause) },
			boundary: "session send",
			cause:    cause.Error(),
		},
		{
			name:     "connection invalid frame",
			send:     func(s *Session, f wire.Frame) bool { return s.conn.SendFrame(f) },
			frame:    func(*int) wire.Frame { return wire.InvalidFrame(cause) },
			boundary: "connection send",
			cause:    cause.Error(),
		},
		{
			name:     "connection last invalid frame",
			send:     func(s *Session, f wire.Frame) bool { return s.conn.sendLast(f) },
			frame:    func(*int) wire.Frame { return wire.InvalidFrame(cause) },
			boundary: "connection send",
			cause:    cause.Error(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRejectLog()
			conn := newConn(discardConn{}, zerolog.Nop())
			conn.rejects = r.reporter
			t.Cleanup(func() { conn.Close() })
			session := NewSession(conn, nil)

			released := 0
			if tc.send(session, tc.frame(&released)) {
				t.Fatal("send accepted an invalid frame")
			}
			if strings.Contains(tc.name, "session invalid") && released != 1 {
				t.Errorf("frame released %d times, want 1", released)
			}
			lines := r.lines(t)
			if len(lines) != 1 || lines[0].Boundary != tc.boundary || lines[0].Error != tc.cause {
				t.Fatalf("diagnostics = %+v, want one %q line with cause %q", lines, tc.boundary, tc.cause)
			}

			// The connection stays usable: an invalid frame is refused
			// alone, not treated as a stalled peer.
			if !conn.SendFrame(wire.BorrowedFrame([]byte{3, 0, 0x2f})) {
				t.Fatal("valid frame refused after an invalid one")
			}
		})
	}
}

// BenchmarkBroadcastFrame measures the shared broadcast fan-out: one lazy
// build and one pooled copy per recipient.
func BenchmarkBroadcastFrame(b *testing.B) {
	payload := make([]byte, 64)
	for _, n := range []int{1, 16, 128} {
		b.Run(fmt.Sprintf("recipients=%d", n), func(b *testing.B) {
			receivers := make([]frameReceiver, n)
			for i := range receivers {
				receivers[i] = &countingReceiver{}
			}
			build := func() wire.Frame {
				w := wire.NewFrameWriter(len(payload))
				w.WriteBytes(payload)
				return wire.BorrowedFrame(w.Frame())
			}
			b.ReportAllocs()
			for b.Loop() {
				broadcastFrame(build, func(send func(frameReceiver)) {
					for _, receiver := range receivers {
						send(receiver)
					}
				})
			}
		})
	}
}
