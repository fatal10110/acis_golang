package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// startOwnerSwing starts the owner swinging at a monster beside it and
// returns once its first Attack is out, with the swing still in flight.
func startOwnerSwing(t *testing.T, h *petWorld) {
	t.Helper()
	if !h.srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	hostile := h.srv.SpawnHostileNPCAt(t, location.Location{X: px + 30, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	for {
		frame := mustRead(t, h.client, "owner swing")
		if frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == h.ownerID {
			break
		}
	}
	h.srv.ReadQueued(t, h.client)
}

// TestSummonItemsMidSwingAnswerCannotSummonInCombat pins
// SummonItems.java:48-52 for every summon kind: a swing in flight refuses
// the tree kit, the pet collar and the wyvern collar alike with
// YOU_CANNOT_SUMMON_IN_COMBAT, before any cast, mount or planting.
func TestSummonItemsMidSwingAnswerCannotSummonInCombat(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		template int32
	}{
		{name: "tree kit", template: treeKitID},
		{name: "pet collar", template: wolfCollarID},
		{name: "wyvern collar", template: wyvernCollarID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var seeds []seedItem
			if tt.template != wolfCollarID {
				seeds = append(seeds, seedItem{TemplateID: tt.template, Count: 1})
			}
			h := bootOwnerWithCollar(t, seeds...)
			used := h.collarID
			if tt.template != wolfCollarID {
				used = h.seededItem(t, tt.template)
			}
			startOwnerSwing(t, h)

			h.client.Send(encodeUseItem(used, false))
			frames := h.srv.ReadQueued(t, h.client)
			if len(frames) == 0 {
				t.Fatalf("%s mid-swing: no answer", tt.name)
			}
			assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageYouCannotSummonInCombat)
			for _, f := range frames[1:] {
				switch f[0] {
				case serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeRide, serverpackets.OpcodeSystemMessage:
					t.Fatalf("%s mid-swing = opcodes %x, want YOU_CANNOT_SUMMON_IN_COMBAT alone", tt.name, frameOpcodes(frames))
				}
			}
			if h.srv.PlayerCastingNow(t, h.ownerID) {
				t.Fatalf("%s mid-swing started a cast", tt.name)
			}
			if h.character(t).Mounted() {
				t.Fatalf("%s mid-swing mounted the owner", tt.name)
			}
			if decorationCount(h) != 0 {
				t.Fatalf("%s mid-swing planted a decoration", tt.name)
			}
			if tt.template == treeKitID {
				if got := h.ownerItemCount(t, treeKitID); got != 1 {
					t.Fatalf("tree kit count = %d after a mid-swing use, want 1", got)
				}
			}
		})
	}
}

// TestWyvernCollarInStanceWithoutSwingMounts pins that the gate reads the
// swing, not the attack stance: an owner a monster has just hit is in
// stance but not swinging, and its wyvern collar mounts.
func TestWyvernCollarInStanceWithoutSwingMounts(t *testing.T) {
	t.Parallel()
	h := bootWyvernOwner(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	attacker := h.srv.SpawnAttackingHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	rider := h.character(t)
	attacker.DoAttack(t, rider)
	readUntilOpcode(t, h.client, serverpackets.OpcodeAutoAttackStart, "owner AutoAttackStart")
	drainUntilQuiet(t, h.client)
	if !rider.InCombat() || rider.Dead() {
		t.Fatalf("owner after the hit: in combat %v dead %v, want in combat and alive", rider.InCombat(), rider.Dead())
	}

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeRide, "mount Ride")
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == serverpackets.SystemMessageYouCannotSummonInCombat {
			t.Fatal("wyvern collar in stance without a swing answered YOU_CANNOT_SUMMON_IN_COMBAT")
		}
	}
	if !rider.Mounted() {
		t.Fatal("wyvern collar in stance without a swing did not mount")
	}
	drainUntilQuiet(t, h.client)
}

// bootCollarSkill boots an owner with a collar whose SUMMON_CREATURE is
// summon, in place of the zero-time fixture skill.
func bootCollarSkill(t *testing.T, summon modelskill.Definition) *petWorld {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		summon,
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
	}), gamesql.NewCharacterSkillStore(db))
	return bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithSkills(skills)})
}

// summonCreature is the collar's SUMMON_CREATURE with the shipped skill's
// static 5000ms hit time and magic flag (skills/2000-2099.xml, id 2046).
func summonCreature() modelskill.Definition {
	return modelskill.Definition{
		ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON_CREATURE", Magic: true, StaticHitTime: true, HitTime: 5000, StaticReuse: true,
	}
}

// TestCollarCastStartPrecedesSummonAPet pins SummonItems.java:95-96: the
// collar's cast starts first (CreatureCast.doCast's MagicSkillUse, then its
// blue gauge for a hit time over 410ms), and SUMMON_A_PET follows it.
func TestCollarCastStartPrecedesSummonAPet(t *testing.T) {
	t.Parallel()
	h := bootCollarSkill(t, summonCreature())

	h.client.Send(encodeUseItem(h.collarID, false))
	assertFrameOpcode(t, mustRead(t, h.client, "collar MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")
	gauge := mustRead(t, h.client, "collar SetupGauge")
	assertFrameOpcode(t, gauge, serverpackets.OpcodeSetupGauge, "collar SetupGauge")
	r := wire.NewReader(gauge[1:])
	if color, current, total := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); color != int32(serverpackets.GaugeBlue) || current != 5000 || total != 5000 {
		t.Fatalf("collar gauge = color %d %d/%d, want blue 5000/5000", color, current, total)
	}
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET system message"), serverpackets.SystemMessageSummonAPet)
	if !h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("collar cast not in flight after SUMMON_A_PET")
	}
}

// TestRefusedCollarCastStillSendsSummonAPet pins SummonItems.java:95-96 for
// a cast tryToCast refuses: SUMMON_A_PET still follows the refusal's
// answer. A skill on reuse is refused at the attempt gate
// (CreatureCast.canAttemptCast) with its reason and ActionFailed; one the
// caster lacks the MP for is refused at canCast (meetsHpMpConditions) with
// NOT_ENOUGH_MP alone, no ActionFailed.
func TestRefusedCollarCastStillSendsSummonAPet(t *testing.T) {
	t.Parallel()
	t.Run("on reuse", func(t *testing.T) {
		t.Parallel()
		summon := summonCreature()
		summon.HitTime, summon.ReuseDelay = 0, 600_000
		h := bootCollarSkill(t, summon)
		h.spawnWolf(t)
		h.returnPet(t)

		h.client.Send(encodeUseItem(h.collarID, false))
		frames := drainFrames(t, h.client)
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage}; string(got) != string(want) {
			t.Fatalf("collar on reuse = opcodes %x, want %x", got, want)
		}
		assertSystemMessageID(t, frames[0], serverpackets.SystemMessageS1PreparedForReuse)
		assertStaticSystemMessage(t, frames[2], serverpackets.SystemMessageSummonAPet)
		if _, ok := h.srv.State.Summon(h.ownerID); ok || h.srv.PlayerCastingNow(t, h.ownerID) {
			t.Fatal("collar on reuse summoned or started a cast")
		}
	})
	t.Run("short of MP", func(t *testing.T) {
		t.Parallel()
		summon := summonCreature()
		summon.MPConsume = 100_000
		h := bootCollarSkill(t, summon)

		h.client.Send(encodeUseItem(h.collarID, false))
		frames := drainFrames(t, h.client)
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage}; string(got) != string(want) {
			t.Fatalf("collar short of MP = opcodes %x, want NOT_ENOUGH_MP then SUMMON_A_PET", got)
		}
		assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNotEnoughMP)
		assertStaticSystemMessage(t, frames[1], serverpackets.SystemMessageSummonAPet)
		if h.srv.PlayerCastingNow(t, h.ownerID) {
			t.Fatal("collar short of MP started a cast")
		}
	})
	// The collar's skill is magic, so a Mute on the owner refuses it at
	// canCast (meetsHpMpDisabledConditions), which answers nothing of its
	// own: SUMMON_A_PET is the only packet.
	t.Run("muted", func(t *testing.T) {
		t.Parallel()
		h := bootCollarSkill(t, summonCreature())
		obj, ok := h.srv.State.Player(h.ownerID)
		if !ok {
			t.Fatal("owner not in world")
		}
		owner := obj.(interface {
			effect.Actor
			EffectList() *effect.List
		})
		mute, err := effect.New(effect.Skill{ID: 1064, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Mute", Time: 30})
		if err != nil {
			t.Fatalf("effect.New(Mute): %v", err)
		}
		mute.Effector, mute.Effected = owner, owner
		runOn(t, h.srv.PlayerQueue(t, h.ownerID), func() { owner.EffectList().Add(mute) })
		drainUntilQuiet(t, h.client)

		h.client.Send(encodeUseItem(h.collarID, false))
		frames := drainFrames(t, h.client)
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeSystemMessage}; string(got) != string(want) {
			t.Fatalf("muted collar = opcodes %x, want SUMMON_A_PET alone", got)
		}
		assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageSummonAPet)
		if _, ok := h.srv.State.Summon(h.ownerID); ok || h.srv.PlayerCastingNow(t, h.ownerID) {
			t.Fatal("muted collar summoned or started a cast")
		}
	})
}
