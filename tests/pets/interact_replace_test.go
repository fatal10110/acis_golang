package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	// bowTemplateID and woodenArrowTemplateID are a no-grade bow and its
	// arrows.
	bowTemplateID         = int32(14)
	woodenArrowTemplateID = int32(17)

	// replaceGroundSkillID is a ground-targeted buff with a short cast range,
	// so a far ground point is walked to first; replaceSelfSkillID is a
	// plain self cast.
	replaceGroundSkillID = 9101
	replaceSelfSkillID   = 9102
)

// isAttackBy reports whether frame is a swing or shot started by attackerID.
func isAttackBy(frame []byte, attackerID int32) bool {
	return frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == attackerID
}

// assertNoAttackBy fails when attackerID starts a swing or shot within d.
func assertNoAttackBy(t *testing.T, c *testsupport.ScriptedClient, attackerID int32, d time.Duration, what string) {
	t.Helper()
	for end := c.Now().Add(d); c.Now().Before(end); {
		frame := c.ReadWithTimeout(end.Sub(c.Now()))
		if frame == nil {
			return
		}
		if isAttackBy(frame, attackerID) {
			t.Fatalf("the owner attacked again %s", what)
		}
	}
}

// interactNow clicks the already-selected pet, with shift held when shift
// is set, and returns the click's own answer.
func interactNow(t *testing.T, h *petWorld, pet *summon.Actor, shift bool, what string) [][]byte {
	t.Helper()
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), shift))
	frames := h.srv.ReadQueued(t, h.client)
	if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("pet click %s = opcodes %x, want ActionFailed first", what, frameOpcodes(frames))
	}
	return frames
}

// selectPetNow selects pet without interacting with it.
func selectPetNow(t *testing.T, h *petWorld, pet *summon.Actor) {
	t.Helper()
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	for _, f := range h.srv.ReadQueued(t, h.client) {
		if f[0] == serverpackets.OpcodePetStatusShow {
			t.Fatal("selecting the pet opened its status window")
		}
	}
}

// TestOwnedPetInteractDuringBowReuseEndsTheAttack pins an owner's click on
// its own pet while its bow reloads. Neither a swing nor a cast is in
// flight, so PlayableAI.tryToInteract (PlayableAI.java:373-390) runs the
// interact at once, and doInteractIntention's prepareIntention replaces the
// ATTACK intention. PlayerAI.thinkInteract (PlayerAI.java:413-462) opens the
// status window and ends idle, so PlayerAI.onEvtBowAttackReuse
// (PlayerAI.java:127-144) finds no ATTACK intention and the bow does not
// fire again.
func TestOwnedPetInteractDuringBowReuseEndsTheAttack(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{bowTemplateID, 1}, seedItem{woodenArrowTemplateID, 10})
	if !h.srv.DrivesClock() {
		t.Skip("landing a click inside the bow reuse needs the driven clock")
	}
	for _, id := range []int32{h.seededItem(t, bowTemplateID), h.seededItem(t, woodenArrowTemplateID)} {
		h.client.Send(encodeUseItem(id, false))
		drainUntilQuiet(t, h.client)
	}
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px - 50, Y: py, Z: pz})
	hostile := h.srv.SpawnHostileNPCAt(t, location.Location{X: px + 30, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	// The shot's SetupGauge covers the shot plus the bow's reuse: about half
	// of it (500000/pAtkSpd against 1500*345/pAtkSpd) is the shot itself.
	gauge := time.Duration(0)
	for {
		frame := mustRead(t, h.client, "owner bow shot")
		if frame[0] == serverpackets.OpcodeSetupGauge {
			r := wire.NewReader(frame[1:])
			r.ReadInt32()
			gauge = time.Duration(r.ReadInt32()) * time.Millisecond
		}
		if isAttackBy(frame, h.ownerID) {
			break
		}
	}
	if gauge == 0 {
		t.Fatal("the bow shot came without its SetupGauge")
	}
	shotAt := h.client.Now()
	selectPetNow(t, h, pet)

	// Wait into the reuse: past the shot, well before the next one.
	for reuse := shotAt.Add(gauge * 3 / 4); h.client.Now().Before(reuse); {
		frame := h.client.ReadWithTimeout(reuse.Sub(h.client.Now()))
		if frame == nil {
			break
		}
		if isAttackBy(frame, h.ownerID) {
			t.Fatal("the bow fired again before its reuse ended")
		}
	}

	frames := interactNow(t, h, pet, false, "during the bow reuse")
	if !hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("pet click during the bow reuse = opcodes %x, want the status window at once", frameOpcodes(frames))
	}
	assertNoAttackBy(t, h.client, h.ownerID, 2*gauge, "after its pet interact replaced the attack")
}

// TestOwnedPetInteractDuringChaseEndsTheAttack is the same for a bare-handed
// owner still walking toward a monster out of reach. The interact replaces
// the ATTACK intention and cancels its follow task (PlayableAI
// .prepareIntention, PlayableAI.java:31-40). A pet within reach gets its
// status window and the interact ends idle, which stops the chase walk
// (CreatureAI.thinkIdle, CreatureAI.java:209-212). A pet out of reach is
// walked to instead, unless shift is held: PlayerMove.maybeMoveToPawn
// (PlayerMove.java:335-352) then does not move and thinkInteract
// (PlayerAI.java:435-440) ends idle, which stops the chase walk too. Either
// way the owner never reaches the monster or swings at it.
func TestOwnedPetInteractDuringChaseEndsTheAttack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		petOff int
		shift  bool
	}{
		{name: "pet in reach", petOff: 50},
		{name: "pet walked to", petOff: 300},
		{name: "shift-click out of reach", petOff: 300, shift: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t)
			if !h.srv.DrivesClock() {
				t.Skip("landing a click inside the chase needs the driven clock")
			}
			pet, _ := h.spawnWolf(t)
			px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
			placePet(t, pet, location.Location{X: px - tc.petOff, Y: py, Z: pz})
			hostileAt := location.Location{X: px + 600, Y: py, Z: pz}
			hostile := h.srv.SpawnHostileNPCAt(t, hostileAt)
			drainUntilQuiet(t, h.client)
			selectPetNow(t, h, pet)
			drainUntilQuiet(t, h.client)

			h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
			drainUntilQuiet(t, h.client)
			h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
			h.srv.ReadQueued(t, h.client)
			if !h.srv.PlayerMove(t, h.ownerID).Moving() {
				t.Fatal("the owner is not chasing the monster")
			}
			selectPetNow(t, h, pet)

			frames := interactNow(t, h, pet, tc.shift, "during the chase")
			switch {
			case tc.shift:
				if hasOpcode(frames, serverpackets.OpcodePetStatusShow) || hasOpcode(frames, serverpackets.OpcodeMoveToPawn) {
					t.Fatalf("shift-click on the pet out of reach during the chase = opcodes %x, want no approach and no status window", frameOpcodes(frames))
				}
				if h.srv.PlayerMove(t, h.ownerID).Moving() {
					t.Fatal("the owner kept walking after its shift-clicked pet interact ended idle")
				}
				var order []byte
				for _, f := range frames {
					switch {
					case f[0] == serverpackets.OpcodeActionFailed,
						f[0] == serverpackets.OpcodeStopMove && wire.NewReader(f[1:]).ReadInt32() == h.ownerID:
						order = append(order, f[0])
					}
				}
				want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeStopMove}
				if string(order) != string(want) {
					t.Fatalf("shift-click on the pet out of reach during the chase order = %x, want %x (all opcodes %x)", order, want, frameOpcodes(frames))
				}
			case tc.petOff == 50:
				if !hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
					t.Fatalf("pet click in reach during the chase = opcodes %x, want the status window at once", frameOpcodes(frames))
				}
				if h.srv.PlayerMove(t, h.ownerID).Moving() {
					t.Fatal("the owner kept walking after its pet interact ended idle")
				}
				// The window opens first; the idle that ends the interact
				// then stops the walk.
				var order []byte
				for _, f := range frames {
					switch {
					case f[0] == serverpackets.OpcodeActionFailed, f[0] == serverpackets.OpcodePetStatusShow,
						f[0] == serverpackets.OpcodeMoveToPawn && isOwnerInteractFrame(f, h.ownerID, pet),
						f[0] == serverpackets.OpcodeStopMove && wire.NewReader(f[1:]).ReadInt32() == h.ownerID:
						order = append(order, f[0])
					}
				}
				want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow, serverpackets.OpcodeStopMove}
				if string(order) != string(want) {
					t.Fatalf("pet click in reach during the chase order = %x, want %x (all opcodes %x)", order, want, frameOpcodes(frames))
				}
			default:
				walk, ok := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
				if !ok {
					t.Fatalf("pet click out of reach during the chase = opcodes %x, want the approach", frameOpcodes(frames))
				}
				if objectID, targetID, _, _ := moveToPawnFields(t, walk); objectID != h.ownerID || targetID != pet.ObjectID() {
					t.Fatalf("approach MoveToPawn = mover %d target %d, want %d toward the pet %d", objectID, targetID, h.ownerID, pet.ObjectID())
				}
				readUntilOpcode(t, h.client, serverpackets.OpcodePetStatusShow, "status window after the approach")
			}
			stoppedAt := location.Location{}
			stoppedAt.X, stoppedAt.Y, stoppedAt.Z = h.srv.PlayerPosition(t, h.ownerID)
			assertNoAttackBy(t, h.client, h.ownerID, 4*time.Second, "after its pet interact replaced the chase")
			ox, oy, oz := h.srv.PlayerPosition(t, h.ownerID)
			if at := (location.Location{X: ox, Y: oy, Z: oz}); at != stoppedAt {
				t.Fatalf("the owner moved from %+v to %+v after its pet interact", stoppedAt, at)
			}
		})
	}
}

func encodeRequestExMagicSkillUseGround(x, y, z, skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestExMagicSkillUseGround)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(skillID)
	w.WriteInt32(0) // ctrl
	w.WriteUint8(0) // shift
	return w.Bytes()
}

// bootGroundCaster is bootOwnerWithCollar with the ground and self casts
// known.
func bootGroundCaster(t *testing.T) *petWorld {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{
			ID: replaceGroundSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
			CastRange: 100, HitTime: 500, StaticHitTime: true, StaticReuse: true, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
		},
		{
			ID: replaceSelfSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 0,
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t, gameservertest.WithSkills(skills))
	ownerID := srv.SoleObjectID(t)
	for _, id := range []int{replaceGroundSkillID, replaceSelfSkillID} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	return h
}

// magicSkillUseID returns the skill a MagicSkillUse cast by casterID
// announces, or false for any other frame.
func magicSkillUseID(frame []byte, casterID int32) (int32, bool) {
	if frame[0] != serverpackets.OpcodeMagicSkillUse {
		return 0, false
	}
	r := wire.NewReader(frame[1:])
	if r.ReadInt32() != casterID {
		return 0, false
	}
	r.ReadInt32()
	return r.ReadInt32(), true
}

// TestOwnedPetInteractDropsGroundCastApproach clicks a pet in reach while
// the owner walks toward a ground point out of its skill's cast range.
// doInteractIntention's prepareIntention (PlayableAI.java:31-40) replaces
// the CAST intention and resets the next one, and thinkInteract
// (PlayerAI.java:413-462) ends idle, which stops the walk. Nothing of the
// cast survives: a later self cast ends with no ground cast resumed, no
// walk toward the ground point and no MagicSkillUse of it.
func TestOwnedPetInteractDropsGroundCastApproach(t *testing.T) {
	t.Parallel()
	h := bootGroundCaster(t)
	if !h.srv.DrivesClock() {
		t.Skip("reading past the self cast's end needs the driven clock")
	}
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px - 50, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	selectPetNow(t, h, pet)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestExMagicSkillUseGround(int32(px+600), int32(py), int32(pz), replaceGroundSkillID))
	if frames := h.srv.ReadQueued(t, h.client); !hasOpcode(frames, serverpackets.OpcodeMoveToLocation) {
		t.Fatalf("ground cast out of range = opcodes %x, want the approach walk", frameOpcodes(frames))
	}
	if !h.srv.PlayerMove(t, h.ownerID).Moving() {
		t.Fatal("the owner is not walking toward the ground point")
	}

	frames := interactNow(t, h, pet, false, "during the ground-cast approach")
	if !hasOpcode(frames, serverpackets.OpcodePetStatusShow) || !hasOpcode(frames, serverpackets.OpcodeStopMove) {
		t.Fatalf("pet click in reach during the ground-cast approach = opcodes %x, want the status window and StopMove", frameOpcodes(frames))
	}
	if h.srv.PlayerMove(t, h.ownerID).Moving() {
		t.Fatal("the owner kept walking after its pet interact ended idle")
	}
	drainUntilQuiet(t, h.client)
	stoppedAt := location.Location{}
	stoppedAt.X, stoppedAt.Y, stoppedAt.Z = h.srv.PlayerPosition(t, h.ownerID)

	h.client.Send(encodeRequestMagicSkillUse(replaceSelfSkillID))
	selfCast := false
	for end := h.client.Now().Add(4 * time.Second); h.client.Now().Before(end); {
		frame := h.client.ReadWithTimeout(end.Sub(h.client.Now()))
		if frame == nil {
			break
		}
		if skill, ok := magicSkillUseID(frame, h.ownerID); ok {
			if skill == replaceGroundSkillID {
				t.Fatal("the ground cast the pet interact replaced ran after the self cast")
			}
			selfCast = selfCast || skill == replaceSelfSkillID
		}
		if frame[0] == serverpackets.OpcodeMoveToLocation && wire.NewReader(frame[1:]).ReadInt32() == h.ownerID {
			t.Fatal("the owner walked again toward the ground point after the self cast")
		}
	}
	if !selfCast {
		t.Fatal("the self cast never started; the test needs its end to resume anything queued")
	}
	ox, oy, oz := h.srv.PlayerPosition(t, h.ownerID)
	if at := (location.Location{X: ox, Y: oy, Z: oz}); at != stoppedAt {
		t.Fatalf("the owner moved from %+v to %+v after its pet interact", stoppedAt, at)
	}
}
