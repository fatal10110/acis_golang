package pets

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: Player.deleteMe (Player.java:6281-6292) calls Summon.unSummon,
// which returns early for a dead summon (Summon.java:550-556): the servitor
// corpse stays in the world with the offline Player as its owner, and that
// Player still names it as its summon. Resurrect.useSkill
// (Resurrect.java:23-52) revives it outright whether or not its owner is
// online: a player caster through doRevive, which leaves its decay pending,
// a non-player caster after DecayTaskManager.cancel. Summon.onDecay
// (Summon.java:193-199) then removes it at its deadline, since the offline
// owner still names it. Servitor.doDie (Servitor.java:109-128) stopped its
// lifetime task and nothing restarts it. The owner's next session is another
// Player that never learns of it. A hit on it enters its offline owner's
// attack stance (SummonAI.startAttackStance, SummonAI.java:235-244; the
// logout removed that owner from AttackStanceTaskManager): AutoAttackStart
// for the servitor, and AttackStanceTaskManager.run (:52-80) broadcasts its
// AutoAttackStop once the stance runs out. aCis revision in the outer repo.

// leftServitor is an owner at character select whose dead servitor stayed
// in the world, and a healer beside it who knows the power-100
// resurrection.
type leftServitor struct {
	*servitorOwner
	servitor *summon.Actor
	decay    *corpseDecay
	healer   *testsupport.ScriptedClient
	healerID int32
	// stanceMS is the attack-stance clock, in Unix milliseconds.
	stanceMS *atomic.Int64
}

// leaveServitorDead boots the owner and a healer, summons the servitor,
// kills it and logs the owner out to character select.
func leaveServitorDead(t *testing.T) *leftServitor {
	t.Helper()
	decay := newCorpseDecay(t)
	stanceMS := &atomic.Int64{}
	o := bootServitorOwnerWithSkills(t, []modelskill.Definition{resurrectionSkill()},
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(stanceMS.Load()) }),
	)
	decay.attach(o.srv.State)
	healer := o.srv.SeedCharacterFor(t, "healer", "Healer", 5, 0)
	if err := o.srv.KnownSkills.SetKnownSkill(context.Background(), healer.ID, 0, petResurrectSkillID, 1); err != nil {
		t.Fatalf("seed resurrection: %v", err)
	}
	healerClient := o.srv.DialClient(t, "healer", 1)
	startInWorld(t, healerClient)
	drainUntilQuiet(t, o.client)

	servitor := o.summonServitor(t)
	drainUntilQuiet(t, healerClient)
	o.killServitor(t, servitor)
	o.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, o.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	if !servitor.OwnerLeft() {
		t.Fatal("dead servitor does not answer to the session that left it")
	}
	if _, ok := o.srv.State.Summon(o.id); ok {
		t.Fatal("servitor corpse kept its departed owner's summon slot")
	}
	drainUntilQuiet(t, healerClient)
	return &leftServitor{servitorOwner: o, servitor: servitor, decay: decay, healer: healerClient, healerID: healer.ID, stanceMS: stanceMS}
}

// relogIn brings the owner, at character select, back into the world and
// returns every frame of its EnterWorld burst.
func (l *leftServitor) relogIn(t *testing.T) [][]byte {
	t.Helper()
	l.client.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, l.client, serverpackets.OpcodeCharSelected, "CharSelected")
	l.client.Send(encodeEnterWorld())
	frames := readUntilOpcode(t, l.client, serverpackets.OpcodeActionFailed, "end of the EnterWorld burst")
	return append(frames, drainFrames(t, l.client)...)
}

// assertStandsUp checks the servitor came back at 70% HP, still nobody's.
func (l *leftServitor) assertStandsUp(t *testing.T) {
	t.Helper()
	if l.servitor.Dead() {
		t.Fatal("resurrection left the departed owner's servitor dead")
	}
	// Kat the Cat's live max HP is 410 whole points, at the 0.7 respawn
	// restore share.
	if got := l.servitor.HP(); got != 287 {
		t.Fatalf("revived servitor HP = %v, want 287", got)
	}
	if !l.servitor.OwnerLeft() {
		t.Fatal("revive handed the servitor to an owner who is not in the world")
	}
	if _, ok := l.srv.State.Summon(l.id); ok {
		t.Fatal("revived servitor took its departed owner's summon slot back")
	}
}

// sawPetDelete reports whether frames carry a PetDelete for objectID.
func sawPetDelete(frames [][]byte, objectID int32) bool {
	for _, f := range frames {
		if len(f) >= 9 && f[0] == serverpackets.OpcodePetDelete {
			r := wire.NewReader(f[1:])
			r.ReadInt32()
			if r.ReadInt32() == objectID {
				return true
			}
		}
	}
	return false
}

// TestPlayerResurrectionRevivesServitorLeftBehind: another player's
// resurrection on a servitor corpse whose owner is at character select
// stands it up at 70% HP for the caster to see. It keeps its decay and stays
// nobody's: the owner's next session gets no PetInfo for it and summons a
// new servitor at once, and at its corpse time it leaves the world, alive,
// with no PetDelete for the owner and the new servitor's slot untouched.
func TestPlayerResurrectionRevivesServitorLeftBehind(t *testing.T) {
	t.Parallel()
	l := leaveServitorDead(t)
	servitor := l.servitor

	castResurrection(t, l.srv, l.healer, l.healerID, servitor.ObjectID())
	if frameIndex(drainFrames(t, l.healer), serverpackets.OpcodeRevive, servitor.ObjectID()) < 0 {
		t.Fatal("caster never saw the servitor revive")
	}
	l.assertStandsUp(t)

	if sawServitorPetInfo(l.relogIn(t), servitor.ObjectID()) {
		t.Fatal("returning owner got a PetInfo for the servitor it left behind")
	}
	next := l.summonServitor(t)
	if next.ObjectID() == servitor.ObjectID() {
		t.Fatal("returning owner's summon answered with the servitor it left behind")
	}

	l.decay.passAndTick(t, l.srv, (servitorCorpseTime+1)*time.Second)
	if _, ok := l.srv.State.Object(servitor.ObjectID()); ok {
		t.Fatal("revived servitor still in the world past its corpse time")
	}
	if servitor.Dead() {
		t.Fatal("revived servitor died on its way out, want it leaving alive")
	}
	if sawPetDelete(drainFrames(t, l.client), servitor.ObjectID()) {
		t.Fatal("owner's new session got a PetDelete for the servitor it left behind")
	}
	if got, ok := l.srv.State.Summon(l.id); !ok || got.ObjectID() != next.ObjectID() {
		t.Fatal("the revived servitor's departure took the owner's new servitor's slot")
	}
}

// TestNonPlayerResurrectionRevivesServitorLeftBehind: a monster's
// resurrection on a servitor corpse whose owner is at character select
// cancels its decay and stands it up: it is still in the world past its
// corpse time, running on the queue of its own it got at the logout. It stays
// nobody's. A monster's hit puts it in an attack stance of its own, seen by
// the owner's next session as AutoAttackStart for the servitor alone, with no
// damage message and no stance for the owner; the stance runs out into an
// AutoAttackStop for it. Killed again, it tells the owner nothing and decays
// at its corpse time, out of any stance.
func TestNonPlayerResurrectionRevivesServitorLeftBehind(t *testing.T) {
	t.Parallel()
	l := leaveServitorDead(t)
	servitor := l.servitor
	corpseQueue := servitor.Queue()

	npcResurrect(t, l.srv, servitor)
	l.assertStandsUp(t)
	l.decay.passAndTick(t, l.srv, (servitorCorpseTime+1)*time.Second)
	if _, ok := l.srv.State.Object(servitor.ObjectID()); !ok {
		t.Fatal("servitor a monster revived left the world at its old corpse deadline")
	}
	if servitor.Queue() != corpseQueue || servitor.EffectList().Queue() != corpseQueue {
		t.Fatal("revived servitor and its effects left the queue it got at the logout")
	}

	if sawServitorPetInfo(l.relogIn(t), servitor.ObjectID()) {
		t.Fatal("returning owner got a PetInfo for the servitor it left behind")
	}
	id := servitor.ObjectID()
	x, y, z := servitor.Position()
	monster := l.srv.SpawnAttackingHostileNPCAt(t, location.Location{X: x + 20, Y: y, Z: z})
	drainUntilQuiet(t, l.client)
	monster.DoAttack(t, servitor)
	if servitor.HP() >= 287 {
		t.Fatal("monster's hit did not land on the revived servitor")
	}
	frames := drainFrames(t, l.client)
	if n := frameCount(frames, serverpackets.OpcodeAutoAttackStart, id); n != 1 {
		t.Fatalf("owner saw %d AutoAttackStart for the hit servitor, want 1", n)
	}
	if frameIndex(frames, serverpackets.OpcodeAutoAttackStart, l.id) >= 0 {
		t.Fatal("a hit on the servitor it left behind put the owner's new session in stance")
	}
	if hasSystemMessage(frames, serverpackets.SystemMessageSummonReceivedS2ByS1) {
		t.Fatal("owner's new session read the damage the servitor it left behind took")
	}
	if l.srv.AttackStance.InAttackStance(ownerKey{id: l.id}) {
		t.Fatal("owner's new session in the stance tracker after a hit on the servitor it left behind")
	}
	if !l.srv.AttackStance.InAttackStance(ownerKey{id: id}) {
		t.Fatal("hit servitor not in the stance tracker")
	}

	l.stanceMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := l.srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	l.srv.Settle(t)
	readUntilOpcode(t, l.client, serverpackets.OpcodeAutoAttackStop, "servitor AutoAttackStop")
	if l.srv.AttackStance.InAttackStance(ownerKey{id: id}) {
		t.Fatal("servitor still in the stance tracker after it ran out")
	}

	monster.DoAttack(t, servitor)
	if !l.srv.AttackStance.InAttackStance(ownerKey{id: id}) {
		t.Fatal("second hit did not put the servitor back in stance")
	}
	runOn(t, monster.Queue(), func() {
		servitor.ReduceHP(servitor.HP()+100, attackable.Combatant(monster.Hostile), modelskill.Definition{})
	})
	if !servitor.Dead() {
		t.Fatal("servitor alive after a lethal hit")
	}
	frames = drainFrames(t, l.client)
	if hasSystemMessage(frames, serverpackets.SystemMessageServitorPassedAway) {
		t.Fatal("owner's new session read the death of the servitor it left behind")
	}
	l.decay.passAndTick(t, l.srv, (servitorCorpseTime+1)*time.Second)
	if _, ok := l.srv.State.Object(id); ok {
		t.Fatal("killed servitor still in the world past its corpse time")
	}
	if l.srv.AttackStance.InAttackStance(ownerKey{id: id}) {
		t.Fatal("decayed servitor still in the stance tracker")
	}
	if sawPetDelete(drainFrames(t, l.client), id) {
		t.Fatal("owner's new session got a PetDelete for the servitor it left behind")
	}
}
