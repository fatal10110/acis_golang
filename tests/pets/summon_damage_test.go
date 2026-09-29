package pets

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// secondPlayer is another player in the world next to the pet owner.
type secondPlayer struct {
	client *testsupport.ScriptedClient
	id     int32
	actor  attackable.Combatant
	queue  *sim.Queue
}

// joinSecondPlayer brings a second character, name, into the world.
func (h *petWorld) joinSecondPlayer(t *testing.T, name string) secondPlayer {
	t.Helper()
	id := h.srv.SeedCharacterFor(t, "player2", name, 1, 0).ID
	c := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, c)
	obj, ok := h.srv.State.Player(id)
	if !ok {
		t.Fatalf("%s not in world", name)
	}
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, c)
	return secondPlayer{client: c, id: id, actor: obj.(attackable.Combatant), queue: h.srv.PlayerQueue(t, id)}
}

// runOn runs fn on q and waits for it, the way a hit lands on the pet from
// its attacker's queue.
func runOn(t *testing.T, q *sim.Queue, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !q.Post(func() { defer close(done); fn() }) {
		t.Fatal("post: queue closed")
	}
	<-done
}

// landEveryHit makes every auto-attack of player id hit: the swings these
// scenarios drive must land, and an unlucky run of misses would say
// nothing about the rules under test.
func landEveryHit(t *testing.T, srv *gameservertest.Server, id int32) {
	t.Helper()
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d not in world", id)
	}
	player := obj.(interface{ SetRollSource(func(int) int) })
	runOn(t, srv.PlayerQueue(t, id), func() { player.SetRollSource(func(int) int { return 0 }) })
}

// addPetEffect lands a debuff named name on pet from pet itself.
func addPetEffect(t *testing.T, pet *summon.Actor, name string) {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: name, Time: 30})
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	e.Effector, e.Effected = pet, pet
	runOn(t, pet.Queue(), func() { pet.EffectList().Add(e) })
	if !petHasEffect(pet, e.Type) {
		t.Fatalf("%s did not land on the pet", name)
	}
}

func petHasEffect(pet *summon.Actor, typ effect.Type) bool {
	for _, e := range pet.EffectList().All() {
		if e.Type == typ {
			return true
		}
	}
	return false
}

// TestHitWakesPetButDamageOverTimeAndHPCostDoNot puts the pet to sleep and
// immobilizes it until attacked. A damage-over-time tick and one of the
// pet's own HP costs leave both in place; the first direct hit ends both.
func TestHitWakesPetButDamageOverTimeAndHPCostDoNot(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	other := h.joinSecondPlayer(t, "Hitter")
	addPetEffect(t, pet, "Sleep")
	addPetEffect(t, pet, "ImmobileUntilAttacked")

	runOn(t, other.queue, func() { pet.ReduceHPByDOT(1, other.actor.(effect.Actor), true) })
	runOn(t, pet.Queue(), func() { pet.ConsumeHP(1) })
	for _, typ := range []effect.Type{effect.TypeSleep, effect.TypeImmobileUntilAttacked} {
		if !petHasEffect(pet, typ) {
			t.Fatalf("effect %v ended on damage over time or an HP cost, want it kept", typ)
		}
	}

	runOn(t, other.queue, func() { pet.ReduceHP(1, other.actor, modelskill.Definition{}) })
	for _, typ := range []effect.Type{effect.TypeSleep, effect.TypeImmobileUntilAttacked} {
		if petHasEffect(pet, typ) {
			t.Fatalf("effect %v survived a direct hit, want it ended", typ)
		}
	}
}

// TestHitBreaksPetStunOneTimeInTen stuns the pet. Damage-over-time ticks
// never break the stun; direct hits break it on a one-in-ten roll, so a run
// of hits breaks it well before 500 of them.
func TestHitBreaksPetStunOneTimeInTen(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	other := h.joinSecondPlayer(t, "Hitter")
	addPetEffect(t, pet, "Stun")

	for range 100 {
		runOn(t, other.queue, func() { pet.ReduceHPByDOT(0.01, other.actor.(effect.Actor), true) })
	}
	if !petHasEffect(pet, effect.TypeStun) {
		t.Fatal("stun broke on damage over time, want only direct hits to break it")
	}
	hits := 0
	for hits < 500 && petHasEffect(pet, effect.TypeStun) {
		runOn(t, other.queue, func() { pet.ReduceHP(0.01, other.actor, modelskill.Definition{}) })
		hits++
	}
	if petHasEffect(pet, effect.TypeStun) {
		t.Fatal("stun survived 500 direct hits, want a one-in-ten break")
	}
	if hits == 500 || pet.Dead() {
		t.Fatalf("stun broke after %d hits, dead %v", hits, pet.Dead())
	}
}

// TestInvulnerablePetStillReportsTheHitToItsOwner hits an invulnerable pet.
// It keeps its HP, and its owner still reads the damage message naming the
// attacker and the damage.
func TestInvulnerablePetStillReportsTheHitToItsOwner(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	other := h.joinSecondPlayer(t, "Hitter")
	pet.SetInvul(true)
	full := pet.HP()

	runOn(t, other.queue, func() { pet.ReduceHP(37, other.actor, modelskill.Definition{}) })
	if pet.HP() != full {
		t.Fatalf("invulnerable pet HP = %v, want %v", pet.HP(), full)
	}
	frame := findSystemMessage(t, drainFrames(t, h.client), serverpackets.SystemMessagePetReceivedS2DamageByS1)
	if frame == nil {
		t.Fatal("owner never read the pet damage message for a blocked hit")
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	if params, typ, name := r.ReadInt32(), r.ReadInt32(), r.ReadString(); params != 2 || typ != serverpackets.SystemMessageParamText || name != "Hitter" {
		t.Fatalf("damage message = %d params, type %d name %q, want 2 with attacker Hitter", params, typ, name)
	}
	if typ, amount := r.ReadInt32(), r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber || amount != 37 {
		t.Fatalf("damage message amount = type %d value %d, want number 37", typ, amount)
	}
}

// TestKilledPetDiesForObserversAndOwner kills the pet of an owner in combat
// stance with beast soulshots on auto. The killer sees the pet die and gains
// the summon-kill karma for a PK count of 0: 30, a quarter of a player
// kill's 120. The owner sees the pet die and the pet's stance end while its
// own stance stays, then loses the auto beast soulshot, reads the pet death
// message, and last the damage message of the killing hit.
func TestKilledPetDiesForObserversAndOwner(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: beastSoulshotID, Count: 10})
	pet, _ := h.spawnWolf(t)
	killer := h.joinSecondPlayer(t, "Killer")
	h.client.Send(encodeRequestAutoSoulShot(beastSoulshotID, 1))
	drainUntilQuiet(t, h.client)
	owner, _ := h.srv.State.Player(h.ownerID)
	h.srv.SetPlayerInCombat(t, h.ownerID, true)

	runOn(t, killer.queue, func() { pet.ReduceHP(pet.HP()+100, killer.actor, modelskill.Definition{}) })
	if !pet.Dead() {
		t.Fatal("pet alive after a lethal hit")
	}

	petID := pet.ObjectID()
	got := deathTags(t, drainFrames(t, h.client), petID, h.ownerID)
	want := []string{
		fmt.Sprintf("Die %d", petID),
		fmt.Sprintf("AutoAttackStop %d", petID),
		fmt.Sprintf("ExAutoSoulShot %d off", beastSoulshotID),
		fmt.Sprintf("SystemMessage %d", serverpackets.SystemMessageAutoUseOfItemCancelled),
		fmt.Sprintf("SystemMessage %d", serverpackets.SystemMessageResurrectPetWithin20Minutes),
		fmt.Sprintf("SystemMessage %d", serverpackets.SystemMessagePetReceivedS2DamageByS1),
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("owner death frames = %q, want %q", got, want)
	}
	h.srv.Settle(t)
	if !owner.(interface{ InCombat() bool }).InCombat() {
		t.Fatal("owner left combat when its pet died, want its stance kept")
	}

	if got := deathTags(t, drainFrames(t, killer.client), petID, killer.id); len(got) == 0 || got[0] != fmt.Sprintf("Die %d", petID) {
		t.Fatalf("killer death frames = %q, want the pet's Die first", got)
	}
	if karma := killer.actor.Karma(); karma != 30 {
		t.Fatalf("killer karma = %d, want 30 for a summon kill at PK count 0", karma)
	}

	fed := pet.Fed()
	runOn(t, pet.Queue(), func() { pet.TickPet(h.srv.State) })
	if pet.Fed() != fed {
		t.Fatalf("dead pet meal gauge = %d after a feed tick, want %d kept", pet.Fed(), fed)
	}
}

// TestSummonKillKarmaFollowsTheOwnersStanding kills the pet of a PvP-flagged
// owner: the killer gains no karma. A summon's karma and flag are its
// owner's.
func TestSummonKillKarmaFollowsTheOwnersStanding(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	killer := h.joinSecondPlayer(t, "Killer")
	owner, _ := h.srv.State.Player(h.ownerID)
	owner.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
	if pet.PvPFlagState() != task.PvPFlagOn {
		t.Fatalf("pet PvP flag = %v, want the owner's", pet.PvPFlagState())
	}

	runOn(t, killer.queue, func() { pet.ReduceHP(pet.HP()+100, killer.actor, modelskill.Definition{}) })
	if !pet.Dead() {
		t.Fatal("pet alive after a lethal hit")
	}
	if karma := killer.actor.Karma(); karma != 0 {
		t.Fatalf("killer karma = %d, want 0 for a flagged owner's summon", karma)
	}
}

// TestSummonPKKillEndsTheKillersFlag has a PvP-flagged player kill the pet
// of an unflagged, karma-free owner: the kill earns karma, so the killer's
// PvP flag task stops and its flag resets, which the owner sees as a
// RelationChanged without the flag.
func TestSummonPKKillEndsTheKillersFlag(t *testing.T) {
	t.Parallel()
	flags := task.NewPvPFlags(task.DefaultPvPFlagOptions(), nil)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithPvPFlags(flags)})
	pet, _ := h.spawnWolf(t)
	killer := h.joinSecondPlayer(t, "Killer")
	flagged := killer.actor.(interface {
		task.PvPFlagActor
		PvPFlagState() task.PvPFlagState
	})
	runOn(t, killer.queue, func() { flags.AddNormal(flagged) })
	drainUntilQuiet(t, h.client)

	runOn(t, killer.queue, func() { pet.ReduceHP(pet.HP()+100, killer.actor, modelskill.Definition{}) })
	if !pet.Dead() {
		t.Fatal("pet alive after a lethal hit")
	}
	h.srv.Settle(t)
	if karma := killer.actor.Karma(); karma <= 0 {
		t.Fatalf("killer karma = %d, want a PK gain for an innocent owner's summon", karma)
	}
	if state := flagged.PvPFlagState(); state != task.PvPFlagNone {
		t.Fatalf("killer PvP flag = %v after the summon PK kill, want none", state)
	}
	if n := flags.Len(); n != 0 {
		t.Fatalf("PvP flag task tracks %d players after the summon PK kill, want none", n)
	}
	var last []byte
	for _, f := range drainFrames(t, h.client) {
		if f[0] == serverpackets.OpcodeRelationChanged && wire.NewReader(f[1:]).ReadInt32() == killer.id {
			last = f
		}
	}
	if last == nil {
		t.Fatal("owner never received the killer's RelationChanged")
	}
	r := wire.NewReader(last[1:])
	r.ReadInt32()
	relation, _, karma := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if pvpFlag := r.ReadInt32(); relation&serverpackets.RelationPvPFlag != 0 || pvpFlag != 0 || karma <= 0 {
		t.Fatalf("owner's last RelationChanged for the killer = relation %#x karma %d flag %d, want karma and no flag", relation, karma, pvpFlag)
	}
}

// TestOtherPlayerNeedsForceToAttackAPet has a second player click the pet
// of an unflagged owner: a plain second click only releases the action,
// while an attack request on the selected pet attacks it, flags the
// attacker and lands damage.
func TestOtherPlayerNeedsForceToAttackAPet(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithPvPFlags(task.NewPvPFlags(task.DefaultPvPFlagOptions(), nil)),
	})
	pet, _ := h.spawnWolf(t)
	other := h.joinSecondPlayer(t, "Hitter")
	landEveryHit(t, h.srv, other.id)
	x, y, z := pet.Position()

	other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	selected := false
	for _, frame := range drainFrames(t, other.client) {
		if frame[0] != serverpackets.OpcodeMyTargetSelected {
			continue
		}
		r := wire.NewReader(frame[1:])
		if id, color := r.ReadInt32(), int16(r.ReadUint16()); id != pet.ObjectID() || int(color) != 1-wolfLevel {
			t.Fatalf("MyTargetSelected = %d color %d, want pet %d color %d", id, color, pet.ObjectID(), 1-wolfLevel)
		}
		selected = true
	}
	if !selected {
		t.Fatal("first click on the pet sent no MyTargetSelected")
	}

	other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	frames := drainFrames(t, other.client)
	if countOpcode(frames, serverpackets.OpcodeActionFailed) != 1 || countOpcode(frames, serverpackets.OpcodeAttack) != 0 {
		t.Fatalf("plain second click on an unflagged owner's pet = %x, want ActionFailed and no attack", frameOpcodes(frames))
	}

	other.client.Send(encodeAttackRequest(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	full := pet.HP()
	h.srv.AdvanceUntil(t, "forced attack landing on the pet", func() bool { return pet.HP() < full })
	player, _ := h.srv.State.Player(other.id)
	if flag := player.(interface{ PvPFlagState() task.PvPFlagState }).PvPFlagState(); flag == task.PvPFlagNone {
		t.Fatal("attacking a pet left the attacker unflagged")
	}
}

// TestForcedAttackWakesASleepingPet puts the pet to sleep; another player's
// forced attack lands on it and wakes it.
func TestForcedAttackWakesASleepingPet(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	other := h.joinSecondPlayer(t, "Hitter")
	landEveryHit(t, h.srv, other.id)
	addPetEffect(t, pet, "Sleep")
	x, y, z := pet.Position()

	other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, other.client)
	full := pet.HP()
	other.client.Send(encodeAttackRequest(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	h.srv.AdvanceUntil(t, "forced attack landing on the sleeping pet", func() bool { return pet.HP() < full })
	if petHasEffect(pet, effect.TypeSleep) {
		t.Fatal("pet still asleep after a landed attack")
	}
}

// TestFlaggedOwnersPetIsAttackedWithoutForce flags the pet's owner: another
// player's plain second click on the pet attacks it.
func TestFlaggedOwnersPetIsAttackedWithoutForce(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	other := h.joinSecondPlayer(t, "Hitter")
	landEveryHit(t, h.srv, other.id)
	owner, _ := h.srv.State.Player(h.ownerID)
	owner.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
	drainUntilQuiet(t, other.client)
	x, y, z := pet.Position()

	other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, other.client)
	full := pet.HP()
	other.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	h.srv.AdvanceUntil(t, "plain attack landing on a flagged owner's pet", func() bool { return pet.HP() < full })
}

// TestOwnerSecondClickOnItsPetOpensTheStatusWindow keeps the owner's own
// pet out of its plain clicks: the second click shows the status window
// and never attacks.
func TestOwnerSecondClickOnItsPetOpensTheStatusWindow(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	x, y, z := pet.Position()
	h.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	frames := drainFrames(t, h.client)
	if countOpcode(frames, serverpackets.OpcodePetStatusShow) != 1 || countOpcode(frames, serverpackets.OpcodeAttack) != 0 {
		t.Fatalf("owner second click on its pet = %x, want PetStatusShow and no attack", frameOpcodes(frames))
	}
}

// TestSummonKillInsidePvPZoneAwardsNoKarma kills an unflagged owner's pet
// with the killer and the owner both inside an arena: a kill inside a PvP
// zone never earns karma.
func TestSummonKillInsidePvPZoneAwardsNoKarma(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-100_000, 100_000, -100_000, 100_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("arena form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewArena(1, form))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithZones(zones)})
	pet, _ := h.spawnWolf(t)
	killer := h.joinSecondPlayer(t, "Killer")
	owner, _ := h.srv.State.Player(h.ownerID)
	for name, p := range map[string]any{"owner": owner, "killer": killer.actor} {
		if !p.(interface{ InPvPZone() bool }).InPvPZone() {
			t.Fatalf("%s is not inside the arena", name)
		}
	}

	runOn(t, killer.queue, func() { pet.ReduceHP(pet.HP()+100, killer.actor, modelskill.Definition{}) })
	if !pet.Dead() {
		t.Fatal("pet alive after a lethal hit")
	}
	if karma := killer.actor.Karma(); karma != 0 {
		t.Fatalf("killer karma = %d, want 0 for a summon kill inside a PvP zone", karma)
	}
}

// TestOwnerForcedAttackHitsItsOwnPet has the owner select its own pet and
// send an attack request on it: forcing, the owner attacks its own pet
// instead of opening its status window.
func TestOwnerForcedAttackHitsItsOwnPet(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	landEveryHit(t, h.srv, h.ownerID)
	x, y, z := pet.Position()
	h.client.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, h.client)

	full := pet.HP()
	h.client.Send(encodeAttackRequest(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	h.srv.AdvanceUntil(t, "owner's forced attack landing on its pet", func() bool { return pet.HP() < full })
	if frames := drainFrames(t, h.client); countOpcode(frames, serverpackets.OpcodePetStatusShow) != 0 {
		t.Fatal("owner's forced attack opened the pet status window")
	}
}

// TestPetNpcInfoMarksItAttackableForOtherViewers checks the attackable flag
// of the NpcInfo another player gets for a pet: set while the pet's owner is
// PvP-flagged, both when the pet appears and when its status is republished,
// and cleared once the owner's flag is gone.
func TestPetNpcInfoMarksItAttackableForOtherViewers(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	other := h.joinSecondPlayer(t, "Viewer")
	owner, _ := h.srv.State.Player(h.ownerID)
	flagged := owner.(interface{ UpdatePvPFlag(task.PvPFlagState) })
	flagged.UpdatePvPFlag(task.PvPFlagOn)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, other.client)

	pet, _ := h.spawnWolf(t)
	if got, ok := petNpcInfoAttackable(drainFrames(t, other.client), pet.ObjectID()); !ok || !got {
		t.Fatalf("NpcInfo on a flagged owner's pet appearing: seen %v attackable %v, want attackable", ok, got)
	}

	flagged.UpdatePvPFlag(task.PvPFlagNone)
	drainUntilQuiet(t, other.client)
	runOn(t, pet.Queue(), pet.UpdateStatus)
	if got, ok := petNpcInfoAttackable(drainFrames(t, other.client), pet.ObjectID()); !ok || got {
		t.Fatalf("NpcInfo on an unflagged owner's pet: seen %v attackable %v, want not attackable", ok, got)
	}
}

// petNpcInfoAttackable reads the attackable flag of the last NpcInfo for
// petID among frames.
func petNpcInfoAttackable(frames [][]byte, petID int32) (attackable, seen bool) {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() != petID {
			continue
		}
		r.ReadInt32() // template id
		attackable, seen = r.ReadInt32() == 1, true
	}
	return attackable, seen
}

func encodeAttackRequest(objectID int32, x, y, z int32, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

// deathTags names the death-sequence frames among frames, in order: Die,
// AutoAttackStop for the pet or self, ExAutoSoulShot, and system messages.
func deathTags(t *testing.T, frames [][]byte, petID, selfID int32) []string {
	t.Helper()
	var tags []string
	for _, frame := range frames {
		r := wire.NewReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeDie:
			if id := r.ReadInt32(); id == petID {
				tags = append(tags, fmt.Sprintf("Die %d", id))
			}
		case serverpackets.OpcodeAutoAttackStop:
			if id := r.ReadInt32(); id == petID || id == selfID {
				tags = append(tags, fmt.Sprintf("AutoAttackStop %d", id))
			}
		case serverpackets.OpcodeExtended:
			if r.ReadUint16() == serverpackets.OpcodeExAutoSoulShot {
				itemID, on := r.ReadInt32(), r.ReadInt32()
				state := "on"
				if on == 0 {
					state = "off"
				}
				tags = append(tags, fmt.Sprintf("ExAutoSoulShot %d %s", itemID, state))
			}
		case serverpackets.OpcodeSystemMessage:
			tags = append(tags, fmt.Sprintf("SystemMessage %d", r.ReadInt32()))
		}
	}
	return tags
}
