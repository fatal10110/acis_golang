package pets

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// curHPStatusID is StatusType.CUR_HP (StatusType.java).
const curHPStatusID = 9

// curHPFixture is the literal StatusUpdate a health-bar watcher receives
// (StatusUpdate.java:26-38): writeC(0x0e), writeD(objectId), writeD(1),
// then writeD(CUR_HP=9), writeD(hp), little-endian.
func curHPFixture(objectID, hp int32) []byte {
	out := []byte{0x0e}
	out = binary.LittleEndian.AppendUint32(out, uint32(objectID))
	out = binary.LittleEndian.AppendUint32(out, 1)
	out = binary.LittleEndian.AppendUint32(out, curHPStatusID)
	return binary.LittleEndian.AppendUint32(out, uint32(hp))
}

// statusUpdatesFor returns every StatusUpdate about objectID in frames, and
// each one's index in frames.
func statusUpdatesFor(frames [][]byte, objectID int32) (updates [][]byte, at []int) {
	for i, frame := range frames {
		if len(frame) >= 5 && frame[0] == serverpackets.OpcodeStatusUpdate &&
			int32(binary.LittleEndian.Uint32(frame[1:5])) == objectID {
			updates = append(updates, frame)
			at = append(at, i)
		}
	}
	return updates, at
}

func assertCurHPUpdates(t *testing.T, what string, got [][]byte, want ...[]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d StatusUpdate frames %x, want %d %x", what, len(got), got, len(want), want)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("%s: StatusUpdate %d = %x, want %x", what, i, got[i], want[i])
		}
	}
}

// joinBystander brings a third character into the world next to the owner.
func (h *petWorld) joinBystander(t *testing.T) *testsupport.ScriptedClient {
	t.Helper()
	h.srv.SeedCharacterFor(t, "player3", "Bystander", 1, 0)
	c := h.srv.DialClient(t, "player3", 1)
	startInWorld(t, c)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, c)
	return c
}

// selectPet has c target pet and drains the selection's own frames.
func selectPet(t *testing.T, c *testsupport.ScriptedClient, pet *summon.Actor) {
	t.Helper()
	x, y, z := pet.Position()
	c.Send(encodeAction(pet.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, c)
}

// TestSummonVitalsReachTargetingPlayersFirst pins
// SummonStatus.broadcastStatusUpdate (SummonStatus.java:50-56): a summon's
// HP or MP change first sends CUR_HP to the players targeting it (the status
// listeners Player.setTarget registers, Player.java:2463-2484) and only then
// refreshes the owner's pet window and the observers' NpcInfo. A player
// who does not target the summon gets no StatusUpdate.
//
// The bar gate needHpUpdate (CreatureStatus.java:428-454) is never
// calibrated for a summon: initializeValues (CreatureStatus.java:416-422)
// runs only for NPCs and players, so its segment width stays zero and every
// change is reported, even a half-point one inside a single 352nd of the
// wolf's 400 max HP, and an MP-only change.
func TestSummonVitalsReachTargetingPlayersFirst(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	bystander := h.joinBystander(t)
	if maxHP := int(pet.MaxHPValue()); maxHP < 352 {
		t.Fatalf("wolf max HP = %d, want at least 352 so a calibrated bar would filter", maxHP)
	}
	selectPet(t, watcher.client, pet)
	id := pet.ObjectID()

	for _, step := range []struct {
		name   string
		change func()
	}{
		{name: "half-point HP loss", change: func() { pet.SetHP(pet.HP() - 0.5) }},
		{name: "another half point", change: func() { pet.SetHP(pet.HP() - 0.5) }},
		{name: "MP-only change", change: func() { pet.ReduceMP(1) }},
		{name: "heal back to full", change: func() { pet.AddHP(pet.MaxHPValue()) }},
		{name: "MP change at full HP", change: func() { pet.ReduceMP(1) }},
	} {
		runOn(t, pet.Queue(), step.change)
		want := curHPFixture(id, int32(pet.HP()))

		frames := drainFrames(t, watcher.client)
		updates, at := statusUpdatesFor(frames, id)
		assertCurHPUpdates(t, step.name+": watcher", updates, want)
		info := frameIndex(frames, serverpackets.OpcodeNPCInfo, id)
		if info < 0 || at[0] > info {
			t.Fatalf("%s: watcher StatusUpdate at %d, NpcInfo at %d; want the StatusUpdate first", step.name, at[0], info)
		}

		frames = drainFrames(t, bystander)
		updates, _ = statusUpdatesFor(frames, id)
		assertCurHPUpdates(t, step.name+": bystander", updates)
		if n := countNPCInfoFor(frames, id); n != 1 {
			t.Fatalf("%s: bystander got %d pet NpcInfo, want 1", step.name, n)
		}

		frames = drainFrames(t, h.client)
		updates, _ = statusUpdatesFor(frames, id)
		assertCurHPUpdates(t, step.name+": owner not targeting", updates)
		if n := countOpcode(frames, serverpackets.OpcodePetStatusUpdate); n != 1 {
			t.Fatalf("%s: owner got %d PetStatusUpdate, want 1", step.name, n)
		}
	}
}

// TestSummonCurHPFramesAreOwnedPerTargeter has the owner and another player
// target the pet: both get the CUR_HP update, the owner's ahead of its pet
// window, and each frame is independently owned.
func TestSummonCurHPFramesAreOwnedPerTargeter(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	selectPet(t, watcher.client, pet)
	selectPet(t, h.client, pet)
	id := pet.ObjectID()

	runOn(t, pet.Queue(), func() { pet.SetHP(pet.HP() - 30) })
	want := curHPFixture(id, int32(pet.HP()))

	ownerFrames := drainFrames(t, h.client)
	owner, at := statusUpdatesFor(ownerFrames, id)
	assertCurHPUpdates(t, "owner targeting its pet", owner, want)
	window := slices.IndexFunc(ownerFrames, func(f []byte) bool { return f[0] == serverpackets.OpcodePetStatusUpdate })
	if window < 0 || at[0] > window {
		t.Fatalf("owner StatusUpdate at %d, PetStatusUpdate at %d; want the StatusUpdate first", at[0], window)
	}
	other, _ := statusUpdatesFor(drainFrames(t, watcher.client), id)
	assertCurHPUpdates(t, "watcher", other, want)

	owner[0][len(owner[0])-1] ^= 0xff
	assertCurHPUpdates(t, "watcher after mutating the owner's frame", other, want)
}

const balanceLifeSkill = 1335

// bootBalanceLifeOwner brings the owner in knowing a free, instant-ish
// Balance Life over its party, and calls out the wolf.
func bootBalanceLifeOwner(t *testing.T) (*petWorld, *summon.Actor) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: wolfFeedSkill, Level: 1, Feed: wolfFeedAmount},
		{
			ID: balanceLifeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetParty,
			SkillType: "BALANCE_LIFE", Radius: 1000,
			StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 0,
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv := bootPets(t, gameservertest.WithSkills(skills))
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, balanceLifeSkill, 1); err != nil {
		t.Fatalf("seed known skill %d: %v", balanceLifeSkill, err)
	}
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	return h, pet
}

// TestBalanceLifeOnPetPublishesItsStatusOnce has the owner cast Balance
// Life over itself and its wounded wolf. BalanceLife.java:65 sets each
// target's HP through CreatureStatus.setHp, whose broadcastStatusUpdate
// (CreatureStatus.java:130-162) is, for a summon, SummonStatus's: the
// targeting player gets CUR_HP, the owner one PetStatusUpdate carrying the
// balanced HP, and an observer one NpcInfo.
func TestBalanceLifeOnPetPublishesItsStatusOnce(t *testing.T) {
	t.Parallel()
	h, pet := bootBalanceLifeOwner(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	selectPet(t, watcher.client, pet)
	runOn(t, pet.Queue(), func() { pet.SetHP(pet.HP() - 200) })
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, watcher.client)

	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}
	owner := obj.(interface {
		HP() float64
		MaxHPValue() float64
	})
	ratio := (owner.HP() + pet.HP()) / (owner.MaxHPValue() + pet.MaxHPValue())
	wantHP := pet.MaxHPValue() * ratio
	if wantHP == pet.HP() {
		t.Fatalf("balanced HP %v equals the pet's current HP; the fixture must move it", wantHP)
	}

	h.client.Send(encodeRequestMagicSkillUse(balanceLifeSkill))
	h.srv.AdvanceUntil(t, "Balance Life landing on the pet", func() bool { return pet.HP() == wantHP })
	want := petVitals{hp: int32(wantHP), mp: int32(pet.MPValue())}

	var updates []petVitals
	for _, frame := range drainFrames(t, h.client) {
		if frame[0] == serverpackets.OpcodePetStatusUpdate {
			updates = append(updates, readPetStatusVitals(t, frame))
		}
	}
	if len(updates) != 1 || updates[0] != want {
		t.Fatalf("owner PetStatusUpdates = %+v, want one carrying %+v", updates, want)
	}
	frames := drainFrames(t, watcher.client)
	if n := countNPCInfoFor(frames, pet.ObjectID()); n != 1 {
		t.Fatalf("watcher got %d pet NpcInfo, want 1", n)
	}
	got, _ := statusUpdatesFor(frames, pet.ObjectID())
	assertCurHPUpdates(t, "watcher targeting the pet", got, curHPFixture(pet.ObjectID(), int32(wantHP)))
}

// TestBalanceLifeOnPlayerCasterSendsOneSelfStatus has the owner cast
// Balance Life over itself and its wounded wolf. BalanceLife.java:65 sets the
// caster's HP through CreatureStatus.setHp, whose broadcast is, for a player,
// PlayerStatus.broadcastStatusUpdate (PlayerStatus.java:408-416): one self
// StatusUpdate carrying CUR_HP at the balanced value, CUR_MP, CUR_CP and
// MAX_CP, and no further CUR_HP update after the hit.
func TestBalanceLifeOnPlayerCasterSendsOneSelfStatus(t *testing.T) {
	t.Parallel()
	h, pet := bootBalanceLifeOwner(t)
	runOn(t, pet.Queue(), func() { pet.SetHP(pet.HP() - 200) })
	drainUntilQuiet(t, h.client)

	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}
	owner := obj.(interface {
		HP() float64
		MaxHPValue() float64
	})
	ratio := (owner.HP() + pet.HP()) / (owner.MaxHPValue() + pet.MaxHPValue())
	wantHP := owner.MaxHPValue() * ratio
	if wantHP == owner.HP() {
		t.Fatalf("balanced HP %v equals the owner's current HP; the fixture must move it", wantHP)
	}

	h.client.Send(encodeRequestMagicSkillUse(balanceLifeSkill))
	h.srv.AdvanceUntil(t, "Balance Life landing on the owner", func() bool { return owner.HP() == wantHP })

	updates, _ := statusUpdatesFor(drainFrames(t, h.client), h.ownerID)
	if len(updates) != 1 {
		t.Fatalf("owner got %d self StatusUpdates %x, want one", len(updates), updates)
	}
	want := []byte{serverpackets.OpcodeStatusUpdate}
	want = binary.LittleEndian.AppendUint32(want, uint32(h.ownerID))
	want = binary.LittleEndian.AppendUint32(want, 4)
	for _, attr := range []struct {
		typ   serverpackets.StatusType
		value int
	}{
		{serverpackets.StatusCurrentHP, int(wantHP)},
		{serverpackets.StatusCurrentMP, h.srv.PlayerCurrentMP(t, h.ownerID)},
		{serverpackets.StatusCurrentCP, h.srv.PlayerCurrentCP(t, h.ownerID)},
		{serverpackets.StatusMaxCP, h.srv.PlayerMaxCP(t, h.ownerID)},
	} {
		want = binary.LittleEndian.AppendUint32(want, uint32(attr.typ))
		want = binary.LittleEndian.AppendUint32(want, uint32(int32(attr.value)))
	}
	if !bytes.Equal(updates[0], want) {
		t.Fatalf("owner self StatusUpdate = %x, want %x", updates[0], want)
	}
}

// TestPetLevelUpRefreshesTargeterBar drives a kill-reward level-up on a
// wounded wolf. PetStatus.addLevel -> PlayableStatus.addLevel
// (PlayableStatus.java:147-170) calls setMaxHpMp, whose setHp(getMaxHp())
// (CreatureStatus.java:371-384) broadcasts through
// SummonStatus.broadcastStatusUpdate: a player targeting the pet gets CUR_HP
// at the restored max ahead of the level-up animation, and a bystander gets
// none.
func TestPetLevelUpRefreshesTargeterBar(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	bystander := h.joinBystander(t)
	selectPet(t, watcher.client, pet)
	id := pet.ObjectID()
	runOn(t, pet.Queue(), func() { pet.SetHP(pet.HP() - 150) })
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, watcher.client)
	drainUntilQuiet(t, bystander)

	runOn(t, pet.Queue(), func() { pet.AddExpAndSp(wolfNextLevelExp, 0) })
	if pet.Level() != wolfNextLevel || pet.HP() != pet.MaxHPValue() {
		t.Fatalf("after level-up: level %d HP %v, want level %d at max HP %v", pet.Level(), pet.HP(), wolfNextLevel, pet.MaxHPValue())
	}

	frames := drainFrames(t, watcher.client)
	updates, at := statusUpdatesFor(frames, id)
	assertCurHPUpdates(t, "watcher targeting the pet", updates, curHPFixture(id, int32(pet.MaxHPValue())))
	_, socialAt := petSocialActions(t, frames, id)
	if socialAt < 0 || at[0] > socialAt {
		t.Fatalf("watcher StatusUpdate at %d, level-up SocialAction at %d; want the StatusUpdate first", at[0], socialAt)
	}

	updates, _ = statusUpdatesFor(drainFrames(t, bystander), id)
	assertCurHPUpdates(t, "bystander", updates)

	ownerFrames := drainFrames(t, h.client)
	var vitals []petVitals
	for _, frame := range ownerFrames {
		if frame[0] == serverpackets.OpcodePetStatusUpdate {
			vitals = append(vitals, readPetStatusVitals(t, frame))
		}
	}
	want := petVitals{hp: int32(pet.MaxHPValue()), mp: int32(pet.MaxMPValue())}
	if len(vitals) != 1 || vitals[0] != want {
		t.Fatalf("owner PetStatusUpdates = %+v, want one carrying %+v", vitals, want)
	}
}
