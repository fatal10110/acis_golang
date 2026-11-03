package npcs

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A necropolis entrance and exit gatekeeper (Gatekeeper Ziggurat), each
// with the one instant destination the dungeon commands take.
const (
	dungeonEntranceID = 31095
	dungeonExitID     = 31103
)

var (
	dungeonInside  = location.Location{X: 40000, Y: -50000, Z: -1000}
	dungeonOutside = location.Location{X: -30000, Y: 60000, Z: -2000}
)

// The refusal pages, told apart: the datapack ships the same text in both.
const (
	necroRefusalPage = "<html><body>Gatekeeper Ziggurat:<br>You may not pass into the necropolis.</body></html>"
	cataRefusalPage  = "<html><body>Gatekeeper Ziggurat:<br>You may not pass into the catacomb.</body></html>"
)

// Seven Signs system messages a refused talker reads.
const (
	msgCanBeUsedByDawn = 1301
	msgCanBeUsedByDusk = 1302
)

// bootDungeonWorld enters the world next to nothing, with the dungeon
// gatekeepers' destinations and refusal pages in place.
func bootDungeonWorld(t *testing.T) *folkWorld {
	t.Helper()
	pages := dialogPages()
	pages["seven_signs/necro_no.htm"] = necroRefusalPage
	pages["seven_signs/cata_no.htm"] = cataRefusalPage
	return bootFolkWorld(t, pages,
		gameservertest.WithTeleports(nil, travel.InstantTable{
			dungeonEntranceID: {dungeonInside},
			dungeonExitID:     {dungeonOutside},
		}, false, time.Now),
		noBypassReuse,
	)
}

// askDungeon spawns a dungeon gatekeeper of npcID next to the player and
// sends it command from a page admitting it.
func (w *folkWorld) askDungeon(t *testing.T, npcID int, command string) (*npc.Folk, [][]byte) {
	t.Helper()
	gk := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", npcID), 50)
	w.openAnyNpcPage(t)
	return gk, w.tripFrames(t, npcCommand(gk, command))
}

// appearAt lets the client finish the teleport in frames, then moves the
// fixture's idea of where the player stands to the landing. It returns the
// compass codes the rejoin sends.
func (w *folkWorld) appearAt(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	r := wire.NewReader(landing(t, frames)[5:])
	w.at = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	w.c.Send(encodeAppearing())
	var codes []byte
	for _, f := range drainFrames(t, w.c) {
		if len(f) == 7 && f[0] == serverpackets.OpcodeExtended && f[1] == 0x32 && f[2] == 0 {
			codes = append(codes, f[3])
		}
	}
	return codes
}

// dungeonTrip is a dungeon command that moves the player: the
// gatekeeper's leading release, the jump, then the dispatcher's release.
func dungeonTrip() [][]byte {
	return append(append([][]byte{released}, jumped...), released)
}

// TestDungeonGatekeeperEntryRules pins the necro and cata commands: during
// seal validation a winning cabal lets down only its own members, and only
// while it owns the dungeon's seal (Avarice for a necropolis, Gnosis for a
// catacomb), refusing anyone else with the message naming it and the
// refusal page; after a tie any cabal member goes down. In the other
// periods any cabal member goes down. A player in no cabal is refused with
// the page alone. Whoever goes down jumps to the gatekeeper's instant
// destination and is in the dungeon; exit brings them out of it.
func TestDungeonGatekeeperEntryRules(t *testing.T) {
	t.Parallel()
	w := bootDungeonWorld(t)
	const none, dawn, dusk = sevensigns.NoCabal, sevensigns.Dawn, sevensigns.Dusk
	for _, tc := range []struct {
		name                    string
		period                  sevensigns.Period
		winner, avarice, gnosis sevensigns.Cabal
		player                  sevensigns.Cabal
		command                 string
		admitted                bool
		message                 int32
	}{
		{"no cabal in competition", sevensigns.Competition, dawn, dawn, dawn, none, "necro", false, 0},
		{"member in competition", sevensigns.Competition, dawn, none, none, dusk, "cata", true, 0},
		{"loser in results", sevensigns.Results, dawn, none, none, dusk, "necro", true, 0},
		{"no cabal in recruiting", sevensigns.Recruiting, none, none, none, none, "cata", false, 0},
		{"dawn holding avarice", sevensigns.SealValidation, dawn, dawn, none, dawn, "necro", true, 0},
		{"dawn without avarice", sevensigns.SealValidation, dawn, dusk, dawn, dawn, "necro", false, msgCanBeUsedByDawn},
		{"dusk under dawn", sevensigns.SealValidation, dawn, dawn, dawn, dusk, "necro", false, msgCanBeUsedByDawn},
		{"dawn without gnosis", sevensigns.SealValidation, dawn, dawn, none, dawn, "cata", false, msgCanBeUsedByDawn},
		{"dusk holding gnosis", sevensigns.SealValidation, dusk, none, dusk, dusk, "cata", true, 0},
		{"no cabal under dusk", sevensigns.SealValidation, dusk, dusk, dusk, none, "catacomb", false, msgCanBeUsedByDusk},
		{"member after a tie", sevensigns.SealValidation, none, none, none, dawn, "necropolis", true, 0},
		{"no cabal after a tie", sevensigns.SealValidation, none, none, none, none, "necro", false, 0},
	} {
		setSevenSigns(t, w, tc.period, tc.winner, tc.gnosis, tc.avarice, tc.player)
		gk, frames := w.askDungeon(t, dungeonEntranceID, tc.command)
		if !tc.admitted {
			page := necroRefusalPage
			if tc.command[0] == 'c' {
				page = cataRefusalPage
			}
			want := [][]byte{released, htmlFrame, released}
			if tc.message != 0 {
				want = [][]byte{released, sysMsg(tc.message), htmlFrame, released}
			}
			assertDialog(t, tc.name, frames, gk, wantChatPage(page, gk), want...)
			if w.character(t).In7sDungeon() {
				t.Fatalf("%s: in the dungeon after a refusal", tc.name)
			}
			continue
		}
		assertFrames(t, tc.name, frames, dungeonTrip()...)
		assertLandedNear(t, landing(t, frames), dungeonInside)
		if !w.character(t).In7sDungeon() {
			t.Fatalf("%s: not in the dungeon after going down", tc.name)
		}
		w.appearAt(t, frames)
		_, frames = w.askDungeon(t, dungeonExitID, "exit")
		assertFrames(t, tc.name+" exit", frames, dungeonTrip()...)
		assertLandedNear(t, landing(t, frames), dungeonOutside)
		if w.character(t).In7sDungeon() {
			t.Fatalf("%s: still in the dungeon after exit", tc.name)
		}
		w.appearAt(t, frames)
	}
}

// TestDungeonGatekeeperMembershipShownAndSaved pins what going down
// changes beyond the jump: the compass shows the Seven Signs code once the
// player appears in the dungeon, and the general code again once exit
// brought it out; the membership is saved with the character.
func TestDungeonGatekeeperMembershipShownAndSaved(t *testing.T) {
	t.Parallel()
	w := bootDungeonWorld(t)
	setSevenSigns(t, w, sevensigns.Competition, sevensigns.Dawn, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.Dusk)

	_, frames := w.askDungeon(t, dungeonEntranceID, "necro")
	if codes := w.appearAt(t, frames); string(codes) != string([]byte{byte(serverpackets.CompassSevenSignsZone)}) {
		t.Fatalf("compass after going down = %x, want 0d", codes)
	}
	_, frames = w.askDungeon(t, dungeonExitID, "exit")
	if codes := w.appearAt(t, frames); string(codes) != string([]byte{byte(serverpackets.CompassGeneralZone)}) {
		t.Fatalf("compass after exit = %x, want 0f", codes)
	}

	_, frames = w.askDungeon(t, dungeonEntranceID, "necro")
	w.appearAt(t, frames)
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	drainFrames(t, w.c)
	if !w.savedInDungeon(t) {
		t.Fatal("saved out of the dungeon after going down")
	}
}

// savedInDungeon waits for queued saves and reads the player's persisted
// dungeon membership.
func (w *folkWorld) savedInDungeon(t *testing.T) bool {
	t.Helper()
	w.srv.FlushPersistence(t)
	var in bool
	if err := w.srv.DB.QueryRowContext(context.Background(),
		`SELECT isin7sdungeon FROM characters WHERE obj_Id = ?`, w.player).Scan(&in); err != nil {
		t.Fatal(err)
	}
	return in
}
