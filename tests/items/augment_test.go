package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// augmentSkillHP is the maxHp the red augmentation skill's passive adds.
const augmentSkillHP = 100

// The refine rolls these scenarios script, walked through the reference's
// AugmentationData.generateRandomAugmentation for a no-grade level 46 life
// stone (grade 0, level 0):
//
// skillRolls: skill roll 1 <= 15, glow roll 50 > 0, color 0 <= 5 is red;
// red option 0 of level 0 is 14563 (AugmentationTable's skill 3256); no
// glow, block rnd(0,1)=0 gives stat12 in [1,91], drawn 3: maxHp solo of
// color 0.
//
// plainRolls: no skill (50), no glow (50), no base stat (50), color 30 <=
// 40 is blue; temp 2 puts stat34 in [8191,8281], drawn 8191: color 2 pDef
// solo; block rnd(0,1)=0 gives stat12 in [1,91], drawn 3.
var (
	skillRolls = []augmentDraw{{1, 100, 1}, {1, 100, 50}, {0, 100, 0}, {0, 0, 0}, {0, 1, 0}, {1, 91, 3}}
	plainRolls = []augmentDraw{{1, 100, 50}, {1, 100, 50}, {1, 100, 50}, {0, 100, 30}, {2, 3, 2}, {8191, 8281, 8191}, {0, 1, 0}, {1, 91, 3}}
)

const (
	skillAugmentationID = 14563<<16 | 3
	plainAugmentationID = 8191<<16 | 3
)

// augmentDraw is one scripted refine roll: the bounds it must ask for and
// the value it returns.
type augmentDraw struct{ min, max, value int }

func scriptedAugmentRoll(t *testing.T, draws []augmentDraw) augmentation.Rand {
	t.Helper()
	i := 0
	return func(lo, hi int) int {
		if i >= len(draws) {
			t.Errorf("unscripted refine roll [%d,%d]", lo, hi)
			return lo
		}
		d := draws[i]
		i++
		if lo != d.min || hi != d.max {
			t.Errorf("refine roll %d asks [%d,%d], want [%d,%d]", i, lo, hi, d.min, d.max)
		}
		return d.value
	}
}

// smith is a level 46 character holding a C-grade sword, a level 46 life
// stone, 20 Gemstone D and adena, with an item shortcut on the sword.
type smith struct {
	srv                         *gameservertest.Server
	c                           *testsupport.ScriptedClient
	objID                       int32
	sword, stone, gemstone, ade int32
}

func bootSmith(t *testing.T, level int, draws []augmentDraw, adena int32) *smith {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 2},
		{
			ID: modelskill.ID(gameservertest.AugmentRedSkillID), Level: 1, Activation: modelskill.ActivationPassive,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: augmentSkillHP}},
		},
	}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithAugmentations(gameservertest.AugmentationTable(t), nil, scriptedAugmentRoll(t, draws)),
		gameservertest.WithCharacter("Smith", level, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
	)
	s := &smith{srv: srv, c: srv.Client, objID: srv.SoleObjectID(t)}
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), s.objID, 0, expertiseSkillID, 2); err != nil {
		t.Fatalf("grant expertise: %v", err)
	}
	s.sword = srv.GiveItem(t, s.objID, gameservertest.StormbringerID, 1)
	s.stone = srv.GiveItem(t, s.objID, gameservertest.LifeStone46ID, 2)
	s.gemstone = srv.GiveItem(t, s.objID, gameservertest.GemstoneDID, 40)
	if adena > 0 {
		s.ade = srv.GiveItem(t, s.objID, item.AdenaID, adena)
	}
	if err := srv.Shortcuts.Save(context.Background(), s.objID, 0, shortcut.Shortcut{Slot: 1, Type: shortcut.Item, ID: s.sword, Level: -1, CharacterType: 1}); err != nil {
		t.Fatalf("seed sword shortcut: %v", err)
	}
	startInWorld(t, s.c)
	return s
}

// send sends payload and returns every frame it is answered with.
func (s *smith) send(t *testing.T, payload []byte) [][]byte {
	t.Helper()
	s.c.Send(payload)
	return collectUntilQuiet(t, s.c)
}

func encodeAugmentRequest(opcode uint16, values ...int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(opcode)
	for _, v := range values {
		w.WriteInt32(v)
	}
	return w.Bytes()
}

func (s *smith) refine(t *testing.T, count int32) [][]byte {
	t.Helper()
	return s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestRefine, s.sword, s.stone, s.gemstone, count))
}

// exFrame is an extended server packet's sub-opcode and int32 fields.
type exFrame struct {
	sub    uint16
	fields []int32
}

func readExFrame(t *testing.T, frame []byte) exFrame {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeExtended, "extended packet")
	r := wire.NewReader(frame[1:])
	out := exFrame{sub: r.ReadUint16()}
	for r.Remaining() >= 4 {
		out.fields = append(out.fields, r.ReadInt32())
	}
	return out
}

// assertExFrames requires frames to be exactly the extended packets want,
// in order.
func assertExFrames(t *testing.T, what string, frames [][]byte, want ...exFrame) {
	t.Helper()
	if len(frames) != len(want) {
		t.Fatalf("%s: %d frames %x, want %d", what, len(frames), opcodesOf(frames), len(want))
	}
	for i, frame := range frames {
		got := readExFrame(t, frame)
		if got.sub != want[i].sub || !equalInt32s(got.fields, want[i].fields) {
			t.Fatalf("%s: frame %d = %#x %v, want %#x %v", what, i, got.sub, got.fields, want[i].sub, want[i].fields)
		}
	}
}

func equalInt32s(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func opcodesOf(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}

// findFrame returns the index of the first frame from start whose opcode
// (and, for an extended packet, sub-opcode) matches, or -1.
func findFrame(frames [][]byte, start int, opcode byte, sub uint16) int {
	for i := start; i < len(frames); i++ {
		if frames[i][0] != opcode {
			continue
		}
		if opcode == serverpackets.OpcodeExtended && wire.NewReader(frames[i][1:]).ReadUint16() != sub {
			continue
		}
		return i
	}
	return -1
}

func mustFindFrame(t *testing.T, frames [][]byte, start int, opcode byte, sub uint16, what string) int {
	t.Helper()
	i := findFrame(frames, start, opcode, sub)
	if i < 0 {
		t.Fatalf("no %s among frames %x (from %d)", what, opcodesOf(frames), start)
	}
	return i
}

// persistedAugmentation reads the augmentations row of objectID.
func persistedAugmentation(t *testing.T, srv *gameservertest.Server, objectID int32) (item.Augmentation, bool) {
	t.Helper()
	srv.FlushPersistence(t)
	aug, ok, err := gamesql.NewAugmentationStore(srv.DB).Get(context.Background(), objectID)
	if err != nil {
		t.Fatalf("read augmentation: %v", err)
	}
	return aug, ok
}

// lastUserInfoMaxHP is the MaxHP of the last UserInfo among frames.
func lastUserInfoMaxHP(t *testing.T, frames [][]byte) int32 {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == serverpackets.OpcodeUserInfo {
			return userInfoMaxHP(t, frames[i])
		}
	}
	t.Fatalf("no UserInfo among frames %x", opcodesOf(frames))
	return 0
}

// TestAugmentRefineFlow walks the augmentation window on a worn C-grade
// sword: each confirmation step answers its own packet, a wrong gemstone
// count is refused, and the refine takes the sword off, consumes one life
// stone and 20 gemstones, augments the sword and resends its shortcut. The
// augmentation lands in the same write as the consumed materials and comes
// back on relog, worn; while worn its bonus and skill apply, and taking it
// off removes them.
func TestAugmentRefineFlow(t *testing.T) {
	t.Parallel()
	s := bootSmith(t, 46, skillRolls, 0)

	equip := s.send(t, encodeUseItem(s.sword, false))
	baseHP := lastUserInfoMaxHP(t, equip)
	s.srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, s.c)

	assertExFrames(t, "confirm target", s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestConfirmTargetItem, s.sword)),
		exFrame{serverpackets.OpcodeExConfirmVariationItem, []int32{s.sword, 1, 1}})
	assertExFrames(t, "confirm life stone", s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestConfirmRefinerItem, s.sword, s.stone)),
		exFrame{serverpackets.OpcodeExConfirmVariationRefiner, []int32{s.stone, gameservertest.LifeStone46ID, gameservertest.GemstoneDID, 20, 1}})
	wrong := s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestConfirmGemStone, s.sword, s.stone, s.gemstone, 19))
	if len(wrong) != 1 {
		t.Fatalf("19 gemstones answered %x, want one SystemMessage", opcodesOf(wrong))
	}
	assertStaticSystemMessage(t, wrong[0], serverpackets.SystemMessageGemstoneQuantityIncorrect)
	assertExFrames(t, "confirm gemstones", s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestConfirmGemStone, s.sword, s.stone, s.gemstone, 20)),
		exFrame{serverpackets.OpcodeExConfirmVariationGemstone, []int32{s.gemstone, 1, 20, 1, 1}})

	refined := s.refine(t, 20)
	// The worn sword comes off and the change is broadcast before the
	// result; its shortcut is resent carrying the new augmentation.
	unequipped := mustFindFrame(t, refined, 0, serverpackets.OpcodeUserInfo, 0, "unequip UserInfo")
	result := mustFindFrame(t, refined, unequipped, serverpackets.OpcodeExtended, serverpackets.OpcodeExVariationResult, "ExVariationResult")
	assertExFrames(t, "refine result", refined[result:result+1],
		exFrame{serverpackets.OpcodeExVariationResult, []int32{3, 14563, 1}})
	register := mustFindFrame(t, refined, result, serverpackets.OpcodeShortCutRegister, 0, "ShortCutRegister")
	r := wire.NewReader(refined[register][1:])
	for range 6 { // type, slot, object id, character type, reuse group, remaining
		r.ReadInt32()
	}
	r.ReadInt32() // reuse
	if got := r.ReadInt32(); got != skillAugmentationID {
		t.Fatalf("sword shortcut augmentation = %d, want %d", got, skillAugmentationID)
	}

	// The sword is reported twice, as the reference reports an unstackable
	// item once per change: taken off, then augmented, around the two
	// consumed stacks.
	s.srv.InventoryUpdates.Tick()
	entries := readInventoryUpdateEntries(t, s.c.Read())
	wantEntries := []inventoryEntry{
		{state: 2, objID: s.sword, itemID: gameservertest.StormbringerID, count: 1},
		{state: 2, objID: s.stone, itemID: gameservertest.LifeStone46ID, count: 1},
		{state: 2, objID: s.gemstone, itemID: gameservertest.GemstoneDID, count: 20},
		{state: 2, objID: s.sword, itemID: gameservertest.StormbringerID, count: 1},
	}
	if len(entries) != len(wantEntries) {
		t.Fatalf("InventoryUpdate entries = %+v, want %+v", entries, wantEntries)
	}
	for i, e := range entries {
		w := wantEntries[i]
		if e.state != w.state || e.objID != w.objID || e.itemID != w.itemID || e.count != w.count {
			t.Fatalf("InventoryUpdate entry %d = %+v, want %+v", i, e, w)
		}
	}

	aug, ok := persistedAugmentation(t, s.srv, s.sword)
	if !ok || aug != (item.Augmentation{Attributes: skillAugmentationID, SkillID: gameservertest.AugmentRedSkillID, SkillLevel: 1}) {
		t.Fatalf("persisted augmentation = %+v %v, want %d with skill %d", aug, ok, skillAugmentationID, gameservertest.AugmentRedSkillID)
	}
	if st := mustFindItem(t, s.srv, s.objID, s.stone); st.Count != 1 {
		t.Fatalf("persisted life stones = %d, want 1", st.Count)
	}
	if st := mustFindItem(t, s.srv, s.objID, s.gemstone); st.Count != 20 {
		t.Fatalf("persisted gemstones = %d, want 20", st.Count)
	}

	// Refining it again is refused: an item is augmented once.
	again := s.refine(t, 20)
	if len(again) != 2 {
		t.Fatalf("second refine answered %x, want the failed result and its message", opcodesOf(again))
	}
	assertExFrames(t, "second refine", again[:1], exFrame{serverpackets.OpcodeExVariationResult, []int32{0, 0, 0}})
	assertStaticSystemMessage(t, again[1], serverpackets.SystemMessageAugmentationFailedInappropriate)
	notAgain := s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestConfirmTargetItem, s.sword))
	if len(notAgain) != 1 {
		t.Fatalf("confirming an augmented sword answered %x, want one SystemMessage", opcodesOf(notAgain))
	}
	assertStaticSystemMessage(t, notAgain[0], serverpackets.SystemMessageAlreadyAugmented)

	// Worn, the augmentation adds its maxHp bonus (color 0 maxHp solo)
	// silently, then grants its passive skill, whose own maxHp shows in the
	// UserInfo its stat change sends, ahead of the SkillList naming it.
	bonusHP := int32(gameservertest.AugmentationStatValue(0, 2))
	worn := s.send(t, encodeUseItem(s.sword, false))
	first := mustFindFrame(t, worn, 0, serverpackets.OpcodeUserInfo, 0, "equip UserInfo")
	if got := userInfoMaxHP(t, worn[first]); got != baseHP+bonusHP+augmentSkillHP {
		t.Fatalf("equip UserInfo MaxHP = %d, want %d (base %d + bonus %d + skill %d)", got, baseHP+bonusHP+augmentSkillHP, baseHP, bonusHP, augmentSkillHP)
	}
	list := mustFindFrame(t, worn, first, serverpackets.OpcodeSkillList, 0, "equip SkillList")
	if !skillListHas(t, worn[list], gameservertest.AugmentRedSkillID) {
		t.Fatalf("equip SkillList lacks augmentation skill %d", gameservertest.AugmentRedSkillID)
	}

	// Taken off, the bonus leaves first, then the skill, each with its own
	// UserInfo, and the SkillList no longer names it.
	off := s.send(t, encodeUseItem(s.sword, false))
	first = mustFindFrame(t, off, 0, serverpackets.OpcodeUserInfo, 0, "unequip UserInfo")
	second := mustFindFrame(t, off, first+1, serverpackets.OpcodeUserInfo, 0, "second unequip UserInfo")
	if a, b := userInfoMaxHP(t, off[first]), userInfoMaxHP(t, off[second]); a != baseHP+augmentSkillHP || b != baseHP {
		t.Fatalf("unequip UserInfo MaxHPs = %d, %d, want %d then %d", a, b, baseHP+augmentSkillHP, baseHP)
	}
	list = mustFindFrame(t, off, second, serverpackets.OpcodeSkillList, 0, "unequip SkillList")
	if skillListHas(t, off[list], gameservertest.AugmentRedSkillID) {
		t.Fatalf("unequip SkillList still names augmentation skill %d", gameservertest.AugmentRedSkillID)
	}

	// Worn across a relog, the restored sword carries its augmentation and
	// its bonus is back in the login UserInfo.
	s.send(t, encodeUseItem(s.sword, false))
	logoutSmith(t, s)
	relogin := s.srv.DialClient(t, "player1", 1)
	s.c = relogin
	worn = startInWorld(t, relogin)
	off = s.send(t, encodeUseItem(s.sword, false))
	if on, bare := lastUserInfoMaxHP(t, worn), lastUserInfoMaxHP(t, off); on-bare != bonusHP+augmentSkillHP {
		t.Fatalf("relog UserInfo MaxHP = %d worn, %d bare; want the bonus %d and skill %d between them", on, bare, bonusHP, augmentSkillHP)
	}
	if inst := mustFindItem(t, s.srv, s.objID, s.sword); inst.Augmentation == nil || inst.Augmentation.Attributes != skillAugmentationID {
		t.Fatalf("restored sword augmentation = %+v, want %d", inst.Augmentation, skillAugmentationID)
	}
}

func logoutSmith(t *testing.T, s *smith) {
	t.Helper()
	s.c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	for {
		frame := s.c.Read()
		if frame[0] == serverpackets.OpcodeLeaveWorld {
			break
		}
	}
	s.c.ExpectClosed()
	s.srv.FlushPersistence(t)
}

// TestAugmentRefineRefusals pins the refusals: a life stone above the
// player's level answers its own message ahead of the unsuitable one, and a
// refine naming a gemstone the player does not hold fails with nothing
// consumed.
func TestAugmentRefineRefusals(t *testing.T) {
	t.Parallel()
	s := bootSmith(t, 45, nil, 0)
	low := s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestConfirmRefinerItem, s.sword, s.stone))
	if len(low) != 2 {
		t.Fatalf("level 45 life stone answered %x, want two SystemMessages", opcodesOf(low))
	}
	assertStaticSystemMessage(t, low[0], serverpackets.SystemMessageLifeStoneLevelTooHigh)
	assertStaticSystemMessage(t, low[1], serverpackets.SystemMessageNotSuitableItem)

	failed := s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestRefine, s.sword, s.stone, 999999, 20))
	if len(failed) != 2 {
		t.Fatalf("refine with no gemstone answered %x, want the failed result and its message", opcodesOf(failed))
	}
	assertExFrames(t, "refine without gemstone", failed[:1], exFrame{serverpackets.OpcodeExVariationResult, []int32{0, 0, 0}})
	assertStaticSystemMessage(t, failed[1], serverpackets.SystemMessageAugmentationFailedInappropriate)
	if st := mustFindItem(t, s.srv, s.objID, s.stone); st.Count != 2 {
		t.Fatalf("persisted life stones = %d, want 2", st.Count)
	}
}

// TestAugmentRefineRefusesDGrade pins that a D-grade weapon is refused as
// unsuitable.
func TestAugmentRefineRefusesDGrade(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithAugmentations(gameservertest.AugmentationTable(t), nil, nil),
		gameservertest.WithCharacter("Smith", 46, 0),
		gameservertest.WithWantChars(1),
	)
	objID := srv.SoleObjectID(t)
	dGrade := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, srv.Client)
	srv.Client.Send(encodeAugmentRequest(clientpackets.OpcodeRequestConfirmTargetItem, dGrade))
	frames := collectUntilQuiet(t, srv.Client)
	if len(frames) != 1 {
		t.Fatalf("D-grade sword answered %x, want one SystemMessage", opcodesOf(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNotSuitableItem)
}

// TestAugmentCancelFlow prices and removes an augmentation: the price
// follows the sword's grade and crystal count, a player short of adena is
// refused, and removal from the worn sword disarms it, takes the adena,
// clears the augmentation from the row and resends the sword's shortcut.
func TestAugmentCancelFlow(t *testing.T) {
	t.Parallel()
	const price = 95000 // C grade, 916 crystals < 1720
	s := bootSmith(t, 46, plainRolls, 50000)
	s.send(t, encodeUseItem(s.sword, false))
	s.refine(t, 20)
	s.srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, s.c)

	confirm := s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestConfirmCancelItem, s.sword))
	if len(confirm) != 1 {
		t.Fatalf("confirm cancel answered %x, want ExConfirmCancelItem", opcodesOf(confirm))
	}
	r := wire.NewReader(confirm[0][1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExConfirmCancelItem {
		t.Fatalf("confirm cancel sub-opcode = %#x", sub)
	}
	if obj, id, low, high, cost, one := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt64(), r.ReadInt32(); obj != s.sword || id != gameservertest.StormbringerID || low != 3 || high != 8191 || cost != price || one != 1 {
		t.Fatalf("ExConfirmCancelItem = %d %d %d %d %d %d", obj, id, low, high, cost, one)
	}

	poor := s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestRefineCancel, s.sword))
	if len(poor) != 2 {
		t.Fatalf("cancel without adena answered %x", opcodesOf(poor))
	}
	assertStaticSystemMessage(t, poor[0], serverpackets.SystemMessageYouNotEnoughAdena)
	assertExFrames(t, "cancel without adena", poor[1:], exFrame{serverpackets.OpcodeExVariationCancelResult, []int32{1, 0}})

	logoutSmith(t, s)
	if _, err := s.srv.DB.ExecContext(context.Background(), "UPDATE items SET count = ? WHERE object_id = ?", price+5000, s.ade); err != nil {
		t.Fatalf("raise adena: %v", err)
	}
	c := s.srv.DialClient(t, "player1", 1)
	startInWorld(t, c)
	s.c = c
	s.send(t, encodeUseItem(s.sword, false))

	cancel := s.send(t, encodeAugmentRequest(clientpackets.OpcodeRequestRefineCancel, s.sword))
	paid := mustFindFrame(t, cancel, 0, serverpackets.OpcodeSystemMessage, 0, "adena SystemMessage")
	if id := systemMessageID(t, cancel[paid]); id != serverpackets.SystemMessageS1DisappearedAdena {
		t.Fatalf("first message = %d, want S1DisappearedAdena", id)
	}
	// Disarming stops the attack, which releases the client before the
	// sword comes off.
	released := mustFindFrame(t, cancel, paid+1, serverpackets.OpcodeActionFailed, 0, "disarm ActionFailed")
	if next := findFrame(cancel, paid+1, serverpackets.OpcodeSystemMessage, 0); next < released {
		t.Fatalf("disarm message at %d ahead of its ActionFailed at %d", next, released)
	}
	disarmed := mustFindFrame(t, cancel, released+1, serverpackets.OpcodeSystemMessage, 0, "disarm SystemMessage")
	assertSystemMessageItem(t, cancel[disarmed], serverpackets.SystemMessageS1Disarmed, gameservertest.StormbringerID)
	result := mustFindFrame(t, cancel, disarmed, serverpackets.OpcodeExtended, serverpackets.OpcodeExVariationCancelResult, "ExVariationCancelResult")
	assertExFrames(t, "cancel result", cancel[result:result+1], exFrame{serverpackets.OpcodeExVariationCancelResult, []int32{1, 1}})
	register := mustFindFrame(t, cancel, result, serverpackets.OpcodeShortCutRegister, 0, "ShortCutRegister")
	removed := mustFindFrame(t, cancel, register, serverpackets.OpcodeSystemMessage, 0, "removed SystemMessage")
	assertSystemMessageItem(t, cancel[removed], serverpackets.SystemMessageAugmentationRemovedFromS1, gameservertest.StormbringerID)

	if _, ok := persistedAugmentation(t, s.srv, s.sword); ok {
		t.Fatal("augmentation row survives its removal")
	}
	if st := mustFindItem(t, s.srv, s.objID, s.ade); st.Count != 5000 {
		t.Fatalf("persisted adena = %d, want 5000", st.Count)
	}
	if inst := mustFindItem(t, s.srv, s.objID, s.sword); inst.Augmentation != nil || inst.Location != item.LocationInventory {
		t.Fatalf("sword row = %+v, want unequipped without augmentation", inst)
	}
}

// TestAugmentActiveSkillKeepsReuseAcrossReequip pins Augmentation.applyBonus
// for an active skill: cast, taken off and worn again before its reuse ends,
// the skill comes back disabled until the reuse the cast armed runs out,
// not for a fresh delay and not usable at once, and the SkillList naming it
// is followed by a SkillCoolTime carrying the remaining time.
func TestAugmentActiveSkillKeepsReuseAcrossReequip(t *testing.T) {
	t.Parallel()
	const (
		reuse     = 60 * time.Second
		skillID   = gameservertest.AugmentBlueSkillID
		worn      = 20 * time.Second // between the cast and wearing it again
		augmentID = plainAugmentationID
	)
	def := modelskill.Definition{
		ID: modelskill.ID(skillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, StaticReuse: true, ReuseDelay: int(reuse / time.Millisecond), SkillType: "HEAL", Power: 1,
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 2}, def,
	}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithAugmentations(gameservertest.AugmentationTable(t), nil, nil),
		gameservertest.WithCharacter("Smith", 46, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("pins reuse expiry on the driven clock")
	}
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, expertiseSkillID, 2); err != nil {
		t.Fatalf("grant expertise: %v", err)
	}
	sword := srv.GiveItem(t, objID, gameservertest.StormbringerID, 1)
	if err := gamesql.NewAugmentationStore(db).Create(context.Background(), sword, item.Augmentation{Attributes: augmentID, SkillID: skillID, SkillLevel: 1}); err != nil {
		t.Fatalf("seed augmentation: %v", err)
	}
	s := &smith{srv: srv, c: srv.Client, objID: objID, sword: sword}
	startInWorld(t, s.c)

	live, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("smith missing from the world")
	}
	caster, ok := live.(interface{ SkillDisabled(int32) bool })
	if !ok {
		t.Fatalf("smith %T exposes no SkillDisabled", live)
	}
	key := actorcast.ReuseKey(def)

	equipped := s.send(t, encodeUseItem(sword, false))
	list := mustFindFrame(t, equipped, 0, serverpackets.OpcodeSkillList, 0, "equip SkillList")
	if !skillListHas(t, equipped[list], skillID) {
		t.Fatalf("equip SkillList lacks augmentation skill %d", skillID)
	}
	if i := findFrame(equipped, 0, serverpackets.OpcodeSkillCoolTime, 0); i >= 0 {
		t.Fatalf("equip with no reuse running sent SkillCoolTime at %d among %x", i, opcodesOf(equipped))
	}

	s.c.Send(encodeRequestMagicSkillUse(skillID))
	srv.AdvanceUntil(t, "augmentation skill reuse armed", func() bool { return caster.SkillDisabled(key) })
	srv.Advance(t, worn)
	drainUntilQuiet(t, s.c)
	s.send(t, encodeUseItem(sword, false))

	again := s.send(t, encodeUseItem(sword, false))
	list = mustFindFrame(t, again, 0, serverpackets.OpcodeSkillList, 0, "re-equip SkillList")
	if !skillListHas(t, again[list], skillID) {
		t.Fatalf("re-equip SkillList lacks augmentation skill %d", skillID)
	}
	if list+1 >= len(again) || again[list+1][0] != serverpackets.OpcodeSkillCoolTime {
		t.Fatalf("re-equip frames %x: want SkillCoolTime right after the SkillList at %d", opcodesOf(again), list)
	}
	r := wire.NewReader(again[list+1][1:])
	found := false
	for n := r.ReadInt32(); n > 0; n-- {
		id, level, total, remaining := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if id != skillID {
			continue
		}
		found = true
		if level != 1 || total != int32(reuse/time.Second) || remaining <= 0 || remaining > int32((reuse-worn)/time.Second) {
			t.Fatalf("SkillCoolTime entry = level %d reuse %ds remaining %ds, want level 1 reuse %ds remaining at most %ds",
				level, total, remaining, int32(reuse/time.Second), int32((reuse-worn)/time.Second))
		}
	}
	if !found {
		t.Fatalf("SkillCoolTime lacks augmentation skill %d", skillID)
	}
	if !caster.SkillDisabled(key) {
		t.Fatal("augmentation skill usable at once after re-equip, want its reuse still running")
	}

	// Disabled until the cast's own reuse ends, a little under reuse-worn
	// from here (the cast took a few steps to arm it), not for a new delay.
	srv.Advance(t, reuse-worn-2*time.Second)
	if !caster.SkillDisabled(key) {
		t.Fatal("augmentation skill usable before its reuse ended")
	}
	srv.Advance(t, 2*time.Second)
	if caster.SkillDisabled(key) {
		t.Fatal("augmentation skill still disabled after its original reuse ended")
	}
}
