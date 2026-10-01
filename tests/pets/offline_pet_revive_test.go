package pets

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: Resurrect.useSkill's non-player branch (Resurrect.java:43-51)
// cancels the target's decay and calls doRevive(power) on any dead
// Creature, a Pet whose owner is offline included. Pet.doRevive(double)
// (Pet.java:289-294) restores the lost exp, then Pet.doRevive
// (Pet.java:269-286) clears the offline owner's revive request, stands the
// pet up, cancels its decay and sends it idle: SummonAI.thinkIdle
// (SummonAI.java:31-45) follows the owner, whom friendlyFollowTask
// (CreatureMove.java:586-610) never reaches, since the pet does not know an
// owner who is out of the world. The pet stays in World._pets, and
// Player.restore (Player.java:4145-4151) relinks it, alive, to the owner's
// next session. EffectSignetAntiSummon (EffectSignetAntiSummon.java:47-55)
// unsummons such a living pet: Pet.unSummon (Pet.java:340-356) hands its
// items to the owner and Summon.doUnsummon stores its row. aCis revision in
// the outer repo.

// npcResurrectID is the resurrection a monster casts in these scenarios.
const npcResurrectID = 9700

// offlinePetWorld is an owner at character select whose dead wolf, carrying
// 40 adena, stayed in the world, with a monster beside it that can cast a
// power-100 resurrection and a second player watching.
type offlinePetWorld struct {
	*petWorld
	wolf     *summon.Actor
	decay    *corpseDecay
	observer *testsupport.ScriptedClient
}

// leaveWolfDead boots the owner with a level-10 wolf at 510 exp and 40
// adena on it, kills the wolf (which drops it to level 9 at 481 exp) and
// logs the owner out to character select.
func leaveWolfDead(t *testing.T) *offlinePetWorld {
	t.Helper()
	decay := newCorpseDecay(t)
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{penaltyWolfTemplate(), treeTemplate()})),
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
	)
	decay.attach(srv.State)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	if err := srv.Pets.Save(petCtx(), collarID, pet.State{
		Level: wolfLevel, Exp: 510, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: wolfMaxMeal,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	adenaID := srv.GiveItem(t, ownerID, item.AdenaID, 40)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)

	wolf, _ := h.spawnWolf(t)
	h.giveToPet(t, adenaID, 40)
	observer, _ := joinSecondPlayer(t, srv)
	drainUntilQuiet(t, h.client)
	killPet(t, h, wolf)
	drainUntilQuiet(t, observer)
	if wolf.Exp() != 481 || wolf.Level() != wolfLevel-1 {
		t.Fatalf("dead wolf exp %d level %d, want 481 at level %d", wolf.Exp(), wolf.Level(), wolfLevel-1)
	}

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	if !wolf.OwnerLeft() {
		t.Fatal("dead wolf does not answer to the session that left it")
	}
	drainUntilQuiet(t, observer)
	return &offlinePetWorld{petWorld: h, wolf: wolf, decay: decay, observer: observer}
}

// npcResurrects has a monster beside the wolf cast its resurrection on the
// wolf and lets the hit land.
func (o *offlinePetWorld) npcResurrects(t *testing.T) {
	t.Helper()
	drainUntilQuiet(t, o.observer)
	npcResurrect(t, o.srv, o.wolf)
}

// npcResurrect spawns a monster that can cast a power-100 resurrection
// beside the owner's spot, has it cast the resurrection on target and lets
// the hit land.
func npcResurrect(t *testing.T, srv *gameservertest.Server, target *summon.Actor) {
	t.Helper()
	ref := modelskill.Ref{ID: npcResurrectID, Level: 1}
	hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{{
		ID: npcResurrectID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT",
		CastRange: 400, HitTime: 500, StaticHitTime: true, Power: 100,
	}}))
	runOn(t, hostile.Queue(), func() { aiCtl.Cast(target, ref) })
	srv.Settle(t)
	srv.Advance(t, 600*time.Millisecond)
	srv.Settle(t)
}

// TestNonPlayerResurrectionRevivesOfflineOwnersPet: a monster's resurrection
// revives a wolf corpse whose owner is at character select. The wolf gets
// its 29 lost exp back, stands up at 70% HP for everyone watching, and drops
// its decay: it is still in the world, alive, past its 20 minutes. It keeps
// its adena, runs its effects on a queue of its own, and still answers to
// the session that left it. The owner's next login finds it relinked: the
// new session sees it as its pet, its effects run on the new session's
// queue, the collar shows the level the wolf regained, and returning it
// hands its adena to the owner and saves it alive with its exp.
func TestNonPlayerResurrectionRevivesOfflineOwnersPet(t *testing.T) {
	t.Parallel()
	o := leaveWolfDead(t)
	wolf := o.wolf
	offlineQueue := wolf.Queue()

	o.npcResurrects(t)
	if wolf.Dead() {
		t.Fatal("monster's resurrection left the offline owner's pet dead")
	}
	if got := wolf.Exp(); got != 510 {
		t.Fatalf("revived wolf exp = %d, want 510", got)
	}
	if got := wolf.Level(); got != wolfLevel {
		t.Fatalf("revived wolf level = %d, want %d", got, wolfLevel)
	}
	if got := wolf.HP(); got != revivedWolfHP {
		t.Fatalf("revived wolf HP = %v, want %v", got, revivedWolfHP)
	}
	if frameIndex(drainFrames(t, o.observer), serverpackets.OpcodeRevive, wolf.ObjectID()) < 0 {
		t.Fatal("observer never saw the offline owner's pet revive")
	}
	if !wolf.OwnerLeft() {
		t.Fatal("revive handed the pet to an owner who is not in the world")
	}
	if got := petItemCount(wolf, item.AdenaID); got != 40 {
		t.Fatalf("revived wolf carries %d adena, want 40", got)
	}

	o.decay.passAndTick(t, o.srv, 1201*time.Second)
	if obj, ok := o.srv.State.Summon(o.ownerID); !ok || obj.ObjectID() != wolf.ObjectID() {
		t.Fatal("offline owner's revived pet decayed at its old corpse deadline")
	}
	if wolf.Queue() != offlineQueue || wolf.EffectList().Queue() != offlineQueue {
		t.Fatal("offline owner's revived pet and its effects run on different queues")
	}
	buff := landTimedBuff(t, wolf)
	o.srv.Advance(t, 2200*time.Millisecond)
	o.srv.TickEffects()
	if slices.Contains(wolf.EffectList().All(), buff) {
		t.Fatal("offline owner's revived pet kept its two-second buff past its duration")
	}

	frames := o.relogIn(t)
	if !sawPetInfo(frames, wolf.ObjectID()) {
		t.Fatal("returning owner got no PetInfo for its living pet")
	}
	if wolf.OwnerLeft() || wolf.Dead() {
		t.Fatalf("returning owner's pet: owner left %v, dead %v; want a living pet of the new session", wolf.OwnerLeft(), wolf.Dead())
	}
	session := o.srv.PlayerQueue(t, o.ownerID)
	if wolf.Queue() != session || wolf.EffectList().Queue() != session {
		t.Fatal("relinked living pet does not run on the new session's queue")
	}
	if got := o.liveCollarEnchant(t); got != wolfLevel {
		t.Fatalf("collar enchant after the relink = %d, want the regained level %d", got, wolfLevel)
	}
	buff = landTimedBuff(t, wolf)
	o.srv.Advance(t, 2200*time.Millisecond)
	o.srv.TickEffects()
	if slices.Contains(wolf.EffectList().All(), buff) {
		t.Fatal("relinked living pet kept its two-second buff past its duration")
	}

	o.returnPet(t)
	if got := o.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("owner adena rows after the return = %d, want 40", got)
	}
	state := o.savedPetState(t)
	if state.CurHP <= 0 || state.Exp != 510 {
		t.Fatalf("returned pet saved with HP %v exp %d, want alive with 510 exp", state.CurHP, state.Exp)
	}
}

// TestOfflineOwnersRevivedPetUnsummoned: a living pet revived while its
// owner is away and then unsummoned (as a signet does) leaves the world and
// the owner's summon slot. Its row is saved alive with its regained exp,
// and its adena goes to the offline owner's inventory rows, so the owner's
// next login starts with no pet and the adena in hand.
func TestOfflineOwnersRevivedPetUnsummoned(t *testing.T) {
	t.Parallel()
	o := leaveWolfDead(t)
	wolf := o.wolf
	o.npcResurrects(t)
	if wolf.Dead() {
		t.Fatal("monster's resurrection left the offline owner's pet dead")
	}

	runOn(t, wolf.Queue(), wolf.Unsummon)
	if _, ok := o.srv.State.Object(wolf.ObjectID()); ok {
		t.Fatal("unsummoned pet stayed in the world")
	}
	if _, ok := o.srv.State.Summon(o.ownerID); ok {
		t.Fatal("unsummoned pet kept its offline owner's summon slot")
	}
	state := o.savedPetState(t)
	if state.CurHP <= 0 || state.Exp != 510 {
		t.Fatalf("unsummoned pet saved with HP %v exp %d, want alive with 510 exp", state.CurHP, state.Exp)
	}
	if got := o.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("offline owner adena rows = %d, want the pet's 40", got)
	}
	if got := o.collarItemCount(t, item.AdenaID); got != 0 {
		t.Fatalf("adena still saved under the collar = %d, want none", got)
	}

	frames := o.relogIn(t)
	if sawPetInfo(frames, wolf.ObjectID()) {
		t.Fatal("returning owner got PetInfo for a pet that left the world")
	}
	if inst := o.ownerInventory(t).ItemByTemplateID(item.AdenaID); inst == nil || inst.Snapshot().Count != 40 {
		t.Fatal("returning owner does not hold its pet's 40 adena")
	}
	if got := o.liveCollarEnchant(t); got != wolfLevel {
		t.Fatalf("collar enchant after the relog = %d, want the level %d the pet was saved at", got, wolfLevel)
	}
}

// TestOfflineOwnersRevivedPetFights: a pet revived while its owner is away
// runs, chases and swings on the queue of its own it got at the logout, not
// on the departed session's closed one. A monster's aggression provokes it
// (fireAggressionEvent calls AttackTarget), and it runs to a monster out of
// its reach and lands a hit on it.
func TestOfflineOwnersRevivedPetFights(t *testing.T) {
	t.Parallel()
	o := leaveWolfDead(t)
	wolf := o.wolf
	offlineQueue := wolf.Queue()
	o.npcResurrects(t)
	if wolf.Dead() {
		t.Fatal("monster's resurrection left the offline owner's pet dead")
	}
	if wolf.Move().Queue() != offlineQueue {
		t.Fatal("offline owner's revived pet moves on another queue than its own")
	}

	x, y, z := wolf.Position()
	target := o.srv.SpawnHostileNPCAt(t, location.Location{X: x + 300, Y: y, Z: z})
	runOn(t, offlineQueue, func() {
		wolf.SetRollSource(landNoCrit())
		wolf.AttackTarget(target)
	})
	o.srv.AdvanceUntil(t, "offline owner's revived pet reaching and hitting the monster", func() bool {
		return target.CurrentHP() < target.MaxHP()
	})
	if nx, _, _ := wolf.Position(); nx <= x {
		t.Fatalf("pet hit a monster 300 away from x %d without moving (x %d)", x, nx)
	}
}

// TestRelinkedBabyPetKeepsHealing: a baby pet revived by a monster while
// its owner is away heals the owner once the owner is back: its owner-heal
// carries on on the new session's queue.
func TestRelinkedBabyPetKeepsHealing(t *testing.T) {
	t.Parallel()
	s := bootBabyPet(t, 0, gameservertest.WithReuseDelays(0, 0))
	killPet(t, s.petWorld, s.baby)
	s.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, s.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")

	npcResurrect(t, s.srv, s.baby)
	if s.baby.Dead() {
		t.Fatal("monster's resurrection left the offline owner's baby pet dead")
	}
	s.relogIn(t)
	if s.baby.OwnerLeft() {
		t.Fatal("returning owner's baby pet still answers to the session that left it")
	}
	s.woundOwner(t, 0.5)
	heals, _ := s.healsUntil(t, s.baby.Now().Add(1500*time.Millisecond))
	if len(heals) == 0 || heals[0].skill != babyWeakHeal {
		t.Fatalf("heals %+v for the returning owner below 80%%, want Heal Trick within a tick", heals)
	}
}

// relogIn brings the owner, at character select, back into the world and
// returns every frame of its EnterWorld burst.
func (h *petWorld) relogIn(t *testing.T) [][]byte {
	t.Helper()
	h.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelected, "CharSelected")
	h.client.Send(encodeEnterWorld())
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "end of the EnterWorld burst")
	return append(frames, drainFrames(t, h.client)...)
}

// landTimedBuff lands a two-second buff on pet from its own queue and
// returns it.
func landTimedBuff(t *testing.T, p *summon.Actor) *effect.Effect {
	t.Helper()
	buff, err := effect.New(effect.Skill{ID: 1040, Level: 1}, modelskill.EffectTemplate{Name: "Buff", Time: 2, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(Buff): %v", err)
	}
	buff.Effector, buff.Effected = p, p
	runOn(t, p.Queue(), func() { p.EffectList().Add(buff) })
	if !slices.Contains(p.EffectList().All(), buff) {
		t.Fatal("buff did not land on the pet")
	}
	return buff
}

// TestOfflineOwnersRevivedPetOutlastsALoadingScreenDrop: an owner whose pet
// was revived while it was away selects its character and loses the
// connection before EnterWorld. The session never took the pet over, so its
// departure leaves the pet in the world, alive and still answering to the
// departed session, and the next login takes it over. Relinking at the
// selection, which would make this drop unsummon the living pet, is #3209.
func TestOfflineOwnersRevivedPetOutlastsALoadingScreenDrop(t *testing.T) {
	t.Parallel()
	o := leaveWolfDead(t)
	wolf := o.wolf
	o.npcResurrects(t)
	if wolf.Dead() {
		t.Fatal("monster's resurrection left the offline owner's pet dead")
	}

	o.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, o.client, serverpackets.OpcodeCharSelected, "CharSelected")
	if err := o.client.Close(); err != nil {
		t.Fatalf("close the selecting client: %v", err)
	}
	o.srv.AdvanceUntil(t, "selected owner out of the world", func() bool {
		_, ok := o.srv.State.Player(o.ownerID)
		return !ok
	})
	o.srv.Settle(t)
	if obj, ok := o.srv.State.Summon(o.ownerID); !ok || obj.ObjectID() != wolf.ObjectID() {
		t.Fatal("a session dropped before EnterWorld took the revived pet out of its owner's summon slot")
	}
	if _, ok := o.srv.State.Object(wolf.ObjectID()); !ok || wolf.Dead() || !wolf.OwnerLeft() {
		t.Fatalf("revived pet after the drop: in world %v, dead %v, owner left %v; want it alive, still left behind", ok, wolf.Dead(), wolf.OwnerLeft())
	}

	o.client = o.srv.DialClient(t, o.srv.Account(), 1)
	frames := o.relogIn(t)
	if !sawPetInfo(frames, wolf.ObjectID()) || wolf.OwnerLeft() {
		t.Fatal("the next login did not take the revived pet over")
	}
}
