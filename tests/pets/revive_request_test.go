package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: Summon.doDie's Phoenix Blessing offer (Summon.java:170-178),
// Player.reviveRequest's refusals (Player.java:6015-6056) and the Resurrect
// handler's foreign-pet branch (Resurrect.java:32-37), aCis revision in the
// outer repo.

const petResurrectSkillID = 1016

// petResurrectionTable is the pet fixture's skills plus a CORPSE_PLAYER
// resurrection, which takes any dead playable, a pet included.
func petResurrectionTable(t *testing.T) *skillstate.Persistence {
	t.Helper()
	return petResurrectionTableWithPower(t, 100)
}

// petResurrectionTableWithPower is petResurrectionTable with a resurrection
// of the given power.
func petResurrectionTableWithPower(t *testing.T, power float32) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
		{
			ID: petResurrectSkillID, Level: 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT",
			CastRange: 400, HitTime: 500, StaticHitTime: true, Power: power,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

func encodePetDlgAnswer(messageID, answer int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeDlgAnswer)
	w.WriteInt32(messageID)
	w.WriteInt32(answer)
	w.WriteInt32(0)
	return w.Bytes()
}

// readResurrectionOffer reads c's frames until the resurrection ConfirmDlg
// and asserts it names reviver.
func readResurrectionOffer(t *testing.T, c *testsupport.ScriptedClient, reviver string) {
	t.Helper()
	frames := readUntilOpcode(t, c, serverpackets.OpcodeConfirmDlg, "resurrection ConfirmDlg")
	r := wire.NewReader(frames[len(frames)-1][1:])
	if id, params, typ := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != serverpackets.ConfirmDlgResurrectionRequest || params != 1 || typ != 0 {
		t.Fatalf("ConfirmDlg = id %d params %d type %d, want %d/1/text", id, params, typ, serverpackets.ConfirmDlgResurrectionRequest)
	}
	if got := r.ReadString(); got != reviver {
		t.Fatalf("ConfirmDlg reviver = %q, want %q", got, reviver)
	}
}

func assertNoConfirmDlg(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	for _, f := range drainFrames(t, c) {
		if f[0] == serverpackets.OpcodeConfirmDlg {
			t.Fatalf("%s: got a resurrection offer, want none", what)
		}
	}
}

func assertRefusal(t *testing.T, c *testsupport.ScriptedClient, messageID int) {
	t.Helper()
	for _, f := range drainFrames(t, c) {
		if f[0] == serverpackets.OpcodeSystemMessage && int(wire.NewReader(f[1:]).ReadInt32()) == messageID {
			return
		}
	}
	t.Fatalf("healer never got system message %d", messageID)
}

// TestPhoenixBlessedPetDeathOffersOwnerRevive: a pet that dies under a
// Phoenix Blessing keeps it, and its owner is offered the pet's
// resurrection in its own name.
func TestPhoenixBlessedPetDeathOffersOwnerRevive(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	e, err := effect.New(effect.Skill{ID: 438, Level: 1}, modelskill.EffectTemplate{Name: "PhoenixBless", Time: 1800, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(PhoenixBless): %v", err)
	}
	e.Effector, e.Effected = pet, pet
	runOn(t, pet.Queue(), func() { pet.EffectList().Add(e) })
	drainUntilQuiet(t, h.client)

	killPetKeepingFrames(t, h, pet)
	readResurrectionOffer(t, h.client, "Owner")
	if !pet.EffectList().IsAffected(effect.FlagPhoenixBlessing) {
		t.Fatal("pet lost its Phoenix Blessing at death")
	}
}

// killPetKeepingFrames kills pet without draining its owner's frames.
func killPetKeepingFrames(t *testing.T, h *petWorld, pet *summon.Actor) {
	t.Helper()
	hostile := h.srv.SpawnHostileNPC(t)
	pet.ReduceHP(pet.HP()+1, hostile, modelskill.Definition{})
	h.srv.AdvanceUntil(t, "pet dead", pet.Dead)
}

// petRevivalScene has a dead pet, its owner, and a healer who knows a
// resurrection standing next to them.
type petRevivalScene struct {
	h        *petWorld
	pet      *summon.Actor
	healer   *testsupport.ScriptedClient
	healerID int32
}

func newPetRevivalScene(t *testing.T) *petRevivalScene {
	t.Helper()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithSkills(petResurrectionTable(t))})
	pet, _ := h.spawnWolf(t)
	healer := h.srv.SeedCharacterFor(t, "healer", "Healer", 5, 0)
	if err := h.srv.KnownSkills.SetKnownSkill(context.Background(), healer.ID, 0, petResurrectSkillID, 1); err != nil {
		t.Fatalf("seed resurrection: %v", err)
	}
	s := &petRevivalScene{h: h, pet: pet, healerID: healer.ID, healer: h.srv.DialClient(t, "healer", 1)}
	startInWorld(t, s.healer)
	drainUntilQuiet(t, s.healer)
	killPet(t, h, pet)
	drainUntilQuiet(t, s.healer)
	return s
}

// resurrect has the healer target objectID and cast on it, letting the hit
// land.
func (s *petRevivalScene) resurrect(t *testing.T, objectID int32) {
	t.Helper()
	x, y, z := s.h.srv.PlayerPosition(t, s.healerID)
	s.healer.Send(encodeAction(objectID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, s.healer)
	s.healer.Send(encodeRequestMagicSkillUse(petResurrectSkillID))
	s.h.srv.Settle(t)
	s.h.srv.Advance(t, 600*time.Millisecond)
}

// TestForeignPetResurrectionAsksItsOwner: another player's resurrection on
// a dead pet asks the pet's owner, and the reference refusals answer the
// healer: a second pet offer, an offer for the owner while the pet's is
// open, and a pet offer while the owner's own is open.
func TestForeignPetResurrectionAsksItsOwner(t *testing.T) {
	t.Parallel()
	t.Run("pet offer first", func(t *testing.T) {
		t.Parallel()
		s := newPetRevivalScene(t)
		s.resurrect(t, s.pet.ObjectID())
		readResurrectionOffer(t, s.h.client, "Healer")
		if !s.pet.Dead() {
			t.Fatal("the offer revived the pet without asking")
		}
		drainUntilQuiet(t, s.healer)

		s.resurrect(t, s.pet.ObjectID())
		assertRefusal(t, s.healer, serverpackets.SystemMessageResHasAlreadyBeenProposed)
		assertNoConfirmDlg(t, s.h.client, "second pet offer")

		s.h.srv.MarkPlayerDead(t, s.h.ownerID)
		drainUntilQuiet(t, s.h.client)
		s.resurrect(t, s.h.ownerID)
		assertRefusal(t, s.healer, serverpackets.SystemMessageMasterCannotRes)
		assertNoConfirmDlg(t, s.h.client, "owner offer while the pet's is open")

		// Accepting the pet's offer revives the pet and closes the offer:
		// once the pet dies again, a new resurrection on it reaches the
		// owner as a fresh offer.
		drainUntilQuiet(t, s.healer)
		s.h.client.Send(encodePetDlgAnswer(serverpackets.ConfirmDlgResurrectionRequest, 1))
		s.h.srv.Settle(t)
		if s.pet.Dead() {
			t.Fatal("accepting the pet's offer left the pet dead")
		}
		killPet(t, s.h, s.pet)
		drainUntilQuiet(t, s.healer)
		s.resurrect(t, s.pet.ObjectID())
		readResurrectionOffer(t, s.h.client, "Healer")
		for _, f := range drainFrames(t, s.healer) {
			if f[0] == serverpackets.OpcodeSystemMessage && int(wire.NewReader(f[1:]).ReadInt32()) == serverpackets.SystemMessageResHasAlreadyBeenProposed {
				t.Fatal("healer got 1513 after the owner accepted: the pet's offer stayed open")
			}
		}
	})

	t.Run("owner offer first", func(t *testing.T) {
		t.Parallel()
		s := newPetRevivalScene(t)
		s.h.srv.MarkPlayerDead(t, s.h.ownerID)
		drainUntilQuiet(t, s.h.client)
		drainUntilQuiet(t, s.healer)
		s.resurrect(t, s.h.ownerID)
		readResurrectionOffer(t, s.h.client, "Healer")
		drainUntilQuiet(t, s.healer)

		s.resurrect(t, s.pet.ObjectID())
		assertRefusal(t, s.healer, serverpackets.SystemMessageCannotResPet2)
		assertNoConfirmDlg(t, s.h.client, "pet offer while the owner's is open")

		// Declining closes the owner's offer; the pet's can then be made.
		s.h.client.Send(encodePetDlgAnswer(serverpackets.ConfirmDlgResurrectionRequest, 0))
		s.h.srv.Settle(t)
		s.resurrect(t, s.pet.ObjectID())
		readResurrectionOffer(t, s.h.client, "Healer")
	})
}
