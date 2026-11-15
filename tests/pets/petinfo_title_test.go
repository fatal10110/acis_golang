package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// petInfoNameTitle reads PetInfo's name and title strings.
func petInfoNameTitle(t *testing.T, frame []byte) (name, title string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePetInfo, "PetInfo")
	r := wire.NewReader(frame[1:])
	for range 10 + 1 + 8 { // type .. cast speed, P.Atk. speed, run/walk speed pairs
		r.ReadInt32()
	}
	for range 4 { // speed multipliers, collision radius and height
		r.ReadFloat64()
	}
	for range 3 { // weapon, armor, 0
		r.ReadInt32()
	}
	for range 5 { // owner present, 1, in combat, alike dead, summon animation
		r.ReadUint8()
	}
	name, title = r.ReadString(), r.ReadString()
	if err := r.Err(); err != nil {
		t.Fatalf("read PetInfo: %v", err)
	}
	return name, title
}

// petStatusTitle reads PetStatusUpdate's title string.
func petStatusTitle(t *testing.T, frame []byte) string {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePetStatusUpdate, "PetStatusUpdate")
	r := wire.NewReader(frame[1:])
	for range 5 { // type, object id, x, y, z
		r.ReadInt32()
	}
	title := r.ReadString()
	if err := r.Err(); err != nil {
		t.Fatalf("read PetStatusUpdate: %v", err)
	}
	return title
}

func mustFirstOpcode(t *testing.T, frames [][]byte, opcode byte, what string) []byte {
	t.Helper()
	f, ok := firstOpcode(frames, opcode)
	if !ok {
		t.Fatalf("no %s among % x", what, opcodes(frames))
	}
	return f
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}

// hurtAndReadStatus takes 10 HP off actor on the owner's queue and returns
// the owner's frames.
func hurtAndReadStatus(t *testing.T, srv *gameservertest.Server, client *testsupport.ScriptedClient, ownerID int32, actor *summon.Actor) [][]byte {
	t.Helper()
	owner, _ := srv.State.Player(ownerID)
	runOn(t, srv.PlayerQueue(t, ownerID), func() {
		actor.ReduceHP(10, owner.(attackable.Combatant), modelskill.Definition{})
	})
	return drainFrames(t, client)
}

// TestOwnerPetInfoTitleIsTheOwnersName pins the title the owner's PetInfo and
// PetStatusUpdate write for a pet: the owner's name, given to the summon when
// it is created, on the spawn PetInfo, the rename refresh and a status
// update.
func TestOwnerPetInfoTitleIsTheOwnersName(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	ownerName := h.character(t).CharacterName()
	if ownerName == "" {
		t.Fatal("owner has no name")
	}
	pet, burst := h.spawnWolf(t)

	if _, title := petInfoNameTitle(t, mustFirstOpcode(t, burst, serverpackets.OpcodePetInfo, "spawn PetInfo")); title != ownerName {
		t.Fatalf("spawn PetInfo title = %q, want %q", title, ownerName)
	}
	renamed := renameTo(t, h, "Fenrir")
	if name, title := petInfoNameTitle(t, mustFirstOpcode(t, renamed, serverpackets.OpcodePetInfo, "rename PetInfo")); name != "Fenrir" || title != ownerName {
		t.Fatalf("rename PetInfo name/title = %q/%q, want %q/%q", name, title, "Fenrir", ownerName)
	}
	status := hurtAndReadStatus(t, h.srv, h.client, h.ownerID, pet)
	if got := petStatusTitle(t, mustFirstOpcode(t, status, serverpackets.OpcodePetStatusUpdate, "PetStatusUpdate")); got != ownerName {
		t.Fatalf("PetStatusUpdate title = %q, want %q", got, ownerName)
	}
}

// TestOwnerServitorInfoTitleIsTheOwnersName is the same for a servitor
// summoned by skill.
func TestOwnerServitorInfoTitleIsTheOwnersName(t *testing.T) {
	t.Parallel()
	o := bootServitorOwner(t)
	obj, _ := o.srv.State.Player(o.id)
	owner, ok := obj.(interface{ CharacterName() string })
	if !ok || owner.CharacterName() == "" {
		t.Fatalf("owner %T has no name", obj)
	}
	ownerName := owner.CharacterName()

	o.client.Send(encodeRequestMagicSkillUse(killableServitorSkill))
	var servitor *summon.Actor
	o.srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := o.srv.State.Summon(o.id)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	burst := drainFrames(t, o.client)
	if _, title := petInfoNameTitle(t, mustFirstOpcode(t, burst, serverpackets.OpcodePetInfo, "servitor PetInfo")); title != ownerName {
		t.Fatalf("servitor PetInfo title = %q, want %q", title, ownerName)
	}
	status := hurtAndReadStatus(t, o.srv, o.client, o.id, servitor)
	if got := petStatusTitle(t, mustFirstOpcode(t, status, serverpackets.OpcodePetStatusUpdate, "servitor PetStatusUpdate")); got != ownerName {
		t.Fatalf("servitor PetStatusUpdate title = %q, want %q", got, ownerName)
	}
}
