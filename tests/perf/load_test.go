// Package perf holds the opt-in load baseline for the concurrency refactor:
// many scripted clients in one region driven through the production boot
// path, real packets and the shared MariaDB. It skips unless
// ACIS_PERF_CLIENTS is set, so the default test run never pays for it.
//
//	ACIS_PERF_CLIENTS=50 ACIS_PERF_DURATION=30s go test ./tests/perf/ -run TestLoadBaseline -v -count=1
package perf

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// stepInterval paces each client well under the flood gate (two packets per
// step: the action and its probe).
const stepInterval = 250 * time.Millisecond

// probeTimeout bounds one step; a step that exceeds it counts as a timeout.
const probeTimeout = 5 * time.Second

// TestLoadBaseline boots one server, enters ACIS_PERF_CLIENTS characters at
// the same spot, gives each its own monster next to it, and for
// ACIS_PERF_DURATION (default 30s) has every client cycle target → attack →
// walk → walk, one step every stepInterval. After each step the client sends
// RequestManorList, whose ExSendManorList reply is self-only and stateless:
// a connection handles requests in order, so the time from sending the step
// to reading that reply is the step's server handling latency plus two
// loopback hops. Reported: process CPU time (server and clients share the
// process), step latency p50/p99/max, GC count and pauses, frames received.
func TestLoadBaseline(t *testing.T) {
	clients := envInt(t, "ACIS_PERF_CLIENTS", 0)
	if clients <= 0 {
		t.Skip("set ACIS_PERF_CLIENTS to run the load baseline")
	}
	duration := envDuration(t, "ACIS_PERF_DURATION", 30*time.Second)

	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Perf0", 5, 0),
		gameservertest.WithWantChars(1),
	)
	conns := []*testsupport.ScriptedClient{srv.Client}
	for i := 1; i < clients; i++ {
		account := fmt.Sprintf("perf%d", i)
		srv.SeedCharacterFor(t, account, fmt.Sprintf("Perf%d", i), 5, 0)
		conns = append(conns, srv.DialClient(t, account, 1))
	}

	readers := make([]*frameReader, clients)
	for i, c := range conns {
		enterWorld(t, c)
		readers[i] = startReader(c)
	}
	defer func() {
		for _, r := range readers {
			r.stop()
		}
	}()

	x, y, z := srv.PlayerPosition(t, srv.SoleObjectID(t))
	origin := location.Location{X: x, Y: y, Z: z}
	monsters := make([]*npc.Hostile, clients)
	for i := range monsters {
		// ponytail: one 1000-HP monster per client, never respawned; a dead
		// one still costs a rejected attack step. Respawn if runs outlive it.
		monsters[i] = srv.SpawnHostileNPCAt(t, location.Location{X: x + 40 + i%10*20, Y: y + i/10*20, Z: z})
	}

	var before syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &before); err != nil {
		t.Fatal(err)
	}
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	for _, r := range readers {
		r.frames.Store(0)
	}

	start := time.Now()
	deadline := start.Add(duration)
	samples := make([][]time.Duration, clients)
	var timeouts atomic.Int64
	var wg sync.WaitGroup
	for i := range conns {
		wg.Go(func() {
			samples[i] = runClient(conns[i], readers[i], monsters[i], origin, i, deadline, &timeouts)
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	var after syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &after); err != nil {
		t.Fatal(err)
	}
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	var all []time.Duration
	var frames int64
	disconnected := 0
	for i := range samples {
		all = append(all, samples[i]...)
		frames += readers[i].frames.Load()
		if readers[i].lost.Load() {
			disconnected++
		}
	}
	if len(all) == 0 {
		t.Fatal("no step completed")
	}
	slices.Sort(all)
	cpu := cpuTime(after) - cpuTime(before)
	gcs := memAfter.NumGC - memBefore.NumGC
	t.Logf("load baseline: clients=%d duration=%s GOMAXPROCS=%d", clients, elapsed.Round(time.Millisecond), runtime.GOMAXPROCS(0))
	t.Logf("  disconnected=%d steps=%d timeouts=%d frames_received=%d (%.0f/s)", disconnected, len(all), timeouts.Load(), frames, float64(frames)/elapsed.Seconds())
	t.Logf("  step latency p50=%s p99=%s max=%s", percentile(all, 50), percentile(all, 99), all[len(all)-1])
	t.Logf("  process CPU=%s (%.2f cores)", cpu.Round(time.Millisecond), cpu.Seconds()/elapsed.Seconds())
	t.Logf("  GC count=%d pause_total=%s pause_max=%s", gcs, time.Duration(memAfter.PauseTotalNs-memBefore.PauseTotalNs), maxPause(&memAfter, gcs))
}

// runClient drives one client until deadline, or until its connection is
// lost, and returns its step latencies. A lost client just stops; the
// disconnected count reports it.
func runClient(c *testsupport.ScriptedClient, r *frameReader, monster *npc.Hostile, origin location.Location, index int, deadline time.Time, timeouts *atomic.Int64) []time.Duration {
	rng := rand.New(rand.NewPCG(uint64(index), 2285))
	tick := time.NewTicker(stepInterval)
	defer tick.Stop()
	var latencies []time.Duration
	for step := index; time.Now().Before(deadline) && !r.lost.Load(); step++ {
		<-tick.C
		select { // a reply that outlived its step's timeout
		case <-r.probe:
		default:
		}
		sent := time.Now()
		var action []byte
		switch step % 4 {
		case 0, 1: // first click targets, the second attacks
			action = encodeAction(monster.ObjectID(), origin)
		default:
			target := location.Location{X: origin.X + rng.IntN(601) - 300, Y: origin.Y + rng.IntN(601) - 300, Z: origin.Z}
			action = encodeMoveBackwardToLocation(target, origin)
		}
		if c.TrySend(action) != nil || c.TrySend(encodeRequestManorList()) != nil {
			r.lost.Store(true)
			break
		}
		select {
		case <-r.probe:
			latencies = append(latencies, time.Since(sent))
		case <-time.After(probeTimeout):
			timeouts.Add(1)
		}
	}
	return latencies
}

// frameReader drains one client's stream on its own goroutine so broadcast
// traffic never backs up, counting frames and signaling each probe reply.
// lost is set once the server closed the connection. The read never uses a
// short deadline: one expiring mid-frame would drop the partial frame.
type frameReader struct {
	client  *testsupport.ScriptedClient
	probe   chan struct{}
	frames  atomic.Int64
	lost    atomic.Bool
	done    atomic.Bool
	stopped chan struct{}
}

func startReader(c *testsupport.ScriptedClient) *frameReader {
	r := &frameReader{client: c, probe: make(chan struct{}, 16), stopped: make(chan struct{})}
	go func() {
		defer close(r.stopped)
		for {
			frame, err := c.TryRead(time.Hour)
			if err != nil {
				r.lost.Store(!r.done.Load())
				return
			}
			if frame == nil {
				continue
			}
			r.frames.Add(1)
			if isManorListReply(frame) {
				r.probe <- struct{}{}
			}
		}
	}()
	return r
}

// stop closes the connection to end the blocked read.
func (r *frameReader) stop() {
	r.done.Store(true)
	_ = r.client.Close()
	<-r.stopped
}

func isManorListReply(frame []byte) bool {
	return len(frame) >= 3 && frame[0] == serverpackets.OpcodeExtended &&
		binary.LittleEndian.Uint16(frame[1:3]) == serverpackets.OpcodeExSendManorList
}

// enterWorld selects the account's only character and drains the EnterWorld
// burst plus whatever the arrival triggers.
func enterWorld(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	for c.ReadWithTimeout(300*time.Millisecond) != nil {
	}
}

func encodeAction(objectID int32, origin location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteUint8(0)
	return w.Bytes()
}

func encodeMoveBackwardToLocation(target, origin location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(int32(target.X))
	w.WriteInt32(int32(target.Y))
	w.WriteInt32(int32(target.Z))
	w.WriteInt32(int32(origin.X))
	w.WriteInt32(int32(origin.Y))
	w.WriteInt32(int32(origin.Z))
	w.WriteInt32(1)
	return w.Bytes()
}

func encodeRequestManorList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestManorList)
	return w.Bytes()
}

func percentile(sorted []time.Duration, p int) time.Duration {
	return sorted[(len(sorted)-1)*p/100]
}

func cpuTime(u syscall.Rusage) time.Duration {
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}

// maxPause returns the longest of the last n GC pauses (at most the 256
// runtime.MemStats keeps).
func maxPause(m *runtime.MemStats, n uint32) time.Duration {
	var longest uint64
	for i := uint32(0); i < min(n, uint32(len(m.PauseNs))); i++ {
		longest = max(longest, m.PauseNs[(m.NumGC-1-i)%uint32(len(m.PauseNs))])
	}
	return time.Duration(longest)
}

func envInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("%s=%q: %v", name, v, err)
	}
	return n
}

func envDuration(t *testing.T, name string, fallback time.Duration) time.Duration {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		t.Fatalf("%s=%q: %v", name, v, err)
	}
	return d
}
