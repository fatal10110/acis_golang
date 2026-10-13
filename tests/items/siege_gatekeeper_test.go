package items

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Reference: a TELEPORTER ticket hires a civilian gatekeeper. Castle.
// removeOwner kills every hired NPC (npc.doDie(npc)) whatever its type,
// then forgets them: the corpse leaves the world when it decays
// (DecayTaskManager), as no later despawn knows it.

// gludioGatekeeperTicket is castles.xml's TELEPORTER ticket of Gludio, max 1.
const gludioGatekeeperTicket int32 = 3970

// gludioGatekeeper is the ticket's npcId, of type Folk.
const gludioGatekeeper = 35092

// shippedGatekeeper is the shipped template of Gludio's hired gatekeeper.
var shippedGatekeeper = sync.OnceValue(func() *npc.Template {
	// Callers run datapack.Require first, so the checkout is present here.
	dir, _ := datapack.Find()
	only, err := os.MkdirTemp("", "gatekeeper-npcs")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(only)
	src, err := os.ReadFile(filepath.Join(dir, "data", "xml", "npcs", "35000-35999.xml"))
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(only, "35000-35999.xml"), src, 0o644); err != nil {
		panic(err)
	}
	skills, items := shippedData()
	npcs, err := gamexml.LoadNPCTemplates(only, items, skills, zerolog.Nop())
	if err != nil {
		panic(err)
	}
	tmpl, ok := npcs.Get(gludioGatekeeper)
	if !ok {
		panic("Gludio gatekeeper template missing")
	}
	return tmpl
})

// gatekeeperCorpses is a corpse-decay task on a clock the scenario moves by
// hand; a due corpse still in the world decays there.
type gatekeeperCorpses struct {
	task *task.Decay

	mu    sync.Mutex
	now   time.Time
	state *world.State
}

func newGatekeeperCorpses(t *testing.T) *gatekeeperCorpses {
	t.Helper()
	d := &gatekeeperCorpses{now: time.Unix(1_700_000_000, 0)}
	decay, err := task.NewDecay(d, d.clock)
	if err != nil {
		t.Fatalf("new decay: %v", err)
	}
	d.task = decay
	return d
}

func (d *gatekeeperCorpses) clock() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.now
}

func (d *gatekeeperCorpses) Decay(actor task.DecayActor) {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	if obj, ok := state.Object(actor.ObjectID()); ok {
		if corpse, ok := obj.(interface {
			Decay(*world.State, func()) bool
		}); ok {
			corpse.Decay(state, nil)
		}
	}
}

// pass moves the clock on by elapsed and runs one decay sweep.
func (d *gatekeeperCorpses) pass(t *testing.T, srv *gameservertest.Server, elapsed time.Duration) {
	t.Helper()
	d.mu.Lock()
	d.now = d.now.Add(elapsed)
	d.state = srv.State
	d.mu.Unlock()
	if err := d.task.Tick(); err != nil {
		t.Fatalf("decay tick: %v", err)
	}
	srv.Settle(t)
}

// gatekeepers returns the NPCs of the hired gatekeeper's template in the
// world.
func (w *castleWorld) gatekeepers() []*npc.Folk {
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.Instance.Template.ID == gludioGatekeeper {
			out = append(out, f)
		}
	}
	return out
}

// TestSiegeGatekeeperDiesWithOwner places Gludio's shipped TELEPORTER
// ticket and starts the siege: the shipped gatekeeper is hired where the
// ticket lay. Removing Gludio's owner kills it (Die); the siege's end
// leaves its corpse alone, and the corpse decays out of the world.
func TestSiegeGatekeeperDiesWithOwner(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	_, shippedItems := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{circletOfGludioID, lordsCrownID, gludioGatekeeperTicket} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	corpses := newGatekeeperCorpses(t)
	w := bootTicketWorld(t, ticketSetup{ownsCastle: 1, attacker: true},
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{shippedGatekeeper()})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithDecay(corpses.task),
	)
	ticket := w.srv.GiveItem(t, w.leaderID, gludioGatekeeperTicket, 1)
	w.enter(t)
	_, x, y := w.placeTicket(t, ticket)

	gludio := gludioSiegeOf(t, w)
	gludio.Start()
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	hired := w.gatekeepers()
	if len(hired) != 1 {
		t.Fatalf("gatekeepers at the start = %d, want 1", len(hired))
	}
	keeper := hired[0]
	if gx, gy, _ := keeper.Position(); int32(gx) != x || int32(gy) != y {
		t.Fatalf("gatekeeper at (%d, %d), want the ticket's (%d, %d)", gx, gy, x, y)
	}
	if keeper.Instance.CastleID != 1 {
		t.Fatalf("gatekeeper castle = %d, want Gludio (1)", keeper.Instance.CastleID)
	}

	gm, _ := w.castleCommand(t, "remove gludio_castle")
	var died bool
	for _, f := range gm {
		if f[0] == serverpackets.OpcodeDie && wire.NewReader(f[1:]).ReadInt32() == keeper.ObjectID() {
			died = true
		}
	}
	if !died || !keeper.Dead() {
		t.Fatalf("gatekeeper after the owner's removal: dead %v, Die seen %v among opcodes %x", keeper.Dead(), died, opcodesOf(gm))
	}

	gludio.End()
	w.srv.Settle(t)
	if !keeper.HasCorpse() {
		t.Fatal("the siege's end took the gatekeeper's corpse before it decayed")
	}
	corpses.pass(t, w.srv, keeper.CorpseTime()+time.Second)
	if left := w.gatekeepers(); len(left) != 0 {
		t.Fatalf("gatekeepers after the end and the corpse time = %d, want none", len(left))
	}
}
