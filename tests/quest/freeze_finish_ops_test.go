package quest

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The script reads and one-player sends the finishing lane of the
// script-facing API freeze adds (#3748), driven through a booted world as
// a hook calls them.

// utf16z appends text as a NUL-terminated UTF-16LE string.
func utf16z(out []byte, text string) []byte {
	for _, r := range utf16.Encode([]rune(text)) {
		out = binary.LittleEndian.AppendUint16(out, r)
	}
	return append(out, 0, 0)
}

// int32s appends each value little-endian.
func int32s(out []byte, values ...int32) []byte {
	for _, v := range values {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// TestScriptWorldReads pins GameTimeTaskManager.getCurrentTick (the
// uptime seconds Quest.getElapsedTicks counts from),
// SevenSignsManager.getCurrentPeriod and getSealOwner (each contested
// seal's owner, none for a seal that is not contested), and
// HtmCache.getHtm (a datapack page named from the server root, none for a
// missing one).
func TestScriptWorldReads(t *testing.T) {
	t.Parallel()
	const page = "<html><body>Dungeon gate %objectId%</body></html>"
	srv, objID := bootHelpers(t, gameservertest.WithHTMLPages(map[string]string{"doormen/35506.htm": page}))
	owners := srv.SevenSigns.SealOwners()
	period := srv.SevenSigns.CurrentPeriod()
	run(t, srv, objID, func(sc *script.Script, _ *script.Player) {
		if tick, elapsed := sc.CurrentTick(), sc.ElapsedTicks(0); tick < 0 || elapsed-tick > 1 || elapsed < tick {
			t.Errorf("CurrentTick() = %d, ElapsedTicks(0) = %d; want the same uptime count", tick, elapsed)
		}
		if got := sc.SevenSignsPeriod(); got != period {
			t.Errorf("SevenSignsPeriod() = %v, want %v", got, period)
		}
		for i, seal := range sevensigns.Seals {
			if got := sc.SealOwner(seal); got != owners[i] {
				t.Errorf("SealOwner(%v) = %v, want %v", seal, got, owners[i])
			}
		}
		if got := sc.SealOwner(sevensigns.NoSeal); got != sevensigns.NoCabal {
			t.Errorf("SealOwner(NONE) = %v, want no cabal", got)
		}
		// The fixture stores each page with a closing line break.
		if got, ok := sc.Page("data/html/doormen/35506.htm"); !ok || strings.TrimSuffix(got, "\n") != page {
			t.Errorf("Page(35506) = %q, %v; want the stored page", got, ok)
		}
		if got, ok := sc.Page("data/html/doormen/35506-2.htm"); ok {
			t.Errorf("Page of a missing file = %q, want none", got)
		}
	})
}

// TestScriptDoorOpened pins Door.isOpened: the door's open state as the
// world holds it, before and after a script opens it.
func TestScriptDoorOpened(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, gameservertest.WithDoors(scriptGate()))
	run(t, srv, objID, func(sc *script.Script, _ *script.Player) {
		gate := sc.Door("script_gate")
		if gate.Opened() {
			t.Fatal("the gate starts open")
		}
		gate.Open()
		if !gate.Opened() {
			t.Error("Opened() after Open = false")
		}
		gate.Close()
		if gate.Opened() {
			t.Error("Opened() after Close = true")
		}
	})
}

// TestScriptOnePlayerScreenSends pins the sends a script makes to one
// player: ExShowScreenMessage(NpcStringId, time) (Q114: custom text, no
// system message, top center, the time), Earthquake(player, intensity,
// duration) (Q635: where the player stands, not an NPC's), and a page the
// script filled itself (FreyaDeacon's NpcHtmlMessage.replace) shown through
// the NPC, then ActionFailed. Each reaches that player only, not its party
// mate.
func TestScriptOnePlayerScreenSends(t *testing.T) {
	t.Parallel()
	srv, leader, _, mateClient := partyPair(t, "1", "1")
	x, y, z := srv.PlayerPosition(t, leader)
	const npcObjectID = 0x10000077
	run(t, srv, leader, func(_ *script.Script, p *script.Player) {
		p.ShowScreenMessage(4500, "Is it a monster?")
		p.Earthquake(20, 10)
		p.ShowHTML(npcObjectID, "<html><body>Mate cannot enter %objectId%</body></html>")
	})

	screen := []byte{serverpackets.OpcodeExtended, 0x38, 0x00}
	screen = int32s(screen, 1, -1, 2, 0, 0, 0, 0, 0, 4500, 0)
	screen = utf16z(screen, "Is it a monster?")
	quake := int32s([]byte{serverpackets.OpcodeEarthquake}, int32(x), int32(y), int32(z), 20, 10, 0)
	html := int32s([]byte{serverpackets.OpcodeNpcHtmlMessage}, npcObjectID)
	html = int32s(utf16z(html, "<html><body>Mate cannot enter 268435575</body></html>"), 0)

	frames := srv.ReadQueued(t, srv.Client)
	var got [][]byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeExtended, serverpackets.OpcodeEarthquake, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed:
			got = append(got, f)
		}
	}
	want := [][]byte{screen, quake, html, {serverpackets.OpcodeActionFailed}}
	if len(got) != len(want) {
		t.Fatalf("player frames % x, want % x", got, want)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("frame %d = % x, want % x", i, got[i], want[i])
		}
	}
	for _, f := range srv.ReadQueued(t, mateClient) {
		if slices.Contains([]byte{serverpackets.OpcodeExtended, serverpackets.OpcodeEarthquake, serverpackets.OpcodeNpcHtmlMessage}, f[0]) {
			t.Errorf("mate got % x", f)
		}
	}
}

// Zones of the boss zone world: a grand boss zone over the character,
// which stands at (10, 20, 30), and a script zone beside it.
const (
	sirraZoneID = 110013
	gateZoneID  = 100001
)

func bossZoneWorld(t *testing.T) (*zone.Boss, gameservertest.Option) {
	t.Helper()
	bossForm, err := zone.NewCuboid(-100, 100, -100, 100, -1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	gateForm, err := zone.NewCuboid(300, 500, -100, 100, -1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	boss := zone.NewBoss(sirraZoneID, bossForm)
	zones := zone.NewIndex()
	zones.Add(zone.NewScript(gateZoneID, gateForm))
	zones.Add(boss)
	return boss, gameservertest.WithZones(zones)
}

// TestScriptZoneReadsAndBossEntry pins ZoneManager.getZoneById (any zone
// by id, none for an unknown id), ZoneManager.getZone(x, y, z,
// BossZone.class) (the grand boss zone holding a point, none outside every
// one), ZoneType.isInsideZone (the point in the zone's volume) and
// BossZone.allowPlayerEntry (the player added to the zone's admitted
// players); a zone that is no grand boss zone admits no one.
func TestScriptZoneReadsAndBossEntry(t *testing.T) {
	t.Parallel()
	boss, zones := bossZoneWorld(t)
	srv, objID := bootHelpers(t, zones)
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		sirra, gate := sc.Zone(sirraZoneID), sc.Zone(gateZoneID)
		if sirra == nil || gate == nil {
			t.Fatalf("Zone(%d) = %v, Zone(%d) = %v; want both", sirraZoneID, sirra, gateZoneID, gate)
		}
		if sc.Zone(110014) != nil {
			t.Error("Zone of an unknown id found one")
		}
		if !sirra.Contains(p) || gate.Contains(p) {
			t.Errorf("player inside boss zone %v, gate %v; want true, false", sirra.Contains(p), gate.Contains(p))
		}
		if !gate.Contains(script.Loc{X: 400, Y: 0, Z: 0}) || gate.Contains(script.Loc{X: 400, Y: 0, Z: 2000}) {
			t.Error("the gate's volume is not its cuboid")
		}
		at := sc.BossZoneAt(p)
		if at == nil || !at.Contains(script.Loc{X: 0, Y: 0, Z: 0}) {
			t.Fatalf("BossZoneAt(player) = %v, want the Sirra zone", at)
		}
		if sc.BossZoneAt(script.Loc{X: 400, Y: 0, Z: 0}) != nil {
			t.Error("BossZoneAt found a boss zone inside the script zone only")
		}
		at.AllowPlayerEntry(p, 2100)
	})
	if got := boss.AllowedPlayers(); !slices.Equal(got, []int32{objID}) {
		t.Fatalf("admitted players %v, want [%d]", got, objID)
	}
	if srv.RunScript(t, objID, q001, func(sc *script.Script, p *script.Player) {
		sc.Zone(gateZoneID).AllowPlayerEntry(p, 30)
	}) {
		t.Fatal("entry to a script zone did not abort the invocation")
	}
}
