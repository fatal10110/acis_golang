package admin

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const configReloaded = "Configs files have been reloaded."

// configFiles stands in for the properties files //reload config reads:
// AllowDiscardItem, and whether the files can be read at all.
type configFiles struct {
	discardDisabled atomic.Bool
	broken          atomic.Bool
	reads           atomic.Int32
}

func (f *configFiles) reloads() network.DataReloads {
	return network.DataReloads{Config: func() (network.PlayerConfig, error) {
		f.reads.Add(1)
		if f.broken.Load() {
			return network.PlayerConfig{}, errors.New("broken server.properties")
		}
		return network.PlayerConfig{DiscardItemDisabled: f.discardDisabled.Load(), RespawnRestoreHP: 0.7, MaxBuffsAmount: 20}, nil
	}}
}

func encodeDropItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(spawnX)
	w.WriteInt32(spawnY)
	w.WriteInt32(spawnZ)
	return w.Bytes()
}

// addDropper seeds a plain player holding adena and enters the world with
// it, returning its client and the adena's object id.
func addDropper(t *testing.T, srv *gameservertest.Server) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	ch := srv.SeedCharacterFor(t, "dropper", "Dropper", 1, 0)
	setAccessLevel(t, srv, ch.ID, userLevel)
	adena := srv.GiveItem(t, ch.ID, item.AdenaID, 1000)
	c := srv.DialClient(t, "dropper", 1)
	enterWorld(t, c)
	return c, adena
}

// dropOutcome drops one adena and reports whether it reached the ground;
// a drop that does not must be refused with CANNOT_DISCARD_THIS_ITEM.
func dropOutcome(t *testing.T, c *testsupport.ScriptedClient, adena int32) bool {
	t.Helper()
	frames := exchange(t, c, encodeDropItem(adena, 1))
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeDropItem {
			return true
		}
	}
	if len(frames) != 1 {
		t.Fatalf("refused drop frames = %x, want CANNOT_DISCARD_THIS_ITEM alone", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageCannotDiscardThisItem)
	return false
}

// hasText reports whether frames hold the plain-text message text.
func hasText(t *testing.T, frames [][]byte, text string) bool {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(frame[1:])
		if int(r.ReadInt32()) != serverpackets.SystemMessageS1 || r.ReadInt32() != 1 || r.ReadInt32() != serverpackets.SystemMessageParamText {
			continue
		}
		if r.ReadString() == text {
			return true
		}
	}
	return false
}

// TestAdminReloadConfig pins //reload config (Config.loadGameServer): the
// properties files are read anew and a setting read after the reload is the
// reloaded one, so a player's next drop follows the new AllowDiscardItem.
// Files that cannot be read answer the usage and leave the settings in
// force as they were.
func TestAdminReloadConfig(t *testing.T) {
	t.Parallel()
	files := &configFiles{}
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithDataReloads(files.reloads()))
	gm := srv.Client
	enterWorld(t, gm)
	user, adena := addDropper(t, srv)
	drain(t, gm)

	if !dropOutcome(t, user, adena) {
		t.Fatal("a drop before the reload was refused")
	}
	drain(t, gm)

	files.discardDisabled.Store(true)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	if files.reads.Load() != 1 {
		t.Fatalf("//reload config read the files %d times, want once", files.reads.Load())
	}
	if dropOutcome(t, user, adena) {
		t.Fatal("a drop after AllowDiscardItem = False was reloaded reached the ground")
	}

	// A reload that cannot read the files keeps the settings in force.
	files.broken.Store(true)
	files.discardDisabled.Store(false)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload configs htm")), reloadUsage...)
	if dropOutcome(t, user, adena) {
		t.Fatal("a failed reload replaced the settings in force")
	}

	files.broken.Store(false)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	if !dropOutcome(t, user, adena) {
		t.Fatal("a drop after AllowDiscardItem = True was reloaded was refused")
	}
}

// TestAdminReloadConfigRaceFree reloads the settings back and forth while a
// player drops, each drop reading AllowDiscardItem: under -race, every drop
// sees one config or the other and nothing races.
func TestAdminReloadConfigRaceFree(t *testing.T) {
	t.Parallel()
	files := &configFiles{}
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithDataReloads(files.reloads()))
	gm := srv.Client
	enterWorld(t, gm)
	user, adena := addDropper(t, srv)
	drain(t, gm)

	const rounds = 20
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range rounds {
			files.discardDisabled.Store(i%2 == 0)
			if frames := exchange(t, gm, encodeBuildCmd("reload config")); !hasText(t, frames, configReloaded) {
				t.Errorf("reload %d frames = %x, want %q", i, testsupport.FrameOpcodes(frames), configReloaded)
				return
			}
		}
	}()
	for range rounds {
		dropOutcome(t, user, adena)
	}
	wg.Wait()
	if got := files.reads.Load(); got != rounds {
		t.Fatalf("config read %d times, want %d", got, rounds)
	}
}
