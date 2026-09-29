package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: Pet.doRevive / doRevive(double) (Pet.java:269-294),
// Pet.restoreExp and deathPenalty (Pet.java:584-611), Playable.doRevive
// (Playable.java:189-207), Resurrect.useSkill (Resurrect.java:23-51),
// Player.reviveAnswer (Player.java:6058-6084), Servitor.doDie
// (Servitor.java:109-128) and Summon.onDecay (Summon.java:193-199), aCis
// revision in the outer repo.
//
// Expected values are worked by hand from those methods: a revive without a
// Phoenix Blessing sets HP to int max HP * RESPAWN_RESTORE_HP (the test boot
// uses 0.7), and a resurrection gives back Math.round((expBeforeDeath - exp)
// * power / 100) of a pet's experience.

// wolfLiveMaxHP is the wolf's max HP in whole points: its 400 base HP
// raised by its CON 43 bonus. revivedWolfHP is a revived wolf's HP at the
// boot's 0.7 respawn restore share of it.
const (
	wolfLiveMaxHP = 632
	revivedWolfHP = 442.4
)

// bootResurrectingPetOwner boots an owner with a wolf collar who knows the
// power-100 resurrection, seeding the wolf's pets row at level 10 with exp
// before the owner enters the world.
func bootResurrectingPetOwner(t *testing.T, exp int64, extra ...gameservertest.Option) *petWorld {
	t.Helper()
	srv := bootPets(t, append([]gameservertest.Option{
		gameservertest.WithSkills(petResurrectionTable(t)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{penaltyWolfTemplate(), treeTemplate()})),
	}, extra...)...)
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, petResurrectSkillID, 1); err != nil {
		t.Fatalf("seed resurrection: %v", err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	if err := srv.Pets.Save(petCtx(), collarID, pet.State{
		Level: wolfLevel, Exp: exp, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: wolfMaxMeal,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	return h
}

// castResurrection has the client of player casterID target objectID and
// cast the resurrection on it, letting the hit land.
func castResurrection(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, casterID, objectID int32) {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, casterID)
	c.Send(encodeAction(objectID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(petResurrectSkillID))
	srv.Settle(t)
	srv.Advance(t, 600*time.Millisecond)
	srv.Settle(t)
}

// TestOwnPetResurrectionRevivesIt: an owner's resurrection on its own dead
// pet revives it at once, with no offer. The level-10 wolf at 510 exp lost
// 29 exp and a level on death; a power-100 resurrection gives all 29 back,
// so the wolf is level 10 again, plays the level-up animation and lifts its
// collar, and then stands up at 70% HP for everyone to see. Its decay is
// cancelled: it is still here, collar and all, after its 20 minutes.
func TestOwnPetResurrectionRevivesIt(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	h := bootResurrectingPetOwner(t, 510, gameservertest.WithDecay(decay.task))
	decay.attach(h.srv.State)
	wolf, _ := h.spawnWolf(t)
	observer, _ := joinSecondPlayer(t, h.srv)
	drainUntilQuiet(t, h.client)
	killPet(t, h, wolf)
	drainUntilQuiet(t, observer)
	if wolf.Exp() != 481 || wolf.Level() != wolfLevel-1 {
		t.Fatalf("dead wolf exp %d level %d, want 481 at level %d", wolf.Exp(), wolf.Level(), wolfLevel-1)
	}

	castResurrection(t, h.srv, h.client, h.ownerID, wolf.ObjectID())
	h.srv.InventoryUpdates.Tick()
	frames := drainFrames(t, h.client)

	if wolf.Dead() {
		t.Fatal("owner's resurrection left its own pet dead")
	}
	if frameIndex(frames, serverpackets.OpcodeConfirmDlg, serverpackets.ConfirmDlgResurrectionRequest) >= 0 {
		t.Fatal("owner was asked to confirm its own pet's resurrection")
	}
	if got := wolf.Exp(); got != 510 {
		t.Fatalf("revived wolf exp = %d, want 510", got)
	}
	if got := wolf.Level(); got != wolfLevel {
		t.Fatalf("revived wolf level = %d, want %d", got, wolfLevel)
	}
	if got := h.liveCollarEnchant(t); got != wolfLevel {
		t.Fatalf("collar enchant = %d, want %d back", got, wolfLevel)
	}
	if got := wolf.HP(); got != revivedWolfHP {
		t.Fatalf("revived wolf HP = %v, want %v", got, revivedWolfHP)
	}
	social, revive := frameIndex(frames, serverpackets.OpcodeSocialAction, wolf.ObjectID()), frameIndex(frames, serverpackets.OpcodeRevive, wolf.ObjectID())
	if social < 0 || revive < 0 || social > revive {
		t.Fatalf("owner frames: level-up SocialAction at %d, Revive at %d; want both, the level-up first", social, revive)
	}
	if frameIndex(drainFrames(t, observer), serverpackets.OpcodeRevive, wolf.ObjectID()) < 0 {
		t.Fatal("observer never saw the pet revive")
	}

	decay.passAndTick(t, h.srv, 1201*time.Second)
	if obj, ok := h.srv.State.Summon(h.ownerID); !ok || obj.ObjectID() != wolf.ObjectID() {
		t.Fatal("revived pet decayed at its old corpse deadline")
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("owner collar count = %d, want the collar kept", got)
	}
}

// newPartialRevivalScene has a dead level-10 wolf that died at 700 exp
// (losing 29), its owner, and a healer who knows a power-89 resurrection.
// Whatever the healer's WIT, a power-89 resurrection revives at 89 or 90
// percent (Formulas.calcRevivePower caps it at 90), and both give back
// Math.round(29 * 0.89) = Math.round(29 * 0.9) = 26 exp.
func newPartialRevivalScene(t *testing.T, extra ...gameservertest.Option) *petRevivalScene {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithSkills(petResurrectionTableWithPower(t, 89)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{penaltyWolfTemplate(), treeTemplate()})),
	}, extra...)
	h := bootOwnerWithCollarOpts(t, opts)
	if err := h.srv.Pets.Save(petCtx(), h.collarID, pet.State{
		Level: wolfLevel, Exp: 700, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: wolfMaxMeal,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	wolf, _ := h.spawnWolf(t)
	healer := h.srv.SeedCharacterFor(t, "healer", "Healer", 5, 0)
	if err := h.srv.KnownSkills.SetKnownSkill(context.Background(), healer.ID, 0, petResurrectSkillID, 1); err != nil {
		t.Fatalf("seed resurrection: %v", err)
	}
	s := &petRevivalScene{h: h, pet: wolf, healerID: healer.ID, healer: h.srv.DialClient(t, "healer", 1)}
	startInWorld(t, s.healer)
	drainUntilQuiet(t, s.healer)
	killPet(t, h, wolf)
	drainUntilQuiet(t, s.healer)
	if got := wolf.Exp(); got != 671 {
		t.Fatalf("dead wolf exp = %d, want 671", got)
	}
	return s
}

// TestAcceptedPetOfferRevivesThePet: another player's resurrection on a dead
// pet asks its owner, and accepting revives the pet with the offer's share
// of its lost exp (26 of 29) at 70% HP, cancels its decay, and lets it eat
// again: a dead pet's meal gauge stands still, a revived one's drops.
func TestAcceptedPetOfferRevivesThePet(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	s := newPartialRevivalScene(t, gameservertest.WithDecay(decay.task))
	decay.attach(s.h.srv.State)
	queue := s.h.srv.PlayerQueue(t, s.h.ownerID)

	fed := s.pet.Fed()
	runOn(t, queue, func() { s.pet.TickPet(s.h.srv.State) })
	if got := s.pet.Fed(); got != fed {
		t.Fatalf("dead pet meal gauge = %d after a feed tick, want %d kept", got, fed)
	}

	s.resurrect(t, s.pet.ObjectID())
	readResurrectionOffer(t, s.h.client, "Healer")
	if !s.pet.Dead() {
		t.Fatal("the offer revived the pet before its owner answered")
	}
	drainUntilQuiet(t, s.healer)
	s.h.client.Send(encodePetDlgAnswer(serverpackets.ConfirmDlgResurrectionRequest, 1))
	s.h.srv.Settle(t)

	if s.pet.Dead() {
		t.Fatal("accepted offer left the pet dead")
	}
	if got := s.pet.Exp(); got != 697 {
		t.Fatalf("revived pet exp = %d, want 697", got)
	}
	if got := s.pet.HP(); got != revivedWolfHP {
		t.Fatalf("revived pet HP = %v, want %v", got, revivedWolfHP)
	}
	if frameIndex(drainFrames(t, s.healer), serverpackets.OpcodeRevive, s.pet.ObjectID()) < 0 {
		t.Fatal("healer never saw the pet revive")
	}

	runOn(t, queue, func() { s.pet.TickPet(s.h.srv.State) })
	if got := s.pet.Fed(); got >= fed {
		t.Fatalf("revived pet meal gauge = %d after a feed tick, want below %d", got, fed)
	}

	decay.passAndTick(t, s.h.srv, 1201*time.Second)
	if _, ok := s.h.srv.State.Object(s.pet.ObjectID()); !ok {
		t.Fatal("revived pet decayed at its old corpse deadline")
	}
}

// TestPhoenixBlessedPetRevivesAtFullHP: a pet that died under a Phoenix
// Blessing offers its owner its own resurrection at power 100. Accepting it
// gives all 29 lost exp back, restores full HP and uses the blessing up.
func TestPhoenixBlessedPetRevivesAtFullHP(t *testing.T) {
	t.Parallel()
	h := bootResurrectingPetOwner(t, 700)
	wolf, _ := h.spawnWolf(t)
	e, err := effect.New(effect.Skill{ID: 438, Level: 1}, modelskill.EffectTemplate{Name: "PhoenixBless", Time: 1800, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(PhoenixBless): %v", err)
	}
	e.Effector, e.Effected = wolf, wolf
	runOn(t, wolf.Queue(), func() { wolf.EffectList().Add(e) })
	drainUntilQuiet(t, h.client)

	killPetKeepingFrames(t, h, wolf)
	readResurrectionOffer(t, h.client, "Owner")
	h.client.Send(encodePetDlgAnswer(serverpackets.ConfirmDlgResurrectionRequest, 1))
	h.srv.Settle(t)

	if wolf.Dead() {
		t.Fatal("accepted Phoenix Blessing offer left the pet dead")
	}
	if got := wolf.Exp(); got != 700 {
		t.Fatalf("revived pet exp = %d, want 700", got)
	}
	if got := wolf.HP(); got != wolfLiveMaxHP {
		t.Fatalf("revived pet HP = %v, want full %d", got, wolfLiveMaxHP)
	}
	if wolf.EffectList().IsAffected(effect.FlagPhoenixBlessing) {
		t.Fatal("revived pet kept its Phoenix Blessing")
	}
}

// resurrectionSkill is the power-100 resurrection a servitor owner knows.
func resurrectionSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: petResurrectSkillID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT",
		CastRange: 400, HitTime: 500, StaticHitTime: true, Power: 100,
	}
}

// TestResurrectedServitorKeepsItsDecay: a player's resurrection revives a
// dead servitor at 70% HP, but unlike a pet's it does not cancel the
// corpse's decay. At its corpse time the servitor leaves the world alive,
// freeing its owner's summon slot.
func TestResurrectedServitorKeepsItsDecay(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	o := bootServitorOwnerWithSkills(t, []modelskill.Definition{resurrectionSkill()}, gameservertest.WithDecay(decay.task))
	decay.attach(o.srv.State)
	servitor := o.summonServitor(t)
	o.killServitor(t, servitor)

	castResurrection(t, o.srv, o.client, o.id, servitor.ObjectID())
	frames := drainFrames(t, o.client)
	if servitor.Dead() {
		t.Fatal("resurrection left the servitor dead")
	}
	// Kat the Cat's live max HP is 410 whole points (its 500 base HP under
	// the template's CON bonus), at the 0.7 respawn restore share.
	if got := servitor.HP(); got != 287 {
		t.Fatalf("revived servitor HP = %v, want 287", got)
	}
	if frameIndex(frames, serverpackets.OpcodeRevive, servitor.ObjectID()) < 0 {
		t.Fatal("owner never saw the servitor revive")
	}

	decay.passAndTick(t, o.srv, (servitorCorpseTime+1)*time.Second)
	readUntilOpcode(t, o.client, serverpackets.OpcodePetDelete, "PetDelete for the revived servitor")
	if _, ok := o.srv.State.Object(servitor.ObjectID()); ok {
		t.Fatal("revived servitor still in the world after its corpse time")
	}
	if _, ok := o.srv.State.Summon(o.id); ok {
		t.Fatal("owner still holds the summon slot after the servitor left")
	}
}
