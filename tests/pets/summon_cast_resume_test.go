package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// castAfterAttack boots a pet with the five-second magic strike, has it
// attack a monster, and commands the strike mid-swing so it runs once the
// swing ends: the strike replaces the attack. It returns with the strike in
// flight and the owner's frames drained. The pet's combat rolls are fixed
// to roll.
func castAfterAttack(t *testing.T, roll int) (*petWorld, *summon.Actor, *npc.Hostile) {
	t.Helper()
	strike := wolfStrike()
	strike.Magic, strike.HitTime = true, magicStrikeHitTime
	h, petActor, _ := bootWolfStrikerWith(t, strike, gameservertest.WithAITask())
	runOnPetQueue(t, petActor, func() {
		petActor.SetRollSource(func(n int) int { return min(roll, n-1) })
	})
	hostile := spawnTankyHostile(t, h)
	sendPetMidSwing(t, h, petActor, hostile)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "queued strike ActionFailed")
	h.srv.AdvanceUntil(t, "the queued strike starting", petActor.CastingNow)
	drainFrames(t, h.client)
	return h, petActor, hostile
}

// requireResumedAttackBeforeInterruptMessage pins the frames an interrupted
// strike gives the owner when the pet resumes its attack: the cancel
// animation, then the pet's next swing, then CASTING_INTERRUPTED
// (CreatureCast.interrupt runs stop, whose FINISHED_CASTING resumes the
// attack, before it sends the message).
func requireResumedAttackBeforeInterruptMessage(t *testing.T, frames [][]byte, petActor *summon.Actor) {
	t.Helper()
	canceled, attacked, interrupted := -1, -1, -1
	for i, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillCanceled:
			if canceled < 0 && wire.NewReader(frame[1:]).ReadInt32() == petActor.ObjectID() {
				canceled = i
			}
		case serverpackets.OpcodeAttack:
			if attacked < 0 && petAttacks(frames[i:i+1], petActor) == 1 {
				attacked = i
			}
		case serverpackets.OpcodeSystemMessage:
			if interrupted < 0 && systemMessageID(t, frame) == serverpackets.SystemMessageCastingInterrupted {
				interrupted = i
			}
		}
	}
	if canceled < 0 || attacked < 0 || interrupted < 0 || canceled >= attacked || attacked >= interrupted {
		t.Fatalf("MagicSkillCanceled at %d, pet Attack at %d, CASTING_INTERRUPTED at %d; want all three in that order: opcodes %x",
			canceled, attacked, interrupted, frameOpcodes(frames))
	}
}

// requireKeepsAttacking pins a pet that went back to attacking the monster:
// its intent is the attack, and it keeps swinging.
func requireKeepsAttacking(t *testing.T, h *petWorld, petActor *summon.Actor) {
	t.Helper()
	if got := petActor.Intent(); got != summon.IntentAttackTarget {
		t.Fatalf("pet intent = %v after the strike was broken, want attack-target resumed", got)
	}
	if n := petAttacks(advanceCollecting(t, h, 3*time.Second), petActor); n < 2 {
		t.Fatalf("pet swings after the broken strike = %d, want it to keep attacking the monster", n)
	}
}

// TestBrokenPetCastResumesItsAttack breaks the strike of a pet that was
// attacking the monster. SummonAI.onEvtFinishedCasting resumes the attack
// the strike replaced; the pet's swing starts at once, so the tryToIdle of
// PlayableCast.stop waits it out and the pet keeps attacking.
func TestBrokenPetCastResumesItsAttack(t *testing.T) {
	t.Parallel()
	t.Run("AbortCast", func(t *testing.T) {
		t.Parallel()
		h, petActor, _ := castAfterAttack(t, 99)
		landOnPet(t, h, petActor, "AbortCast")

		requireResumedAttackBeforeInterruptMessage(t, drainFrames(t, h.client), petActor)
		requireKeepsAttacking(t, h, petActor)
	})
	t.Run("monster hit", func(t *testing.T) {
		t.Parallel()
		h, petActor, _ := castAfterAttack(t, 0)
		x, y, z := petActor.Position()
		attacker := h.srv.SpawnAttackingHostileNPCAt(t, location.Location{X: x + 20, Y: y, Z: z})
		drainFrames(t, h.client)
		attacker.DoAttack(t, petActor)

		requireResumedAttackBeforeInterruptMessage(t, drainFrames(t, h.client), petActor)
		requireKeepsAttacking(t, h, petActor)
	})
}

// TestBrokenPetCastWithoutAttackFollowsOwner breaks the strike of a pet
// that was not attacking: with nothing to resume, it goes idle, which for a
// pet following its owner means walking after it again.
func TestBrokenPetCastWithoutAttackFollowsOwner(t *testing.T) {
	t.Parallel()
	strike := wolfStrike()
	strike.Magic, strike.HitTime = true, magicStrikeHitTime
	h, petActor, _ := bootWolfStrikerWith(t, strike, gameservertest.WithAITask())
	startWolfStrike(t, h)
	landOnPet(t, h, petActor, "AbortCast")

	frames := drainFrames(t, h.client)
	if n := petAttacks(frames, petActor); n != 0 {
		t.Fatalf("pet swings after its strike broke = %d, want none", n)
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillCanceled); !ok {
		t.Fatalf("no MagicSkillCanceled for the broken strike: opcodes %x", frameOpcodes(frames))
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent = %v after the broken strike, want follow-owner", got)
	}
	h.ownerWalksAway(t)
	h.requirePetCatchesUp(t, petActor, "pet following its owner once its strike broke")
}

// TestCrowdControlledPetCastAfterAttackSwingsOnce lands a crowd-control
// effect on a pet whose strike replaced its attack. The effect's start runs
// Creature.abortAll before its flag is raised (EffectList.addEffectFromQueue
// calls onStart ahead of computeEffectFlags), so the stopped strike's
// FINISHED_CASTING resumes the attack and the pet, still in reach, starts one
// swing: observers see the cancel animation, then that swing, whose hit
// lands. The tryToIdle calls that follow wait the swing out; once it ends the
// effect's flag is up, so the pet goes idle and swings no more. A fear's
// first flee, asked for mid-swing, is queued behind it and denied by then.
func TestCrowdControlledPetCastAfterAttackSwingsOnce(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Stun", "Sleep", "Paralyze", "Fear"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, petActor, hostile := castAfterAttack(t, 99)
			px, py, _ := petActor.Position()
			landOnPet(t, h, petActor, name)
			hpAtEffect := hostile.HP()

			frames := drainFrames(t, h.client)
			canceled, attacked := -1, -1
			for i, frame := range frames {
				switch frame[0] {
				case serverpackets.OpcodeMagicSkillCanceled:
					if canceled < 0 && wire.NewReader(frame[1:]).ReadInt32() == petActor.ObjectID() {
						canceled = i
					}
				case serverpackets.OpcodeAttack:
					if attacked < 0 && petAttacks(frames[i:i+1], petActor) == 1 {
						attacked = i
					}
				}
			}
			if canceled < 0 || attacked < 0 || canceled >= attacked {
				t.Fatalf("MagicSkillCanceled at %d, pet Attack at %d; want the cancel, then one swing: opcodes %x",
					canceled, attacked, frameOpcodes(frames))
			}

			later := advanceCollecting(t, h, 3*time.Second)
			if n := petAttacks(frames, petActor) + petAttacks(later, petActor); n != 1 {
				t.Fatalf("pet swings once the %s landed = %d, want exactly the resumed one", name, n)
			}
			if hp := hostile.HP(); hp >= hpAtEffect {
				t.Fatalf("monster HP = %v after the resumed swing, want below %v: its hit lands", hp, hpAtEffect)
			}
			if petActor.IsAttackingNow() || petActor.CastingNow() {
				t.Fatalf("pet attacking %v, casting %v after its swing; want neither", petActor.IsAttackingNow(), petActor.CastingNow())
			}
			if got := petActor.Intent(); got != summon.IntentFollowOwner {
				t.Fatalf("pet intent = %v after the %s, want follow-owner", got, name)
			}
			if x, y, _ := petActor.Position(); x != px || y != py {
				t.Fatalf("pet moved from (%d,%d) to (%d,%d) under the %s, want it held in place", px, py, x, y, name)
			}
		})
	}
}

// TestTeleportedPetCastAfterAttackDoesNotResume has the owner teleport
// while the pet's strike, which replaced its attack, is in flight. The
// teleport's abort ends the strike without resuming the attack, so the pet
// arrives following its owner and never swings mid-teleport.
func TestTeleportedPetCastAfterAttackDoesNotResume(t *testing.T) {
	t.Parallel()
	h, petActor, _ := castAfterAttack(t, 99)
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	owner, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an online character", h.ownerID, obj)
	}

	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	owner.TeleportTo(x+300, y, z, 0)
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner TeleportToLocation")
	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	frames = append(frames, readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "pet TeleportToLocation")...)
	h.handled(t)
	frames = append(frames, drainFrames(t, h.client)...)

	if n := petAttacks(frames, petActor); n != 0 {
		t.Fatalf("pet swings across the teleport = %d, want none: opcodes %x", n, frameOpcodes(frames))
	}
	if petActor.CastingNow() || petActor.IsAttackingNow() {
		t.Fatalf("pet casting %v, attacking %v after the teleport; want neither", petActor.CastingNow(), petActor.IsAttackingNow())
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent = %v after the teleport, want follow-owner", got)
	}
}
