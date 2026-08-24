package network

// Summon/pet coverage whose assertions live behind internal seams that no
// single packet reaches deterministically in the e2e suites (tests/pets,
// tests/trade): wireSummonAI's cast-controller and hit-result wiring
// regressions (#1396, #1572), the item-use state gates, and the summon
// action-table dispatch resolved through AI recording doubles. Everything
// else from the deleted trade/pet unit-test files is covered by those flow
// packages or by lower-layer unit tests.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

type fakePetStoreNoSaved struct{}

func (fakePetStoreNoSaved) Get(context.Context, int32) (petmodel.State, bool, error) {
	return petmodel.State{}, false, nil
}

func (fakePetStoreNoSaved) Save(context.Context, int32, petmodel.State) error { return nil }

type fakeSummonIDs struct{ next int32 }

func (f *fakeSummonIDs) NextID() (int32, error) {
	f.next++
	return f.next, nil
}

const (
	summonTestCollarTemplateID = 91000
	summonTestWyvernTemplateID = 91001
)

func summonTestPetNPCTemplate() *npc.Template {
	return &npc.Template{
		ID: 12500, Name: "Wolf", Level: 10,
		STR: 40, CON: 43, DEX: 30, INT: 22, WIT: 20, MEN: 20,
		BaseAttackRange: 40,
		Pet: &npc.PetData{
			Food1: 2515, Food2: 2516,
			AutoFeedLimit: 55, HungryLimit: 30, UnsummonLimit: 10,
			Levels: map[int]npc.PetLevelStats{
				10: {MaxHP: 400, MaxMP: 80, PAtk: 100, PDef: 90, MAtk: 20, MDef: 40, MaxMeal: 3000, MealInNormal: 5, MealInBattle: 10},
			},
		},
		Skills: map[int]int{2046: 1},
	}
}

// newSummonTestLink builds a GameClientLink whose npcs/summonItems/petStore
// are wired for gameSummonSpawner.SpawnPet, sharing state with the caller
// so the caller can assert against it.
func newSummonTestLink(t *testing.T) (*GameClientLink, *world.State) {
	t.Helper()
	npcs := npc.NewTable([]*npc.Template{summonTestPetNPCTemplate()})
	summonItems, err := item.NewSummonItemTable([]item.SummonItem{
		{ItemID: summonTestCollarTemplateID, NPCID: 12500, SummonType: summonItemTypePet},
		{ItemID: summonTestWyvernTemplateID, NPCID: 12621, SummonType: summonItemTypeWyvern},
	})
	if err != nil {
		t.Fatalf("build summon item table: %v", err)
	}
	state := world.New()
	link := NewGameClientLink(GameClientLinkConfig{
		World:         state,
		AI:            task.NewAI(state),
		NPCs:          npcs,
		SummonItems:   summonItems,
		PetStore:      fakePetStoreNoSaved{},
		IDs:           &fakeSummonIDs{},
		Skills:        summonTestSkillTable(t),
		ItemTemplates: testItemTemplates(),
	})
	return link, state
}

// summonTestSkillTable registers SUMMON_CREATURE (2046,1) with a zero hit
// time so a test can drive its Launch/Hit phases synchronously without a
// fake clock, mirroring itemAICastSkillTable's TELEPORT fixture.
func summonTestSkillTable(t *testing.T) *skillstate.Persistence {
	t.Helper()
	store := newMemorySkillSaveStore()
	return skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2046, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
	}), store)
}

func TestUseSummonItemRejectsWyvernWhileSitting(t *testing.T) {
	link, _ := newSummonTestLink(t)
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetStanding(false)
	inst := &item.Instance{ObjectID: 502, TemplateID: summonTestWyvernTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{inst})

	if !link.useSummonItem(live, live.Inventory(), inst) {
		t.Fatal("useSummonItem returned false, want handled rejection")
	}
	if got := live.Character.MountType(); got != 0 {
		t.Fatalf("MountType() = %d, want 0", got)
	}
	testsupport.AssertOpcodeSequence(t, frames.Frames(), serverpackets.OpcodeSystemMessage)
}

func TestUseSummonItemRejectsWyvernInCombat(t *testing.T) {
	link, _ := newSummonTestLink(t)
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetInCombat(true)
	inst := &item.Instance{ObjectID: 503, TemplateID: summonTestWyvernTemplateID, OwnerID: live.ObjectID(), Count: 1, Location: item.LocationInventory}
	live.Inventory().Restore([]*item.Instance{inst})

	if !link.useSummonItem(live, live.Inventory(), inst) {
		t.Fatal("useSummonItem returned false, want handled rejection")
	}
	if got := live.Character.MountType(); got != 0 {
		t.Fatalf("MountType() = %d, want 0", got)
	}
	testsupport.AssertOpcodeSequence(t, frames.Frames(), serverpackets.OpcodeSystemMessage)
}

func TestUseSummonItemRejectsSittingOwner(t *testing.T) {
	link, _ := newSummonTestLink(t)
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetStanding(false)
	inst := &item.Instance{ObjectID: 500, TemplateID: summonTestCollarTemplateID, OwnerID: live.ObjectID()}

	if !link.useSummonItem(live, live.Inventory(), inst) {
		t.Fatal("useSummonItem returned false, want handled rejection")
	}
	if len(frames.Frames()) != 1 {
		t.Fatalf("frames = %d, want exactly one rejection", len(frames.Frames()))
	}
	assertSystemMessageIDFrame(t, frames.Frames()[0], 31)
}

// TestSummonCastControllerRecoversPanickingHook is the regression test for
// #1396: wireSummonAI built every summon's cast controller without
// SetLogger, so a panic recovered from a scheduled Launch/Hit/Finish
// callback (Controller.scheduleLocked's recover wrapper, unconditional for
// every Controller) logged through a zero-value zerolog.Logger and was
// silently discarded, matching the network/dispatch_test.go
// (TestScheduleAfterRecoversPanickingCallback) and cast/schedule_test.go
// (TestScheduleRecoversPanickingHook) recover-and-log proof pattern. It
// drives wireSummonAI's own returned controller — a compiler-checked seam,
// not a reflection-based reach into unexported AI state — so reverting
// wireSummonAI's SetLogger call fails this test.
func TestSummonCastControllerRecoversPanickingHook(t *testing.T) {
	link, state := newSummonTestLink(t)
	buf := &syncBuffer{}
	link.log = zerolog.New(buf)
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})

	spawner := &gameSummonSpawner{link: link, live: live}
	if !spawner.SpawnServitor(live.Character, modelskill.Definition{NpcID: 12500}) {
		t.Fatal("SpawnServitor returned false")
	}
	obj, ok := state.Summon(live.ObjectID())
	if !ok {
		t.Fatal("servitor not registered in world.State")
	}
	actor := obj.(*summon.Actor)

	// wireSummonAI is idempotent enough for this: re-wiring the same actor
	// replaces its AI brain and cast controller (both plain field
	// assignments) with a freshly built one carrying the same l.log,
	// giving the test direct, compiler-checked access to what SpawnServitor
	// itself installed instead of reaching into unexported AI state.
	ctrl := link.wireSummonAI(actor)
	plan, err := ctrl.Controller.Start(time.Now(), actor, modelskill.Definition{ID: 1, Level: 1, HitTime: 0})
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	ctrl.Controller.Schedule(plan, actorcast.Hooks{Launch: func() bool { panic("boom") }})

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(buf.String(), "boom") {
		if time.Now().After(deadline) {
			t.Fatalf("panic was not recovered and logged, got: %s", buf.String())
		}
		time.Sleep(time.Millisecond)
	}
}

// TestWireSummonAIForwardsHitResultToOwner is the regression test for #1572:
// AIController's Hit hook discarded ApplyResolvedEffectsResult's return
// value, so a summon's failed-skill roll never reached the owner. Summon.java
// forwards every packet to the owner (Summon.sendPacket, base
// Creature.sendPacket a no-op) — wireSummonAI's OnHitResult wiring is the Go
// equivalent, and this drives it directly to prove the forward actually
// happens rather than re-testing sendSkillHandlerResult's own encoding
// (already covered by magic_skill_test.go).
func TestWireSummonAIForwardsHitResultToOwner(t *testing.T) {
	link, state := newSummonTestLink(t)
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.AddPlayer(live)

	spawner := &gameSummonSpawner{link: link, live: live}
	if !spawner.SpawnServitor(live.Character, modelskill.Definition{NpcID: 12500}) {
		t.Fatal("SpawnServitor returned false")
	}
	obj, ok := state.Summon(live.ObjectID())
	if !ok {
		t.Fatal("servitor not registered in world.State")
	}
	actor := obj.(*summon.Actor)

	ctrl := link.wireSummonAI(actor)
	if ctrl.OnHitResult == nil {
		t.Fatal("OnHitResult not wired, want a summon caster to forward Hit results to its owner")
	}

	testsupport.ResetCapture(frames)
	ctrl.OnHitResult(actorcast.EffectResult{AttackFailed: 1})

	if len(frames.Frames()) != 1 {
		t.Fatalf("owner frames = %d, want 1", len(frames.Frames()))
	}
	assertStaticSystemMessageFrame(t, frames.Frames()[0], serverpackets.SystemMessageAttackFailed)
}

// TestWireSummonAIOwnerForwardDropsPlayerGatedCategories is the regression
// test for the pr-reviews/1719 finding on #1572: S1_DODGES_ATTACK and
// S1_PERFORMING_COUNTERATTACK (Blow.java:46-47,88-89) and the generic
// per-effect resisted message (L2Skill.java:1196-1197) are gated
// `instanceof Player` on the caster/effector in the reference and never fire
// at all for a Summon — forwarding the raw EffectResult to the owner would
// send the owner messages Java never sends for these categories. Only
// AttackFailed and Lethals (both unconditional `sendPacket` calls in the
// reference) may reach the owner.
func TestWireSummonAIOwnerForwardDropsPlayerGatedCategories(t *testing.T) {
	link, state := newSummonTestLink(t)
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	state.AddPlayer(live)

	spawner := &gameSummonSpawner{link: link, live: live}
	if !spawner.SpawnServitor(live.Character, modelskill.Definition{NpcID: 12500}) {
		t.Fatal("SpawnServitor returned false")
	}
	obj, ok := state.Summon(live.ObjectID())
	if !ok {
		t.Fatal("servitor not registered in world.State")
	}
	actor := obj.(*summon.Actor)

	ctrl := link.wireSummonAI(actor)
	testsupport.ResetCapture(frames)
	ctrl.OnHitResult(actorcast.EffectResult{
		Dodges:         []handlerskill.Dodge{{AttackerID: actor.ObjectID(), DefenderID: live.ObjectID()}},
		Counterattacks: []handlerskill.Counterattack{{AttackerID: actor.ObjectID(), DefenderID: live.ObjectID()}},
		Resisted:       []handlerskill.Resisted{{TargetName: "Target", SkillID: 1, SkillLevel: 1}},
	})

	if len(frames.Frames()) != 0 {
		t.Fatalf("owner frames = %d, want 0 (Dodges/Counterattacks/Resisted are Player-gated in Java and must not reach the owner)", len(frames.Frames()))
	}
}

func TestGameClientLinkSummonSkillUseResolvesTargetKindAndDispatches(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 300)
	state.Spawn(hostile, 100, 0, 0, 0)
	live.SetTargetTracked(hostile)

	liveSummon := summon.NewServitor(summon.ServitorConfig{
		ObjectID: 500, Owner: live, Level: 40,
		Skills: map[int]int{4259: 1, 4378: 1, 4139: 8},
	})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	gcl := &GameClientLink{world: state}

	// Action 36 (Soulless - Toxic Smoke) targets the clicked target.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 36}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 1 || brain.casts[0] != hostile.ObjectID() {
		t.Fatalf("AI casts = %v, want clicked target %d", brain.casts, hostile.ObjectID())
	}

	// Action 42 (Kai the Cat - Self Damage Shield) targets the owner.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 42}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 2 || brain.casts[1] != live.ObjectID() {
		t.Fatalf("AI casts = %v, want owner target %d", brain.casts, live.ObjectID())
	}

	// Action 1001 (Sin Eater - Ultimate Bombastic Buster) targets the
	// summon itself.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 1001}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 3 || brain.casts[2] != liveSummon.ObjectID() {
		t.Fatalf("AI casts = %v, want self target %d", brain.casts, liveSummon.ObjectID())
	}
}

func TestGameClientLinkSinEaterSkillUseBroadcastsFlavorLine(t *testing.T) {
	if got, want := sinEaterActionStrings, [4]string{
		"special skill? Abuses in this kind of place, can turn blood Knots...!",
		"Hey! Brother! What do you anticipate to me?",
		"shouts ha! Flap! Flap! Response?",
		", has not hit...!",
	}; got != want {
		t.Fatalf("Sin Eater flavor strings = %q, want %q", got, want)
	}

	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	liveSummon := summon.NewPet(summon.PetConfig{
		ObjectID: 500, Owner: live, NPCID: 12564, Level: live.LevelValue(),
		Skills: map[int]int{4139: 1},
		Roll:   func(int) int { return 0 },
	})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{world: state}
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 1001}) {
		t.Fatal("handleSummonActionUse returned false for Sin Eater skill action")
	}
	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string([]byte{serverpackets.OpcodeNpcSay, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("Sin Eater skill-use opcodes = %x, want NpcSay then ActionFailed", got)
	}
}

func TestGameClientLinkSinEaterSkillUseMissedFlavorRollDoesNotBroadcast(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	liveSummon := summon.NewPet(summon.PetConfig{
		ObjectID: 500, Owner: live, NPCID: 12564, Level: live.LevelValue(),
		Skills: map[int]int{4139: 1},
		Roll:   func(int) int { return 10 },
	})
	liveSummon.SetAI(&recordingNetworkSummonAI{})
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})
	testsupport.ResetCapture(frames)

	gcl := &GameClientLink{world: state}
	gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 1001})
	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("missed Sin Eater flavor-roll opcodes = %x, want ActionFailed only", got)
	}
}

func TestGameClientLinkSummonSkillUsePetBeyondLevelGapIsBlocked(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)
	live.SetTargetTracked(live)

	livePet := summon.NewPet(summon.PetConfig{
		ObjectID: 500, Owner: live, Level: live.LevelValue() + 21,
		Skills: map[int]int{4259: 1},
	})
	brain := &recordingNetworkSummonAI{}
	livePet.SetAI(brain)
	summon.SpawnBesideOwner(state, livePet, live, location.Location{})

	gcl := &GameClientLink{world: state}
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 36}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 0 {
		t.Fatalf("AI casts = %v, want none for a pet beyond the level gap", brain.casts)
	}
}

func TestGameClientLinkSummonSkillUseUnmappedActionFallsThrough(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	gcl := &GameClientLink{world: state}
	if gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 9999}) {
		t.Fatal("handleSummonActionUse = true for an action id with no command or skill mapping")
	}
}

func TestGameClientLinkSummonSkillUseDoorOnlyActionNeverDispatchesYet(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	hostile := newTestHostileNPC(t, 300)
	state.Spawn(hostile, 100, 0, 0, 0)
	live.SetTargetTracked(hostile)

	liveSummon := summon.NewServitor(summon.ServitorConfig{
		ObjectID: 500, Owner: live, Level: 40,
		Skills: map[int]int{4079: 1},
	})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	gcl := &GameClientLink{world: state}
	// Action 1000 (Siege Golem - Siege Hammer) requires a Door target; no
	// Door world-object type exists yet, so it must never dispatch.
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 1000}) {
		t.Fatal("handleSummonActionUse returned false for a mapped skill action")
	}
	if len(brain.casts) != 0 {
		t.Fatalf("AI casts = %v, want none for a door-only action with no Door target", brain.casts)
	}
}

// TestGameClientLinkSummonActionUseAttackRequiresForceOrCtrl is the
// regression test for the review finding that summonTargetAttackable used a
// plain AttackableBy check, dispatching ATTACK for any living player target
// (party members included) where Java's RequestActionUse.java:177 requires
// AttackableWithoutForceBy or an explicit Ctrl-press.
func TestGameClientLinkSummonActionUseAttackRequiresForceOrCtrl(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	// A party-member-like target: attackable with force (Ctrl) only, not
	// attackable without it.
	partyMember := &summonActionAttackTarget{id: 301, attackableWith: true}
	state.Spawn(partyMember, 150, 0, 0, 0)
	live.SetTargetTracked(partyMember)

	liveSummon := summon.NewServitor(summon.ServitorConfig{ObjectID: 500, Owner: live, Level: 40})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	gcl := &GameClientLink{world: state}

	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16}) {
		t.Fatal("handleSummonActionUse returned false for a summon attack command")
	}
	if len(brain.attacks) != 0 {
		t.Fatalf("AI attacks = %v without Ctrl pressed, want none (party member requires force)", brain.attacks)
	}
	if len(brain.follows) != 1 || brain.follows[0] != partyMember.ObjectID() {
		t.Fatalf("AI follows = %v without Ctrl pressed, want a follow onto the party member", brain.follows)
	}

	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16, CtrlPressed: true}) {
		t.Fatal("handleSummonActionUse returned false for a Ctrl-pressed summon attack command")
	}
	if len(brain.attacks) != 1 || brain.attacks[0] != partyMember.ObjectID() {
		t.Fatalf("AI attacks = %v with Ctrl pressed, want an attack onto the party member", brain.attacks)
	}
}

// TestGameClientLinkSummonActionUseAttackIgnoresFakeDeathButHonorsTrueDeath
// is the regression test for the review finding that the attack command's
// dead-target gate used AlikeDead() (fake death included) where Java's
// RequestActionUse.java:155-156 checks isDead() only ("Fake Death is
// handled elsewhere (attack task)").
func TestGameClientLinkSummonActionUseAttackIgnoresFakeDeathButHonorsTrueDeath(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 100, frames)
	state.Spawn(live, 0, 0, 0, 0)

	fakeDead := &summonActionAttackTarget{id: 301, attackableWithoutForce: true, alikeDead: true}
	state.Spawn(fakeDead, 150, 0, 0, 0)
	live.SetTargetTracked(fakeDead)

	liveSummon := summon.NewServitor(summon.ServitorConfig{ObjectID: 500, Owner: live, Level: 40})
	brain := &recordingNetworkSummonAI{}
	liveSummon.SetAI(brain)
	summon.SpawnBesideOwner(state, liveSummon, live, location.Location{})

	gcl := &GameClientLink{world: state}
	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16}) {
		t.Fatal("handleSummonActionUse returned false for a summon attack command")
	}
	if len(brain.attacks) != 1 || brain.attacks[0] != fakeDead.ObjectID() {
		t.Fatalf("AI attacks = %v against a fake-dead target, want the swing to proceed", brain.attacks)
	}

	trueDead := &summonActionAttackTarget{id: 302, attackableWithoutForce: true, alikeDead: true, trueDead: true}
	state.Spawn(trueDead, 150, 0, 0, 0)
	live.SetTargetTracked(trueDead)

	if !gcl.handleSummonActionUse(context.Background(), live, clientpackets.RequestActionUse{ActionID: 16}) {
		t.Fatal("handleSummonActionUse returned false for a summon attack command")
	}
	if len(brain.attacks) != 1 {
		t.Fatalf("AI attacks = %v against a truly dead target, want the command rejected", brain.attacks)
	}
}

type summonActionAttackTarget struct {
	world.Presence
	id                     int32
	alikeDead, trueDead    bool
	attackableWith         bool
	attackableWithoutForce bool
}

func (c *summonActionAttackTarget) ObjectID() int32  { return c.id }
func (c *summonActionAttackTarget) SiegeGuard() bool { return false }
func (c *summonActionAttackTarget) AlikeDead() bool  { return c.alikeDead }
func (c *summonActionAttackTarget) Dead() bool       { return c.trueDead }
func (c *summonActionAttackTarget) AttackableBy(target.Creature) bool {
	return c.attackableWith
}
func (c *summonActionAttackTarget) AttackableWithoutForceBy(target.Creature) bool {
	return c.attackableWithoutForce
}

type recordingNetworkSummonAI struct {
	attacks []int32
	follows []int32
	idles   int
	casts   []int32
}

func (a *recordingNetworkSummonAI) TryToAttack(target attackable.Combatant) bool {
	a.attacks = append(a.attacks, target.ObjectID())
	return true
}

func (a *recordingNetworkSummonAI) TryToFollow(target attackable.Combatant) bool {
	a.follows = append(a.follows, target.ObjectID())
	return true
}

func (a *recordingNetworkSummonAI) TryToIdle() {
	a.idles++
}

func (a *recordingNetworkSummonAI) TryToCast(target attackable.Combatant, ref modelskill.Ref) bool {
	a.casts = append(a.casts, target.ObjectID())
	return true
}

func (a *recordingNetworkSummonAI) AttackingNow() bool { return false }
