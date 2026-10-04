package network

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

type hpWatchTarget struct {
	world.Presence
	id int32
}

func (t *hpWatchTarget) ObjectID() int32 { return t.id }
func (*hpWatchTarget) Kind() actor.Kind  { return actor.KindNPC }

// statusUpdateHP decodes the CUR_HP of a captured StatusUpdate payload.
func statusUpdateHP(t *testing.T, payload []byte) int32 {
	t.Helper()
	if payload[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("opcode = %#x, want StatusUpdate", payload[0])
	}
	r := wire.NewReader(payload[1:])
	r.ReadInt32() // object id
	if n := r.ReadInt32(); n != 1 {
		t.Fatalf("StatusUpdate carries %d attributes, want CUR_HP only", n)
	}
	if typ := r.ReadInt32(); typ != int32(serverpackets.StatusCurrentHP) {
		t.Fatalf("StatusUpdate attribute = %#x, want CUR_HP", typ)
	}
	hp := r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read StatusUpdate: %v", err)
	}
	return hp
}

// TestSendHPToWatchersQueuesInReadOrder forces the interleaving behind a
// stale health bar: a regen tick reads 53 HP and is still queueing its frame
// when a hit drops HP to 41 on another goroutine. The hit's read must wait
// for the regen frame, so the watcher's last CUR_HP is the server's 41.
func TestSendHPToWatchersQueuesInReadOrder(t *testing.T) {
	t.Parallel()
	const id = 7
	var capture testsupport.FrameCapture
	regenSending, releaseRegen := make(chan struct{}), make(chan struct{})
	var once sync.Once
	watcher := &livePlayer{Character: &player.Character{ID: 1}, session: func(f wire.Frame) bool {
		blocked := false
		once.Do(func() { blocked = true })
		if blocked {
			close(regenSending)
			<-releaseRegen
		}
		return capture.Send(f)
	}}
	watcher.StoreTarget(&hpWatchTarget{id: id})
	known := []world.Tracked{watcher}

	var (
		bar   creature.HPBar // uncalibrated: every change is sent
		hpMu  sync.Mutex
		hp    = 61.0
		sends sync.WaitGroup
	)
	setHP := func(v float64) {
		hpMu.Lock()
		hp = v
		hpMu.Unlock()
	}
	publish := func(onRead func()) func(func(int)) {
		return func(send func(int)) {
			bar.Publish(func() float64 {
				onRead()
				hpMu.Lock()
				defer hpMu.Unlock()
				return hp
			}, 61, send)
		}
	}

	setHP(53) // the regen tick
	sends.Go(func() { sendHPToWatchers(known, id, publish(func() {})) })
	<-regenSending

	setHP(41) // the hit, from another queue
	hitRead := make(chan struct{})
	sends.Go(func() { sendHPToWatchers(known, id, publish(func() { close(hitRead) })) })
	select {
	case <-hitRead:
		t.Fatal("the hit read HP while the regen frame was still being queued")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseRegen)
	sends.Wait()

	frames := capture.Frames()
	if len(frames) != 2 {
		t.Fatalf("watcher got %d frames, want 2", len(frames))
	}
	if first, last := statusUpdateHP(t, frames[0]), statusUpdateHP(t, frames[1]); first != 53 || last != 41 {
		t.Fatalf("watcher read CUR_HP %d then %d, want 53 then the server's 41", first, last)
	}
}
