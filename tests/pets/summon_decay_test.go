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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
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
	return newCorpseDecayAt(t, time.Unix(1_700_000_000, 0))
}

// newCorpseDecayAt is newCorpseDecay with its clock starting at start.
func newCorpseDecayAt(t *testing.T, start time.Time) *corpseDecay {
	t.Helper()
	d := &corpseDecay{now: start}
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

// TestPetCorpseLeftByItsOwnerDecaysOffline logs the owner out with a dead
// pet carrying adena. The pet's adena and its row are settled at once, but
// its corpse stays where it lay, in view and holding the owner's summon slot,
// until its 20 minutes are up. It then decays with the owner still away: an
// observer sees it go, and the collar and the pets row are deleted from the
// database.
func TestPetCorpseLeftByItsOwnerDecaysOffline(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
	}, seedItem{TemplateID: item.AdenaID, Count: 40})
	decay.attach(h.srv.State)
	pet, _ := h.spawnWolf(t)
	h.giveToPet(t, h.seededItem(t, item.AdenaID), 40)
	observer, _ := joinSecondPlayer(t, h.srv)
	drainUntilQuiet(t, h.client)
	killPet(t, h, pet)
	drainUntilQuiet(t, observer)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	if _, ok := h.srv.State.Object(pet.ObjectID()); !ok {
		t.Fatal("pet corpse left the world with its owner, want it kept until it decays")
	}
	if sawDeleteObject(drainFrames(t, observer), pet.ObjectID()) {
		t.Fatal("observer saw the pet corpse removed at its owner's logout")
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("owner adena after logout = %d, want the dead pet's 40 back", got)
	}
	if got := h.savedPetState(t).CurHP; got != 0 {
		t.Fatalf("saved dead pet HP = %v, want 0", got)
	}

	decay.passAndTick(t, h.srv, 1199*time.Second)
	if _, ok := h.srv.State.Object(pet.ObjectID()); !ok {
		t.Fatal("pet corpse decayed before its 20 minutes were up")
	}
	decay.passAndTick(t, h.srv, 2*time.Second)
	if _, ok := h.srv.State.Object(pet.ObjectID()); ok {
		t.Fatal("pet corpse still in the world after its decay")
	}
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("the decayed corpse still holds its offline owner's summon slot")
	}
	h.srv.AdvanceUntil(t, "observer sees the corpse removed", func() bool {
		return sawDeleteObject(drainFrames(t, observer), pet.ObjectID())
	})
	h.srv.FlushPersistence(t)
	if got := h.ownerItemCount(t, wolfCollarID); got != 0 {
		t.Fatalf("offline owner's collar count after decay = %d, want the collar deleted", got)
	}
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row after decay: present=%v err=%v, want it deleted", ok, err)
	}
}

// TestPetCorpseWaitsForItsOwnerToComeBack logs the owner out with a dead pet
// and back in before the corpse decays. The corpse is still the owner's pet:
// the owner sees its pet window again and cannot call out another summon.
// When the corpse decays, the new session loses the collar and gets the
// PetDelete.
func TestPetCorpseWaitsForItsOwnerToComeBack(t *testing.T) {
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
	decay.passAndTick(t, h.srv, 10*time.Minute)

	h.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelected, "CharSelected")
	h.client.Send(encodeEnterWorld())
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "end of the EnterWorld burst")
	frames = append(frames, drainFrames(t, h.client)...)
	if !sawPetInfo(frames, pet.ObjectID()) {
		t.Fatal("returning owner got no PetInfo for its pet's corpse")
	}

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "collar refusal"), serverpackets.SystemMessageSummonOnlyOne)
	drainUntilQuiet(t, h.client)

	decay.passAndTick(t, h.srv, 10*time.Minute+time.Second)
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete for the decayed corpse")
	h.srv.AdvanceUntil(t, "collar destroyed", func() bool {
		return h.ownerInventory(t).ItemByObjectID(h.collarID) == nil
	})
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("owner still holds the summon slot after the corpse decayed")
	}
	h.srv.FlushPersistence(t)
	if got := h.ownerItemCount(t, wolfCollarID); got != 0 {
		t.Fatalf("collar count after decay = %d, want the collar destroyed", got)
	}
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row after decay: present=%v err=%v, want it deleted", ok, err)
	}
}

// sawPetInfo reports whether frames hold a PetInfo for objectID.
func sawPetInfo(frames [][]byte, objectID int32) bool {
	for _, frame := range frames {
		if len(frame) == 0 || frame[0] != serverpackets.OpcodePetInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		r.ReadInt32() // summon type
		if r.ReadInt32() == objectID {
			return true
		}
	}
	return false
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
	return bootServitorOwnerWithSkills(t, nil, extra...)
}

// bootServitorOwnerWithSkills boots an owner who knows a servitor summon
// skill and every skill in known.
func bootServitorOwnerWithSkills(t *testing.T, known []modelskill.Definition, extra ...gameservertest.Option) *servitorOwner {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(append([]modelskill.Definition{{
		ID: killableServitorSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: killableServitorNPCID, SummonTotalLifeTime: 1_200_000,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}}, known...)), gamesql.NewCharacterSkillStore(db))
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
	for _, def := range known {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, int(def.ID), int(def.Level)); err != nil {
			t.Fatalf("seed known skill %d: %v", def.ID, err)
		}
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

// killServitor lands a lethal hit on servitor from its owner, on the owner's
// queue, and drains what the owner is told.
func (o *servitorOwner) killServitor(t *testing.T, servitor *summon.Actor) {
	t.Helper()
	owner, _ := o.srv.State.Player(o.id)
	runOn(t, o.srv.PlayerQueue(t, o.id), func() {
		servitor.ReduceHP(servitor.HP()+100, owner.(attackable.Combatant), modelskill.Definition{})
	})
	if !servitor.Dead() {
		t.Fatal("servitor alive after a lethal hit")
	}
	drainUntilQuiet(t, o.client)
}

// TestServitorCorpseLeftByItsOwnerDoesNotHoldTheSlot logs the owner out with
// a dead servitor and back in before its corpse decays. The corpse stays in
// the world, no longer the owner's: the returning owner sees it as any other
// summon and summons a new servitor at once. The old corpse still decays at
// its deadline, and the new servitor stays.
func TestServitorCorpseLeftByItsOwnerDoesNotHoldTheSlot(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	o := bootServitorOwner(t, gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0))
	decay.attach(o.srv.State)
	corpse := o.summonServitor(t)
	o.killServitor(t, corpse)

	o.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, o.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	if _, ok := o.srv.State.Object(corpse.ObjectID()); !ok {
		t.Fatal("servitor corpse left the world with its owner, want it kept until it decays")
	}
	if _, ok := o.srv.State.Summon(o.id); ok {
		t.Fatal("servitor corpse still holds its departed owner's summon slot")
	}

	o.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, o.client, serverpackets.OpcodeCharSelected, "CharSelected")
	o.client.Send(encodeEnterWorld())
	frames := readUntilOpcode(t, o.client, serverpackets.OpcodeActionFailed, "end of the EnterWorld burst")
	frames = append(frames, drainFrames(t, o.client)...)
	if sawServitorPetInfo(frames, corpse.ObjectID()) {
		t.Fatal("returning owner got a PetInfo for the servitor corpse it left behind")
	}

	next := o.summonServitor(t)
	if next.ObjectID() == corpse.ObjectID() || next.Dead() {
		t.Fatalf("summon after relog = %d dead %v, want a new living servitor", next.ObjectID(), next.Dead())
	}
	decay.passAndTick(t, o.srv, (servitorCorpseTime+1)*time.Second)
	if _, ok := o.srv.State.Object(corpse.ObjectID()); ok {
		t.Fatal("servitor corpse still in the world past its corpse time")
	}
	if got, ok := o.srv.State.Summon(o.id); !ok || got.ObjectID() != next.ObjectID() {
		t.Fatal("the old corpse's decay took the owner's new servitor's slot")
	}
}

// TestCorpseMobSkillsOnServitorCorpse casts CORPSE_MOB skills on a dead
// servitor, whose corpse awaits its decay like an NPC's: harvest fails for
// want of a seed, sweep for want of a spoil, and past half its corpse time
// the corpse is too old for either.
func TestCorpseMobSkillsOnServitorCorpse(t *testing.T) {
	t.Parallel()
	sweep := modelskill.Definition{
		ID: 42, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpseMob, SkillType: "SWEEP",
		CastRange: 600, HitTime: 500, ReuseDelay: 500,
		StaticHitTime: true, StaticReuse: true, Power: 100,
	}
	harvest := sweep
	harvest.ID, harvest.SkillType = 432, "HARVEST"
	for _, tt := range []struct {
		name string
		def  modelskill.Definition
		// age is how long before now the servitor's corpse started.
		age     time.Duration
		message int
	}{
		{"harvest on a servitor corpse", harvest, 0, serverpackets.SystemMessageHarvestFailedSeedNotSown},
		{"sweep on a servitor corpse", sweep, 0, serverpackets.SystemMessageSweeperFailedTargetNotSpoiled},
		{"sweep on a too-old servitor corpse", sweep, 4 * time.Second, serverpackets.SystemMessageCorpseTooOldSkillNotUsed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			decay := newCorpseDecayAt(t, time.Now().Add(-tt.age))
			o := bootServitorOwnerWithSkills(t, []modelskill.Definition{tt.def}, gameservertest.WithDecay(decay.task))
			decay.attach(o.srv.State)
			servitor := o.summonServitor(t)
			x, y, z := servitor.Position()
			o.client.Send(encodeAction(servitor.ObjectID(), int32(x), int32(y), int32(z), false))
			readUntilOpcode(t, o.client, serverpackets.OpcodeMyTargetSelected, "servitor selected")
			drainUntilQuiet(t, o.client)
			o.killServitor(t, servitor)
			if !servitor.HasCorpse() {
				t.Fatal("dead servitor has no corpse awaiting its decay")
			}

			o.client.Send(encodeRequestMagicSkillUse(int32(tt.def.ID)))
			assertStaticSystemMessage(t, mustRead(t, o.client, tt.name), tt.message)
		})
	}
}

// relogOwner logs the owner out to character select and back into the world.
func (h *petWorld) relogOwner(t *testing.T) {
	t.Helper()
	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	h.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelected, "CharSelected")
	h.client.Send(encodeEnterWorld())
	readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "end of the EnterWorld burst")
	drainUntilQuiet(t, h.client)
}

// TestPetRestoredDeadLeavesWithItsOwner calls out a wolf whose row was saved
// dead, as a corpse lost to a restart leaves it. Nothing will ever decay
// that corpse, so it leaves the world with its owner, and the owner can call
// the wolf out again after logging back in: it comes back dead from its row.
func TestPetRestoredDeadLeavesWithItsOwner(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
	})
	decay.attach(h.srv.State)
	if err := h.srv.Pets.Save(context.Background(), h.collarID, pet.State{
		Level: wolfLevel, Exp: wolfLevelExp, CurHP: 0, CurMP: 10, Fed: wolfMaxMeal,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	wolf, _ := h.spawnWolf(t)
	if !wolf.Dead() {
		t.Fatal("wolf saved dead restored alive")
	}

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	if _, ok := h.srv.State.Object(wolf.ObjectID()); ok {
		t.Fatal("restored dead wolf stayed in the world after its owner left, with no decay to remove it")
	}
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("restored dead wolf still holds its offline owner's summon slot")
	}
	decay.passAndTick(t, h.srv, 2*time.Hour)

	h.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelected, "CharSelected")
	h.client.Send(encodeEnterWorld())
	readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "end of the EnterWorld burst")
	drainUntilQuiet(t, h.client)

	again, _ := h.spawnWolf(t)
	if !again.Dead() {
		t.Fatal("wolf called out again came back alive, want it restored dead from its row")
	}
}

// TestOfflinePetDecaySparesACollarThatChangedHands has an owner come back to
// its dead pet, trade the pet's collar to another player, and log
// out again. When the corpse then decays with its owner offline, the collar
// is no longer the owner's to lose: the other player's collar row stays.
func TestOfflinePetDecaySparesACollarThatChangedHands(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
	})
	decay.attach(h.srv.State)
	wolf, _ := h.spawnWolf(t)
	alt, altID := joinSecondPlayer(t, h.srv)
	drainUntilQuiet(t, h.client)
	killPet(t, h, wolf)

	h.relogOwner(t)
	drainUntilQuiet(t, alt)
	h.client.Send(encodeTradeRequest(altID))
	readUntilOpcode(t, alt, serverpackets.OpcodeSendTradeRequest, "trade request")
	alt.Send(encodeAnswerTradeRequest(1))
	readUntilOpcode(t, h.client, serverpackets.OpcodeTradeStart, "owner TradeStart")
	readUntilOpcode(t, alt, serverpackets.OpcodeTradeStart, "alt TradeStart")
	h.client.Send(encodeAddTradeItem(0, h.collarID, 1))
	readUntilOpcode(t, alt, serverpackets.OpcodeTradeOtherAdd, "the collar offered")
	h.client.Send(encodeTradeDone(1))
	readUntilOpcode(t, alt, serverpackets.OpcodeTradePressOtherOk, "owner confirms")
	alt.Send(encodeTradeDone(1))
	h.srv.AdvanceUntil(t, "the collar changes hands", func() bool {
		return h.ownerInventory(t).ItemByObjectID(h.collarID) == nil
	})
	drainUntilQuiet(t, alt)
	drainUntilQuiet(t, h.client)
	h.srv.FlushItems(t)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	decay.passAndTick(t, h.srv, 1201*time.Second)
	if _, ok := h.srv.State.Object(wolf.ObjectID()); ok {
		t.Fatal("pet corpse still in the world after its decay")
	}
	h.srv.FlushPersistence(t)
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), altID)
	if err != nil {
		t.Fatalf("list alt items: %v", err)
	}
	for _, row := range rows {
		if row.ObjectID == h.collarID {
			return
		}
	}
	t.Fatal("the offline owner's pet decay deleted the collar row another player now owns")
}
