package pets

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: AbstractNpcInfo.SummonInfo.writeImpl (AbstractNpcInfo.java:
// 236-300) writes _summon.isInCombat() and _summon.getAbnormalEffect();
// PetInfo writes getAbnormalEffect too. Summon.isInCombat (Summon.java:
// 302-305) is its owner's attack stance, and Creature.getAbnormalEffect
// (Creature.java:944-964) ORs the crowd-control bits the live state implies
// onto the stored ones.

const abnormalStunBit = 0x000040

// summonNPCInfos returns the in-combat byte and abnormal-effect field of
// every NpcInfo of objectID in frames. The in-combat byte follows 29 fixed
// fields (116 bytes) and two flag bytes; everything after the abnormal
// field is fixed width, 42 bytes.
func summonNPCInfos(frames [][]byte, objectID int32) (inCombat []bool, abnormal []int) {
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNPCInfo || int32(binary.LittleEndian.Uint32(f[1:5])) != objectID {
			continue
		}
		inCombat = append(inCombat, f[1+118] != 0)
		abnormal = append(abnormal, int(binary.LittleEndian.Uint32(f[len(f)-46:])))
	}
	return inCombat, abnormal
}

// petInfoAbnormals returns the abnormal-effect field of every PetInfo in
// frames. Everything after it is fixed width (PetInfo.writeImpl): 14 bytes.
func petInfoAbnormals(frames [][]byte) []int {
	var out []int
	for _, f := range frames {
		if f[0] == serverpackets.OpcodePetInfo {
			out = append(out, int(binary.LittleEndian.Uint32(f[len(f)-18:])))
		}
	}
	return out
}

// enterWorldSeeing seeds a character on account and brings it into the
// world, returning every frame it gets, the NpcInfo of the summons it comes
// into sight of included.
func enterWorldSeeing(t *testing.T, srv *gameservertest.Server, account, name string) [][]byte {
	t.Helper()
	srv.SeedCharacterFor(t, account, name, 1, 0)
	c := srv.DialClient(t, account, 1)
	c.Send(encodeRequestGameStart(0))
	readUntilOpcode(t, c, serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	frames := readUntilOpcode(t, c, serverpackets.OpcodeActionFailed, "end of the EnterWorld burst")
	return append(frames, drainFrames(t, c)...)
}

// requireOneSummonInfo checks frames hold one NpcInfo of a and returns its
// in-combat byte.
func requireOneSummonInfo(t *testing.T, frames [][]byte, a *summon.Actor, who string) bool {
	t.Helper()
	inCombat, _ := summonNPCInfos(frames, a.ObjectID())
	if len(inCombat) != 1 {
		t.Fatalf("%s got %d NpcInfo of the summon, want 1", who, len(inCombat))
	}
	return inCombat[0]
}

// TestViewerSeesSummonInCombatPoseWhileOwnerFights pins #2880: a player
// coming into sight of a summon whose owner is out of combat gets its
// NpcInfo with the in-combat byte clear, and one coming into sight of it
// while its owner is in attack stance gets the byte set.
func TestViewerSeesSummonInCombatPoseWhileOwnerFights(t *testing.T) {
	t.Parallel()
	h, pet, monster := petNextToMonster(t)

	if requireOneSummonInfo(t, enterWorldSeeing(t, h.srv, "player2", "Calm"), pet, "viewer before the fight") {
		t.Fatal("viewer entering before the fight saw the pet in combat")
	}

	monster.DoAttack(t, pet)
	if !h.srv.AttackStance.InAttackStance(ownerKey{id: h.ownerID}) {
		t.Fatal("owner not in the stance tracker after its pet was hit")
	}
	if !requireOneSummonInfo(t, enterWorldSeeing(t, h.srv, "player3", "Late"), pet, "viewer during the fight") {
		t.Fatal("viewer entering while the owner fights saw the pet out of combat")
	}
}

// TestViewerSeesRevivedLeftBehindServitorInItsOwnStance pins #2880's other
// half: a servitor revived after its owner left the world holds a stance
// of its own, and a player coming into sight of it while that stance lasts
// sees it in combat.
func TestViewerSeesRevivedLeftBehindServitorInItsOwnStance(t *testing.T) {
	t.Parallel()
	l := leaveServitorDead(t)
	servitor := l.servitor
	npcResurrect(t, l.srv, servitor)
	l.assertStandsUp(t)

	x, y, z := servitor.Position()
	monster := l.srv.SpawnAttackingHostileNPCAt(t, location.Location{X: x + 20, Y: y, Z: z})
	monster.DoAttack(t, servitor)
	if !l.srv.AttackStance.InAttackStance(ownerKey{id: servitor.ObjectID()}) {
		t.Fatal("hit servitor not in the stance tracker")
	}
	if !requireOneSummonInfo(t, enterWorldSeeing(t, l.srv, "late", "Late"), servitor, "viewer during the servitor's stance") {
		t.Fatal("viewer entering during the left-behind servitor's own stance saw it out of combat")
	}
}

// TestStunnedSummonInfoCarriesStunBit pins #2852 on a summon: a stun landing
// on a pet refreshes its PetInfo for the owner and its NpcInfo for an
// observer with the stun bit set, and the refresh when it wears off clears
// it.
func TestStunnedSummonInfoCarriesStunBit(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	drainUntilQuiet(t, h.client)

	stun, err := effect.New(effect.Skill{ID: 4100, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "Stun", Count: 1, Time: 1, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(Stun): %v", err)
	}
	stun.Effector, stun.Effected = pet, pet
	runOn(t, pet.Queue(), func() { pet.EffectList().Add(stun) })
	assertSummonAbnormal(t, h, watcher.client, pet, "stun start", abnormalStunBit)

	for i := 0; slices.Contains(pet.EffectList().All(), stun); i++ {
		if i == 10 {
			t.Fatal("pet stun not worn off within 10 effect ticks")
		}
		h.srv.Advance(t, 1100*time.Millisecond)
		h.srv.TickEffects()
	}
	assertSummonAbnormal(t, h, watcher.client, pet, "stun exit", 0)
}

// assertSummonAbnormal checks every PetInfo the owner got and every NpcInfo
// of pet the watcher got carry want. Their count is not pinned here.
func assertSummonAbnormal(t *testing.T, h *petWorld, watcher *testsupport.ScriptedClient, pet *summon.Actor, what string, want int) {
	t.Helper()
	owned := petInfoAbnormals(drainFrames(t, h.client))
	if len(owned) == 0 {
		t.Fatalf("%s: owner got no PetInfo", what)
	}
	for i, got := range owned {
		if got != want {
			t.Fatalf("%s: owner PetInfo %d abnormal = %#x, want %#x", what, i, got, want)
		}
	}
	_, seen := summonNPCInfos(drainFrames(t, watcher), pet.ObjectID())
	if len(seen) == 0 {
		t.Fatalf("%s: watcher got no NpcInfo of the pet", what)
	}
	for i, got := range seen {
		if got != want {
			t.Fatalf("%s: watcher NpcInfo %d abnormal = %#x, want %#x", what, i, got, want)
		}
	}
}
