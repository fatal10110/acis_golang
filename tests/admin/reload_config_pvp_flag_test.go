package admin

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func encodeAttackRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(spawnX)
	w.WriteInt32(spawnY)
	w.WriteInt32(spawnZ)
	w.WriteUint8(0)
	return w.Bytes()
}

// pvpFlagScene is a game master force-attacking karma-free players with
// the PvP flags on a clock the test moves.
type pvpFlagScene struct {
	srv      *gameservertest.Server
	flags    *task.PvPFlags
	clock    atomic.Int64 // unix nanoseconds the flags read as now
	victims  int
	attacker interface{ PvPFlagState() task.PvPFlagState }
}

func (s *pvpFlagScene) now() time.Time { return time.Unix(0, s.clock.Load()) }

func (s *pvpFlagScene) at(d time.Duration) { s.clock.Store(time.Unix(1_000_000, 0).Add(d).UnixNano()) }

// attack has the attacker force-attack a fresh karma-free victim until it
// is flagged. A forced attack on an object not selected only selects it,
// so the request goes twice.
func (s *pvpFlagScene) attack(t *testing.T) {
	t.Helper()
	s.victims++
	account := "victim" + strconv.Itoa(s.victims)
	// A level 40 victim survives the hits: killing an innocent player would
	// make the attacker a PKer, which ends its PvP flag again.
	victimID := s.srv.SeedCharacterFor(t, account, "Victim"+strconv.Itoa(s.victims), 40, 0).ID
	enterWorld(t, s.srv.DialClient(t, account, 1))
	s.srv.Settle(t)
	drain(t, s.srv.Client)
	s.srv.Client.Send(encodeAttackRequest(victimID))
	s.srv.Client.Send(encodeAttackRequest(victimID))
	s.srv.AdvanceUntil(t, "attacker PvP-flagged by its attack", func() bool {
		return s.attacker.PvPFlagState() != task.PvPFlagNone
	})
}

// stop has the attacker walk off, ending its attack, so nothing flags it
// again, and lets the swing in progress and the walk end.
func (s *pvpFlagScene) stop(t *testing.T) {
	t.Helper()
	s.srv.Client.Send(encodeMove(spawnX+60, spawnY, spawnZ))
	s.srv.Advance(t, 3*time.Second)
	s.srv.Settle(t)
	drain(t, s.srv.Client)
}

// flaggedAt reports whether the attacker is still flagged once the flag
// task has run at d.
func (s *pvpFlagScene) flaggedAt(t *testing.T, d time.Duration) bool {
	t.Helper()
	s.at(d)
	s.flags.Tick()
	s.srv.Settle(t)
	return s.attacker.PvPFlagState() != task.PvPFlagNone
}

func bootPvPFlagScene(t *testing.T, reloads gameservertest.Option) *pvpFlagScene {
	t.Helper()
	s := &pvpFlagScene{}
	s.at(0)
	s.flags = task.NewPvPFlags(task.DefaultPvPFlagOptions(), s.now)
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithPvPFlags(s.flags), reloads)
	s.srv = srv
	enterWorld(t, srv.Client)
	drain(t, srv.Client)
	obj, ok := srv.State.Player(gmID)
	if !ok {
		t.Fatal("attacker missing from world state")
	}
	s.attacker = obj.(interface{ PvPFlagState() task.PvPFlagState })
	// Every swing hits, so the scenario never waits on a lucky roll.
	done := make(chan struct{})
	if !srv.PlayerQueue(t, gmID).Post(func() {
		defer close(done)
		obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(int) int { return 0 })
	}) {
		t.Fatal("attacker queue closed")
	}
	<-done
	return s
}

// TestAdminReloadConfigPvPFlagTimes: a flag started after //reload config
// lasts the reloaded PvPVsNormalTime, as the reference reads PVP_NORMAL_TIME
// each time it flags a player. At the boot 40 seconds a flag holds at 39
// and is gone at 41; at the reloaded 5 seconds a flag holds at 4 and is
// gone at 6. A flag started while another game master swaps the times back
// and forth ends by one of them.
func TestAdminReloadConfigPvPFlagTimes(t *testing.T) {
	t.Parallel()
	off, on := liveSettings(), liveSettings()
	off.PvPFlagTimes = &task.PvPFlagTimes{Normal: 40 * time.Second, Flagged: 20 * time.Second}
	on.PvPFlagTimes = &task.PvPFlagTimes{Normal: 5 * time.Second, Flagged: 7 * time.Second}
	var calls atomic.Int32
	s := bootPvPFlagScene(t, flipReloads(&calls, off, on))
	gm2, _ := addPlayer(t, s.srv, "gm2", "Reloader", adminLevel)

	s.attack(t)
	s.stop(t)
	if !s.flaggedAt(t, 39*time.Second) || s.flaggedAt(t, 41*time.Second) {
		t.Fatal("boot flag: want it held at 39s and gone at 41s")
	}

	drain(t, gm2)
	assertTexts(t, exchange(t, gm2, encodeBuildCmd("reload config")), configReloaded)
	s.attack(t)
	s.stop(t)
	if !s.flaggedAt(t, 45*time.Second) || s.flaggedAt(t, 47*time.Second) {
		t.Fatal("reloaded flag: want it held 4s after it started and gone after 6s")
	}

	const rounds = 20
	sent := reloadsInBackground(gm2, rounds)
	s.attack(t)
	awaitReloads(t, s.srv, &calls, rounds+1, sent)
	s.stop(t)
	if s.flaggedAt(t, 47*time.Second+41*time.Second) {
		t.Fatal("flag started under swapping times outlived both")
	}
}
