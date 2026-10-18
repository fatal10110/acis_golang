package npcs

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/castledefendteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The castle defend teleporters (CastleDefendTeleporter), driven through
// real Action and bypass packets against the shipped castles: the first
// talk reads the player's side in the gatekeeper's castle siege, and a link
// moves the player to one of the three points of its index.

// Gatekeepers of the shipped castles' residence NPCs: the first of Gludio
// Castle and the first of Aden Castle.
const (
	defendGludio1 = 35092
	defendAden1   = 35261
	defendClanID  = 0x70000041
	defendDir     = "script/teleport/CastleDefendTeleporter/"
)

// defendWorld is a booted server running the castle defend teleporters,
// with its character in the world and the script draws recorded.
type defendWorld struct {
	*folkWorld
	mu    sync.Mutex
	draws []int
	pick  int
}

// bootDefendWorld boots the teleporters with their datapack pages and the
// shipped castles and sieges; with side set, the character leads a clan
// registered on that side of Gludio Castle's siege ("OWNER" makes the clan
// the castle's owner); "" leaves the character in no clan. Every script draw
// answers pick.
func bootDefendWorld(t *testing.T, side string, pick int) *defendWorld {
	t.Helper()
	datapack.Require(t)
	pages := map[string]string{"test/any.htm": anyNpcPage}
	dir := datapack.Path(t, "data", "html", "script", "teleport", "CastleDefendTeleporter")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		pages[defendDir+e.Name()] = string(data)
	}
	dw := &defendWorld{pick: pick}
	kinds := map[int32]script.NPCKind{defendGludio1: script.KindFolk, defendAden1: script.KindFolk}
	list := []script.Listing{{Path: "script.teleport.CastleDefendTeleporter"}}
	catalog := script.Catalog{"script.teleport.CastleDefendTeleporter": castledefendteleporter.New}
	opts := []gameservertest.Option{
		shippedCastles(t),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int {
			dw.mu.Lock()
			defer dw.mu.Unlock()
			dw.draws = append(dw.draws, n)
			return dw.pick
		}),
		noBypassReuse,
	}
	if side != "" {
		opts = append(opts, gameservertest.WithClanSeed(func(db *sql.DB) {
			id := strconv.Itoa(defendClanID)
			stmts := []string{
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
					SELECT ` + id + `, 'Wardens', 5, obj_Id FROM characters WHERE char_name = 'Talker'`,
				`UPDATE characters SET clanid = ` + id + ` WHERE char_name = 'Talker'`,
			}
			if side == "OWNER" {
				stmts = append(stmts, `UPDATE clan_data SET hasCastle = 1 WHERE clan_id = `+id)
			} else {
				stmts = append(stmts, `INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+id+`, '`+side+`')`)
			}
			for _, stmt := range stmts {
				if _, err := db.ExecContext(context.Background(), stmt); err != nil {
					t.Fatalf("seed clan: %s: %v", stmt, err)
				}
			}
		}))
	}
	dw.folkWorld = bootFolkWorld(t, pages, opts...)
	return dw
}

// drawn returns the bounds of the script draws so far and forgets them.
func (w *defendWorld) drawn() []int {
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.draws
	w.draws = nil
	return d
}

// defendPage returns the gatekeeper's page name as n shows it.
func defendPage(t *testing.T, name string, n *npc.Folk) string {
	t.Helper()
	return scriptPage(t, defendDir+name, n)
}

// firstTalk selects the gatekeeper, talks to it and requires page.
func (w *defendWorld) firstTalk(t *testing.T, gk *npc.Folk, page string) {
	t.Helper()
	w.selectFolk(t, gk)
	wantTalkPage(t, w.talk(t, gk, false), gk, defendPage(t, page, gk))
}

// jumps returns where each of the player's own TeleportToLocation among
// frames lands.
func (w *defendWorld) jumps(frames [][]byte) []location.Location {
	var out []location.Location
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != w.player {
			continue
		}
		out = append(out, location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())})
	}
	return out
}

// requireStill requires the player to stand where it entered the world.
func (w *defendWorld) requireStill(t *testing.T, what string) {
	t.Helper()
	if x, y, z := w.srv.PlayerPosition(t, w.player); (location.Location{X: x, Y: y, Z: z}) != w.at {
		t.Fatalf("%s: player at %d,%d,%d, want still at %+v", what, x, y, z, w.at)
	}
}

// TestCastleDefendTeleporterMovesDefenders: a clan registered to defend
// Gludio Castle gets the first Gludio gatekeeper's page of links, and its
// "1" link moves the player to the drawn one of the three points of index
// 1, at no offset: one draw of bound 3, the jump with the releases of its
// own stops, and the player then stands there. The script answers nothing
// more: no page and no release of its own.
func TestCastleDefendTeleporterMovesDefenders(t *testing.T) {
	t.Parallel()
	w := bootDefendWorld(t, "DEFENDER", 1)
	gk := w.spawnFolk(t, folkTemplate("Gatekeeper", defendGludio1), 50)
	w.firstTalk(t, gk, "first_group_1.htm")

	frames := w.tripFrames(t, "Quest CastleDefendTeleporter 1")
	want := location.Location{X: -18615, Y: 108298, Z: -2556}
	if got := w.jumps(frames); !slices.Equal(got, []location.Location{want}) {
		t.Fatalf("jumps = %+v, want one to %+v", got, want)
	}
	if got, wantOps := opcodes(frames), opcodes(jumped); !slices.Equal(got, wantOps) {
		t.Fatalf("the link answered %x, want the jump alone %x", got, wantOps)
	}
	if got := w.drawn(); !slices.Equal(got, []int{3}) {
		t.Fatalf("script draws = %v, want one of bound 3", got)
	}
	if x, y, z := w.srv.PlayerPosition(t, w.player); (location.Location{X: x, Y: y, Z: z}) != want {
		t.Fatalf("player at %d,%d,%d, want %+v", x, y, z, want)
	}
}

// TestCastleDefendTeleporterOwnerPage: the clan owning Gludio Castle is on
// its siege's owner side, and gets the page of links too.
func TestCastleDefendTeleporterOwnerPage(t *testing.T) {
	t.Parallel()
	w := bootDefendWorld(t, "OWNER", 0)
	w.firstTalk(t, w.spawnFolk(t, folkTemplate("Gatekeeper", defendGludio1), 50), "first_group_1.htm")
}

// TestCastleDefendTeleporterRefusals: an attacking clan, a player in no
// clan, and a gatekeeper of a castle the player's clan is not registered
// at, each get the gatekeeper's refusal, which carries no link: a link
// sent anyway is dropped, nothing drawn and nobody moved.
func TestCastleDefendTeleporterRefusals(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, side string
		npcID      int
		page       string
	}{
		{name: "attacker", side: "ATTACKER", npcID: defendGludio1, page: "first_group_1_no.htm"},
		{name: "no clan", npcID: defendGludio1, page: "first_group_1_no.htm"},
		{name: "other castle", side: "DEFENDER", npcID: defendAden1, page: "second_group_no.htm"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootDefendWorld(t, tt.side, 0)
			gk := w.spawnFolk(t, folkTemplate("Gatekeeper", tt.npcID), 50)
			w.firstTalk(t, gk, tt.page)
			if got := w.bypass(t, "Quest CastleDefendTeleporter 1"); len(dialogFrames(got)) != 0 || len(w.jumps(got)) != 0 {
				t.Fatalf("a link off the refusal answered %x", opcodes(got))
			}
			if got := w.drawn(); len(got) != 0 {
				t.Fatalf("script draws = %v, want none", got)
			}
			w.requireStill(t, "a link off the refusal")
		})
	}
}

// TestCastleDefendTeleporterUnknownIndex: through a page that admits every
// link, an index the gatekeeper has no points for (the first Gludio
// gatekeeper goes to 0 and 1 only), a negative one, one past every
// gatekeeper's last, and an event naming no number each move nobody, draw
// nothing and answer nothing. The gatekeeper still answers a valid link
// afterwards.
func TestCastleDefendTeleporterUnknownIndex(t *testing.T) {
	t.Parallel()
	w := bootDefendWorld(t, "DEFENDER", 2)
	gk := w.spawnFolk(t, folkTemplate("Gatekeeper", defendGludio1), 50)
	w.firstTalk(t, gk, "first_group_1.htm")
	for _, event := range []string{"2", "-1", "5", "99999999999", "inner"} {
		w.openAnyNpcPage(t)
		if got := w.bypass(t, "Quest CastleDefendTeleporter "+event); len(dialogFrames(got)) != 0 || len(w.jumps(got)) != 0 {
			t.Fatalf("event %q answered %x, want nothing", event, opcodes(got))
		}
		if got := w.drawn(); len(got) != 0 {
			t.Fatalf("event %q: script draws = %v, want none", event, got)
		}
		w.requireStill(t, "event "+strings.TrimSpace(event))
	}
	w.openAnyNpcPage(t)
	want := location.Location{X: -18123, Y: 109947, Z: -2492}
	if got := w.jumps(w.bypass(t, "Quest CastleDefendTeleporter 0")); !slices.Equal(got, []location.Location{want}) {
		t.Fatalf("jumps = %+v, want one to %+v", got, want)
	}
}
