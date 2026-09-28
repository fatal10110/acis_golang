package pets

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// corpseDecay is a corpse-decay task on a clock the scenario moves by hand.
// Its effect is the production one: a due actor still in the world decays.
type corpseDecay struct {
	task *task.Decay

	mu    sync.Mutex
	now   time.Time
	state *world.State
}

func newCorpseDecay(t *testing.T) *corpseDecay {
	t.Helper()
	d := &corpseDecay{now: time.Unix(1_700_000_000, 0)}
	decay, err := task.NewDecay(d, d.clock)
	if err != nil {
		t.Fatalf("task.NewDecay: %v", err)
	}
	d.task = decay
	return d
}

func (d *corpseDecay) clock() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.now
}

// Decay mirrors the production decay effect: the actor is looked up in the
// world and decays there.
func (d *corpseDecay) Decay(actor task.DecayActor) {
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

// attach points the decay effect at the booted world.
func (d *corpseDecay) attach(state *world.State) {
	d.mu.Lock()
	d.state = state
	d.mu.Unlock()
}

// passAndTick moves the clock on by elapsed, runs one decay sweep and waits
// for every decay it posted.
func (d *corpseDecay) passAndTick(t *testing.T, srv *gameservertest.Server, elapsed time.Duration) {
	t.Helper()
	d.mu.Lock()
	d.now = d.now.Add(elapsed)
	d.mu.Unlock()
	if err := d.task.Tick(); err != nil {
		t.Fatalf("decay tick: %v", err)
	}
	srv.Settle(t)
}

// TestDecayedPetCorpseTakesItsCollar kills a pet carrying adena. Its corpse
// stays for 20 minutes; when it decays, the pet leaves the world with its
// adena back in its owner's inventory, and the owner loses the collar and
// the pet's saved row with it.
func TestDecayedPetCorpseTakesItsCollar(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithDecay(decay.task)},
		seedItem{TemplateID: item.AdenaID, Count: 40})
	decay.attach(h.srv.State)
	pet, _ := h.spawnWolf(t)
	h.giveToPet(t, h.seededItem(t, item.AdenaID), 40)
	killPet(t, h, pet)

	decay.passAndTick(t, h.srv, 1199*time.Second)
	if _, ok := h.srv.State.Summon(h.ownerID); !ok {
		t.Fatal("pet corpse decayed before its 20 minutes were up")
	}

	decay.passAndTick(t, h.srv, 2*time.Second)
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete for the decayed pet")
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("owner still holds the summon slot after the pet corpse decayed")
	}
	if _, ok := h.srv.State.Object(pet.ObjectID()); ok {
		t.Fatal("decayed pet is still in the world")
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 0 {
		t.Fatalf("owner collar count after decay = %d, want the collar destroyed", got)
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("owner adena after decay = %d, want the pet's 40 back", got)
	}
	h.srv.FlushPersistence(t)
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row after decay: present=%v err=%v, want it deleted", ok, err)
	}
}

// TestPetCorpseLeftByItsOwnerNeverDecays logs the owner out with a dead pet:
// the corpse leaves with its owner, so its decay entry is dropped, and the
// collar and the pet's row both survive.
func TestPetCorpseLeftByItsOwnerNeverDecays(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
	})
	decay.attach(h.srv.State)
	pet, _ := h.spawnWolf(t)
	killPet(t, h, pet)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	decay.passAndTick(t, h.srv, time.Hour)

	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("collar count after the owner left with a dead pet = %d, want it kept", got)
	}
	if got := h.savedPetState(t).CurHP; got != 0 {
		t.Fatalf("saved dead pet HP = %v, want 0", got)
	}
}

// servitorOwner is an owner in the world who can cast a servitor summon.
type servitorOwner struct {
	srv    *gameservertest.Server
	client *testsupport.ScriptedClient
	id     int32
}

const (
	killableServitorSkill = 1112
	killableServitorNPCID = 12601
	// servitorCorpseTime is the servitor template's corpse time, in seconds.
	servitorCorpseTime = 7
)

// bootServitorOwner boots an owner who knows a servitor summon skill.
func bootServitorOwner(t *testing.T, extra ...gameservertest.Option) *servitorOwner {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: killableServitorSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: killableServitorNPCID, SummonTotalLifeTime: 1_200_000,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}}), gamesql.NewCharacterSkillStore(db))
	cat := &npc.Template{
		ID: killableServitorNPCID, TemplateID: killableServitorNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20, CorpseTime: servitorCorpseTime,
	}
	srv := bootPets(t, append([]gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{cat})),
		gameservertest.WithSkills(skills),
	}, extra...)...)
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, killableServitorSkill, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	startInWorld(t, srv.Client)
	return &servitorOwner{srv: srv, client: srv.Client, id: ownerID}
}

// summonServitor casts the servitor summon and waits for the servitor.
func (o *servitorOwner) summonServitor(t *testing.T) *summon.Actor {
	t.Helper()
	o.client.Send(encodeRequestMagicSkillUse(killableServitorSkill))
	var servitor *summon.Actor
	o.srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := o.srv.State.Summon(o.id)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	drainUntilQuiet(t, o.client)
	return servitor
}

// TestKilledServitorPassesAwayAndItsCorpseDecays kills a servitor. Its owner
// sees it die and reads the servitor death message before the killing hit's
// damage message; its lifetime stops counting down. Once its template's
// corpse time is up the corpse decays, which frees the owner's summon slot
// for the next servitor.
func TestKilledServitorPassesAwayAndItsCorpseDecays(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	o := bootServitorOwner(t, gameservertest.WithDecay(decay.task))
	decay.attach(o.srv.State)
	servitor := o.summonServitor(t)
	queue := o.srv.PlayerQueue(t, o.id)

	before := servitor.Lifetime().TimeRemaining
	runOn(t, queue, func() { servitor.TickServitor(o.srv.State) })
	alive := servitor.Lifetime().TimeRemaining
	if alive >= before {
		t.Fatalf("living servitor lifetime = %d after a tick, want it below %d", alive, before)
	}

	owner, _ := o.srv.State.Player(o.id)
	runOn(t, queue, func() {
		servitor.ReduceHP(servitor.HP()+100, owner.(attackable.Combatant), modelskill.Definition{})
	})
	if !servitor.Dead() {
		t.Fatal("servitor alive after a lethal hit")
	}
	id := servitor.ObjectID()
	got := deathTags(t, drainFrames(t, o.client), id, o.id)
	want := []string{
		fmt.Sprintf("Die %d", id),
		fmt.Sprintf("AutoAttackStop %d", id),
		fmt.Sprintf("SystemMessage %d", serverpackets.SystemMessageServitorPassedAway),
		fmt.Sprintf("SystemMessage %d", serverpackets.SystemMessageSummonReceivedS2ByS1),
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("owner death frames = %q, want %q", got, want)
	}

	runOn(t, queue, func() { servitor.TickServitor(o.srv.State) })
	if got := servitor.Lifetime().TimeRemaining; got != alive {
		t.Fatalf("dead servitor lifetime = %d after a tick, want %d kept", got, alive)
	}

	decay.passAndTick(t, o.srv, (servitorCorpseTime-1)*time.Second)
	if _, ok := o.srv.State.Summon(o.id); !ok {
		t.Fatal("servitor corpse decayed before its corpse time was up")
	}
	decay.passAndTick(t, o.srv, 2*time.Second)
	frames := readUntilOpcode(t, o.client, serverpackets.OpcodePetDelete, "PetDelete for the decayed servitor")
	if typ := wire.NewReader(frames[len(frames)-1][1:]).ReadInt32(); typ != 1 {
		t.Fatalf("PetDelete summon type = %d, want 1 (servitor)", typ)
	}
	if _, ok := o.srv.State.Summon(o.id); ok {
		t.Fatal("owner still holds the summon slot after the servitor corpse decayed")
	}
	drainUntilQuiet(t, o.client)

	if next := o.summonServitor(t); next.ObjectID() == id || next.Dead() {
		t.Fatalf("resummon after decay = %d dead %v, want a new living servitor", next.ObjectID(), next.Dead())
	}
}
