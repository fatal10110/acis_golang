package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Compass codes (ExSetCompassZoneCode).
const (
	compassSevenSigns = 0x0d
	compassGeneral    = 0x0f
)

// dungeonTown is where an expelled player lands.
var dungeonTown = location.Location{X: -5000, Y: 2000, Z: 30}

// dawnLeads is a competition Dawn leads, in period.
func dawnLeads(period sevensigns.Period) func(*sevensigns.StatusRow) {
	return func(row *sevensigns.StatusRow) {
		row.Period = period
		row.DawnStoneScore = 100
	}
}

// bootInDungeon boots Newbie stored in a Seven Signs dungeon, signed up for
// cabal (none for NoCabal), in period with Dawn leading.
func bootInDungeon(t *testing.T, period sevensigns.Period, cabal sevensigns.Cabal) *sevenSignsWorld {
	t.Helper()
	return bootSevenSigns(t, sevenSignsSetup{status: dawnLeads(period), cabal: cabal, dungeon: true},
		gameservertest.WithRestartPoints(escapeTown(dungeonTown)))
}

// teleportedToTown reports whether frames hold Newbie's TeleportToLocation
// within 20 of dungeonTown, and its index.
func teleportedToTown(t *testing.T, w *sevenSignsWorld, frames [][]byte) (int, bool) {
	t.Helper()
	i := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation)
	if i < 0 {
		return -1, false
	}
	r := wire.NewReader(frames[i][1:])
	id, x, y := r.ReadInt32(), int(r.ReadInt32()), int(r.ReadInt32())
	if id != w.objID || x < dungeonTown.X-20 || x > dungeonTown.X+20 || y < dungeonTown.Y-20 || y > dungeonTown.Y+20 {
		t.Fatalf("TeleportToLocation = %d at %d, %d, want %d within 20 of %v", id, x, y, w.objID, dungeonTown)
	}
	return i, true
}

// restart leaves the world for character selection, which saves Newbie.
func (w *sevenSignsWorld) restart(t *testing.T) {
	t.Helper()
	c := w.srv.Client
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("restart opcode = %#x, want RestartResponse", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-restart opcode = %#x, want CharSelectInfo", reply[0])
	}
}

// TestSevenSignsDungeonKeptAcrossRelog pins characters.isin7sdungeon: a
// Dawn member stored in a dungeon during the competition may stay there,
// enters the world with the Seven Signs compass code (0x0D), and is saved
// and restored still in it.
func TestSevenSignsDungeonKeptAcrossRelog(t *testing.T) {
	t.Parallel()
	w := bootSevenSigns(t, sevenSignsSetup{status: dawnLeads(sevensigns.Competition), cabal: sevensigns.Dawn, dungeon: true},
		gameservertest.WithRestartPoints(escapeTown(dungeonTown)), gameservertest.WithReuseDelays(0, 0))
	for login := range 2 {
		frames := w.enter(t)
		assertCompassCodes(t, frames, compassSevenSigns)
		if _, ok := teleportedToTown(t, w, frames); ok {
			t.Fatalf("login %d frames = %x, want no expulsion", login, opcodes(frames))
		}
		w.restart(t)
		if !persistedCharacter(t, w.srv, w.objID).In7sDungeon() {
			t.Fatalf("login %d: saved out of the dungeon, want still in it", login)
		}
	}
}

// TestSevenSignsDungeonLoginCheck pins Player.onPlayerEnter: a player
// entering the world in a dungeon it is no longer allowed in — outside the
// winning cabal during results or seal validation, in no cabal otherwise —
// is sent to town and out of the dungeon, after the login's compass code
// showed the dungeon; it lands with the general code and is saved out of
// the dungeon. Anyone else stays.
func TestSevenSignsDungeonLoginCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		period   sevensigns.Period
		cabal    sevensigns.Cabal
		expelled bool
	}{
		{"no cabal in competition", sevensigns.Competition, sevensigns.NoCabal, true},
		{"member in recruiting", sevensigns.Recruiting, sevensigns.Dusk, false},
		{"loser in results", sevensigns.Results, sevensigns.Dusk, true},
		{"winner in seal validation", sevensigns.SealValidation, sevensigns.Dawn, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootInDungeon(t, tc.period, tc.cabal)
			frames := w.enter(t)
			teleport, ok := teleportedToTown(t, w, frames)
			if ok != tc.expelled || w.online(t).In7sDungeon() == tc.expelled {
				t.Fatalf("login frames = %x, in dungeon %v; want expelled %v", opcodes(frames), w.online(t).In7sDungeon(), tc.expelled)
			}
			assertCompassCodes(t, frames, compassSevenSigns)
			if !tc.expelled {
				return
			}
			if login := firstOpcode(frames, serverpackets.OpcodeSkillCoolTime); login < teleport {
				t.Fatalf("login frames = %x, want the expulsion ahead of the burst's SkillCoolTime", opcodes(frames))
			}
			assertCompassCodes(t, appear(t, w.srv.Client), compassGeneral)
			w.restart(t)
			if persistedCharacter(t, w.srv, w.objID).In7sDungeon() {
				t.Fatal("saved in the dungeon after the expulsion, want out of it")
			}
		})
	}
}

// TestSevenSignsDungeonPeriodChangeSweep pins teleLosingCabalFromDungeons:
// once a period change is saved, a player online in a dungeon it may no
// longer stay in is sent to town and out of the dungeon, ahead of the new
// sky. As results begin only the winning cabal stays. In the other periods
// the sweep reverses the login rule: a cabal member is sent out.
func TestSevenSignsDungeonPeriodChangeSweep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		period   sevensigns.Period
		cabal    sevensigns.Cabal
		expelled bool
	}{
		{"loser as results begin", sevensigns.Competition, sevensigns.Dusk, true},
		{"winner as results begin", sevensigns.Competition, sevensigns.Dawn, false},
		{"member as the competition begins", sevensigns.Recruiting, sevensigns.Dawn, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootInDungeon(t, tc.period, tc.cabal)
			w.enter(t)
			frames := w.fire(t)
			teleport, ok := teleportedToTown(t, w, frames)
			if ok != tc.expelled || w.online(t).In7sDungeon() == tc.expelled {
				t.Fatalf("period change frames = %x, in dungeon %v; want expelled %v", opcodes(frames), w.online(t).In7sDungeon(), tc.expelled)
			}
			sky := firstOpcode(frames, serverpackets.OpcodeSSQInfo)
			if sky < 0 || sky != len(frames)-1 || (ok && teleport > sky) {
				t.Fatalf("period change frames = %x, want the expulsion ahead of the closing SSQInfo", opcodes(frames))
			}
		})
	}
}

// TestRestartPointLeavesSevenSignsDungeon pins RequestRestartPoint.portPlayer:
// a dead player restarting in town leaves its dungeon.
func TestRestartPointLeavesSevenSignsDungeon(t *testing.T) {
	t.Parallel()
	w := bootInDungeon(t, sevensigns.Competition, sevensigns.Dawn)
	if _, err := w.srv.DB.Exec("UPDATE characters SET curHp = 0 WHERE obj_Id = ?", w.objID); err != nil {
		t.Fatal(err)
	}
	if frames := w.enter(t); firstOpcode(frames, serverpackets.OpcodeDie) < 0 {
		t.Fatalf("login frames = %x, want a Die", opcodes(frames))
	}
	req := wire.NewPacketWriter(clientpackets.OpcodeRequestRestartPoint)
	req.WriteInt32(0)
	w.srv.Client.Send(req.Bytes())
	if _, ok := teleportedToTown(t, w, readUntilQuiet(w.srv.Client)); !ok {
		t.Fatal("restart point sent nobody to town")
	}
	if w.online(t).In7sDungeon() {
		t.Fatal("restarted in town still in the dungeon")
	}
}
