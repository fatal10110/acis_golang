package network

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/rs/zerolog"
)

// lockedBuffer is a log sink safe to write from the writer goroutine while
// the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// lines decodes every JSON log line written so far.
func (b *lockedBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		out = append(out, entry)
	}
	return out
}

// pooledFrame is a size-byte frame with opcode op built in a writer of
// writerCap capacity, like a pooled writer reused for a smaller packet.
func pooledFrame(op byte, size, writerCap int) wire.Frame {
	w := wire.NewFrameWriter(writerCap)
	w.WriteUint8(op)
	w.WriteBytes(make([]byte, size-wire.FrameHeaderSize-1))
	return wire.OwnedFrame(w.Frame(), w, func(*wire.Writer) {})
}

func awaitStopped(t *testing.T, c *Conn) {
	t.Helper()
	select {
	case <-c.stopped:
	case <-time.After(time.Second):
		t.Fatal("connection writer did not stop")
	}
}

// Frames built in pooled writers larger than their bytes are charged their
// wire bytes against the high-water mark, so writer reuse cannot shrink the
// backlog a healthy burst may reach. Each 1 KiB frame here sits in a 2 KiB
// writer; charged by capacity the limit would trip at half the frames.
func TestConnChargesWireBytesAgainstHighWater(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	c := newConn(server, zerolog.Nop())

	const frameSize, writerCap = 1024, 2048
	want := outboundHighWater / (frameSize + int(unsafe.Sizeof(wire.Frame{})))
	accepted := 0
	for sendWithin(t, c, pooledFrame(0x03, frameSize, writerCap)) {
		accepted++
		if accepted > want+1 {
			break
		}
	}
	if accepted != want {
		t.Fatalf("accepted %d %d-byte frames in %d-byte writers, want %d under the %d-byte high-water mark",
			accepted, frameSize, writerCap, want, outboundHighWater)
	}
	awaitStopped(t, c)
}

// The backlog abort names the account and address, the backlog's frame
// count, wire bytes and pinned memory, and the opcodes filling it, and the
// writer whose write the abort cut off does not report a second failure.
func TestConnBacklogAbortLogsBacklog(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	var logs lockedBuffer
	c := newConn(server, zerolog.New(&logs))
	session := NewSession(c, nil) // cleartext: the cipher is not enabled yet
	NewClient(session).SetAuthenticated("bot0042", link.SessionKey{})

	const frameSize, writerCap = 300, 512
	const common, rare = 0x03, 0x16
	send := func(frame wire.Frame) bool {
		done := make(chan bool, 1)
		go func() { done <- session.SendFrame(frame) }()
		select {
		case ok := <-done:
			return ok
		case <-time.After(time.Second):
			t.Fatal("SendFrame blocked on a peer that is not reading")
			return false
		}
	}
	accepted := 0
	for {
		op := byte(common)
		if accepted%4 == 0 {
			op = rare
		}
		if !send(pooledFrame(op, frameSize, writerCap)) {
			break
		}
		accepted++
		if accepted > outboundHighWater {
			t.Fatal("backlog never aborted")
		}
	}
	awaitStopped(t, c)

	var abort map[string]any
	for _, entry := range logs.lines(t) {
		switch entry["message"] {
		case "outbound backlog over high-water mark, aborting connection":
			abort = entry
		case "game connection write failed, closing":
			t.Fatalf("aborted connection also logged a write failure: %v", entry)
		}
	}
	if abort == nil {
		t.Fatalf("no backlog abort logged; logs: %v", logs.lines(t))
	}
	slot := int(unsafe.Sizeof(wire.Frame{}))
	wantInt := map[string]int{
		"frames":                accepted,
		"wire_bytes":            accepted * frameSize,
		"charged_bytes":         accepted * (frameSize + slot),
		"pinned_bytes":          accepted * (writerCap + slot),
		"rejected_wire_bytes":   frameSize,
		"rejected_pinned_bytes": writerCap + slot,
		"high_water_bytes":      outboundHighWater,
		"pinned_limit_bytes":    outboundPinnedLimit,
	}
	for field, want := range wantInt {
		if got, ok := abort[field].(float64); !ok || int(got) != want {
			t.Errorf("%s = %v, want %d", field, abort[field], want)
		}
	}
	if abort["account"] != "bot0042" {
		t.Errorf("account = %v, want bot0042", abort["account"])
	}
	if abort["addr"] != server.RemoteAddr().String() {
		t.Errorf("addr = %v, want %s", abort["addr"], server.RemoteAddr())
	}
	if queued, ok := abort["frames_queued"].(float64); !ok || queued < 1 || int(queued) > accepted {
		t.Errorf("frames_queued = %v, want 1..%d", abort["frames_queued"], accepted)
	}
	top, _ := abort["top_opcodes"].(string)
	if !strings.HasPrefix(top, "0x03=") || !strings.Contains(top, ",0x16=") {
		t.Errorf("top_opcodes = %q, want 0x03 first, then 0x16", top)
	}
	if abort["rejected_opcode"] != "0x16" && abort["rejected_opcode"] != "0x03" {
		t.Errorf("rejected_opcode = %v, want one of the sent opcodes", abort["rejected_opcode"])
	}
}

// A write that fails because the peer went away is still reported.
func TestConnWriteFailureWithoutAbortIsLogged(t *testing.T) {
	server, client := net.Pipe()
	var logs lockedBuffer
	c := newConn(server, zerolog.New(&logs))
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	sendWithin(t, c, pooledFrame(0x03, 16, 64))
	awaitStopped(t, c)
	for _, entry := range logs.lines(t) {
		if entry["message"] == "game connection write failed, closing" {
			return
		}
	}
	t.Fatalf("write failure not logged; logs: %v", logs.lines(t))
}

func TestTopOpcodesOrdersByCount(t *testing.T) {
	ops := []byte{0x16, 0x03, 0x03, 0xfe, 0x03, 0x16, 0x31, 0x32, 0x33, 0x34}
	if got, want := topOpcodes(ops, 3), "0x03=3,0x16=2,0x31=1"; got != want {
		t.Fatalf("topOpcodes = %q, want %q", got, want)
	}
	if got := topOpcodes(nil, 3); got != "" {
		t.Fatalf("topOpcodes(nil) = %q, want empty", got)
	}
}
