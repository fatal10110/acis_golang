package admin

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// manualDecay is a corpse-decay task on a clock the test moves; a corpse
// whose deadline has passed decays in the booted world, as in production.
type manualDecay struct {
	task *task.Decay

	mu    sync.Mutex
	now   time.Time
	state *world.State
}

func newManualDecay(t *testing.T) *manualDecay {
	t.Helper()
	d := &manualDecay{now: time.Unix(1_700_000_000, 0)}
	decay, err := task.NewDecay(d, d.clock)
	if err != nil {
		t.Fatalf("task.NewDecay: %v", err)
	}
	d.task = decay
	return d
}

func (d *manualDecay) clock() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.now
}

func (d *manualDecay) Decay(actor task.DecayActor) {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	obj, ok := state.Object(actor.ObjectID())
	if !ok {
		return
	}
	if corpse, ok := obj.(interface {
		Decay(*world.State, func()) bool
	}); ok {
		corpse.Decay(state, nil)
	}
}

// passAndTick moves the clock on by elapsed, runs one decay sweep and waits
// for what it posted.
func (d *manualDecay) passAndTick(t *testing.T, srv *gameservertest.Server, elapsed time.Duration) {
	t.Helper()
	d.mu.Lock()
	d.now = d.now.Add(elapsed)
	d.mu.Unlock()
	if err := d.task.Tick(); err != nil {
		t.Fatalf("decay tick: %v", err)
	}
	srv.Settle(t)
}

// TestAdminManageSummon pins //heal, //kill and //res on the GM's selected
// pet: the heal fills its HP and MP; the kill puts it to death, paying its
// death penalty; the res stands it up without giving that experience back,
// and its corpse no longer decays: it is still out, alive, well past the
// pet corpse's twenty minutes.
func TestAdminManageSummon(t *testing.T) {
	t.Parallel()
	decay := newManualDecay(t)
	srv, gmID := bootAdmin(t, adminLevel, append(summonAdminOptions(t), gameservertest.WithDecay(decay.task))...)
	decay.state = srv.State
	collar := srv.GiveItem(t, gmID, infoCollarID, 1)
	gm := srv.Client
	enterWorld(t, gm)
	pet := callPet(t, srv, gm, gmID, collar)
	exchange(t, gm, encodeAction(pet.ObjectID()))
	drain(t, gm)

	onParaQueue(t, pet, func() {
		pet.SetHP(10)
		pet.ReduceMP(pet.MPValue())
	})
	requireNoSystemMessage(t, "//heal", exchange(t, gm, encodeBuildCmd("heal")))
	onParaQueue(t, pet, func() {
		if pet.HP() != pet.MaxHPValue() || pet.MPValue() != pet.MaxMPValue() {
			t.Errorf("healed pet HP %v/%v MP %v/%v, want both full", pet.HP(), pet.MaxHPValue(), pet.MPValue(), pet.MaxMPValue())
		}
	})

	before := pet.Exp()
	exchange(t, gm, encodeBuildCmd("kill"))
	onParaQueue(t, pet, func() {})
	if !pet.Dead() {
		t.Fatal("pet alive after //kill")
	}
	dead := pet.Exp()
	if dead >= before {
		t.Fatalf("pet exp after death = %d, want under %d", dead, before)
	}
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("res"))
	onParaQueue(t, pet, func() {})
	frames = append(frames, settle(t, gm)...)
	if !slices.Contains(opcodesOf(frames), serverpackets.OpcodeRevive) {
		t.Fatalf("//res frames = %x, want the pet's Revive", testsupport.FrameOpcodes(frames))
	}
	if pet.Dead() {
		t.Fatal("pet dead after //res")
	}
	if got := pet.Exp(); got != dead {
		t.Fatalf("pet exp after //res = %d, want %d: //res gives no experience back", got, dead)
	}

	decay.passAndTick(t, srv, 21*time.Minute)
	if obj, ok := srv.State.Summon(gmID); !ok || obj.ObjectID() != pet.ObjectID() || pet.Dead() {
		t.Fatal("revived pet left the world at its old corpse decay")
	}
}

// TestAdminManageFolkHeal pins //heal on a selected civilian NPC: its HP and
// MP fill.
func TestAdminManageFolkHeal(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	x, y, z := srv.PlayerPosition(t, gmID)
	f := srv.SpawnFolkNPCAt(t, &npc.Template{
		ID: 31932, TemplateID: 31932, Type: "Merchant", Level: 1, HPMax: 1000, MPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, location.Location{X: x + 60, Y: y, Z: z})
	drain(t, gm)
	onParaQueue(t, f.Queue(), func() {
		f.SetHP(10)
		f.ReduceMP(f.MPValue())
	})
	exchange(t, gm, encodeAction(f.ObjectID()))

	requireNoSystemMessage(t, "//heal", exchange(t, gm, encodeBuildCmd("heal")))
	onParaQueue(t, f.Queue(), func() {
		if f.HP() != f.MaxHPValue() || f.MPValue() != f.MaxMPValue() {
			t.Errorf("healed folk HP %v/%v MP %v/%v, want both full", f.HP(), f.MaxHPValue(), f.MPValue(), f.MaxMPValue())
		}
	})
}
