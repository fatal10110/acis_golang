// Package augment runs weapon augmentation at a blacksmith: checking a
// weapon, life stone and gemstones, refining the weapon with a new
// augmentation, and pricing and removing one, without knowing how clients
// are told.
package augment

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Message is a system message an augmentation step answers with.
type Message uint8

const (
	// MessageWhileOperating refuses a player running a private store or
	// workshop.
	MessageWhileOperating Message = iota + 1
	// MessageWhileTrading refuses a player tied up in a trade.
	MessageWhileTrading
	// MessageWhileDead refuses a dead player.
	MessageWhileDead
	// MessageWhileParalyzed refuses a paralyzed player.
	MessageWhileParalyzed
	// MessageWhileFishing refuses a fishing player.
	MessageWhileFishing
	// MessageWhileSitting refuses a sitting player.
	MessageWhileSitting
	// MessageLifeStoneLevelTooHigh refuses a life stone above the player's
	// level.
	MessageLifeStoneLevelTooHigh
	// MessageNotSuitable refuses an item the step cannot take.
	MessageNotSuitable
	// MessageAlreadyAugmented refuses a weapon that already carries an
	// augmentation.
	MessageAlreadyAugmented
	// MessageGemstoneQuantityIncorrect refuses a gemstone count other than
	// the one the weapon's grade asks for.
	MessageGemstoneQuantityIncorrect
	// MessageRemovalNeedsAugmentedItem refuses removal from an item that
	// carries no augmentation.
	MessageRemovalNeedsAugmentedItem
)

// State is the player state an augmentation step is gated on, read on the
// player's own queue.
type State struct {
	Level        int
	Operating    bool
	Trading      bool
	Dead         bool
	Paralyzed    bool
	Fishing      bool
	Sitting      bool
	CursedWeapon bool
}

// playerRefusal is the first state that keeps the player from augmenting:
// its message, or ok false with no message for a cursed weapon.
func (s State) playerRefusal() (msgs []Message, ok bool) {
	switch {
	case s.Operating:
		return []Message{MessageWhileOperating}, false
	case s.Trading:
		return []Message{MessageWhileTrading}, false
	case s.Dead:
		return []Message{MessageWhileDead}, false
	case s.Paralyzed:
		return []Message{MessageWhileParalyzed}, false
	case s.Fishing:
		return []Message{MessageWhileFishing}, false
	case s.Sitting:
		return []Message{MessageWhileSitting}, false
	case s.CursedWeapon:
		return nil, false
	}
	return nil, true
}

// Service performs augmentation steps on an inventory.
type Service struct {
	table   *augmentation.Table
	chances augmentation.Chances
	rnd     augmentation.Rand
	// skillLoaded reports whether a skill option's skill is in the skill
	// table: an augmentation keeps no skill it cannot resolve.
	skillLoaded func(id int32, level int) bool
}

// NewService returns an augmentation service rolling new augmentations
// from table with chances, drawing from rnd. skillLoaded tells whether a
// rolled skill exists.
func NewService(table *augmentation.Table, chances augmentation.Chances, rnd augmentation.Rand, skillLoaded func(id int32, level int) bool) *Service {
	return &Service{table: table, chances: chances, rnd: rnd, skillLoaded: skillLoaded}
}

// Check is the outcome of one confirmation step: the messages it answers
// with, in order, and whether the step accepted what it was shown.
type Check struct {
	Messages []Message
	OK       bool
}

// targetValid reports whether the weapon target, owned by playerID, can be
// augmented by a player in st, with the state's refusal message.
func targetValid(st State, playerID int32, target *item.Instance, tmpl *item.Template) ([]Message, bool) {
	if target == nil || tmpl == nil {
		return nil, false
	}
	msgs, ok := st.playerRefusal()
	if !ok {
		return msgs, false
	}
	s := target.Snapshot()
	if s.OwnerID != playerID || s.Augmentation != nil || tmpl.HeroItem() || target.ShadowItem(tmpl) || tmpl.Crystal < item.CrystalC {
		return nil, false
	}
	if s.Location != item.LocationInventory && s.Location != item.LocationPaperdoll {
		return nil, false
	}
	if tmpl.Weapon == nil {
		return nil, false
	}
	return nil, tmpl.Weapon.Type != item.WeaponNone && tmpl.Weapon.Type != item.WeaponFishingRod
}

// lifeStoneValid is targetValid extended to the life stone refiner.
func lifeStoneValid(st State, playerID int32, target *item.Instance, tmpl *item.Template, refiner *item.Instance) ([]Message, augmentation.LifeStone, bool) {
	if refiner == nil {
		return nil, augmentation.LifeStone{}, false
	}
	msgs, ok := targetValid(st, playerID, target, tmpl)
	if !ok {
		return msgs, augmentation.LifeStone{}, false
	}
	s := refiner.Snapshot()
	if s.OwnerID != playerID || s.Location != item.LocationInventory {
		return nil, augmentation.LifeStone{}, false
	}
	ls, ok := augmentation.LifeStoneByItemID(s.TemplateID)
	if !ok {
		return nil, augmentation.LifeStone{}, false
	}
	if st.Level < ls.PlayerLevel() {
		return []Message{MessageLifeStoneLevelTooHigh}, ls, false
	}
	return nil, ls, true
}

// gemstoneValid is lifeStoneValid extended to the gemstone stack.
func gemstoneValid(st State, playerID int32, target *item.Instance, tmpl *item.Template, refiner, gemstone *item.Instance) ([]Message, augmentation.LifeStone, bool) {
	msgs, ls, ok := lifeStoneValid(st, playerID, target, tmpl, refiner)
	if !ok {
		return msgs, ls, false
	}
	s := gemstone.Snapshot()
	if s.OwnerID != playerID || s.Location != item.LocationInventory {
		return nil, ls, false
	}
	if tmpl.Crystal.GemstoneID() != s.TemplateID {
		return nil, ls, false
	}
	return nil, ls, s.Count >= tmpl.Crystal.GemstoneCount()
}

// ConfirmTarget checks the weapon objectID, held in inv, as a target. A
// weapon not in inv answers nothing at all.
func (s *Service) ConfirmTarget(st State, playerID int32, inv *itemcontainer.Inventory, objectID int32) Check {
	target := inv.ItemByObjectID(objectID)
	if target == nil {
		return Check{}
	}
	tmpl, _ := inv.Templates().Get(target.TemplateID)
	msgs, ok := targetValid(st, playerID, target, tmpl)
	if ok {
		return Check{OK: true}
	}
	if target.Augmented() {
		return Check{Messages: append(msgs, MessageAlreadyAugmented)}
	}
	return Check{Messages: append(msgs, MessageNotSuitable)}
}

// RefinerCheck is ConfirmRefiner's outcome: on success, the life stone and
// the gemstones the weapon's grade asks for.
type RefinerCheck struct {
	Check
	LifeStoneItemID int32
	GemstoneItemID  int32
	GemstoneCount   int
}

// ConfirmRefiner checks the life stone refinerID against the target
// targetID, both held in inv. Either one missing answers nothing at all.
func (s *Service) ConfirmRefiner(st State, playerID int32, inv *itemcontainer.Inventory, targetID, refinerID int32) RefinerCheck {
	target := inv.ItemByObjectID(targetID)
	if target == nil {
		return RefinerCheck{}
	}
	refiner := inv.ItemByObjectID(refinerID)
	if refiner == nil {
		return RefinerCheck{}
	}
	tmpl, _ := inv.Templates().Get(target.TemplateID)
	msgs, _, ok := lifeStoneValid(st, playerID, target, tmpl, refiner)
	if !ok {
		return RefinerCheck{Check: Check{Messages: append(msgs, MessageNotSuitable)}}
	}
	return RefinerCheck{
		Check:           Check{OK: true},
		LifeStoneItemID: refiner.TemplateID,
		GemstoneItemID:  tmpl.Crystal.GemstoneID(),
		GemstoneCount:   tmpl.Crystal.GemstoneCount(),
	}
}

// ConfirmGemstone checks count gemstones of the stack gemstoneID for the
// target and life stone, all held in inv. Any of the three missing answers
// nothing at all.
func (s *Service) ConfirmGemstone(st State, playerID int32, inv *itemcontainer.Inventory, targetID, refinerID, gemstoneID int32, count int) Check {
	target, refiner, gemstone := inv.ItemByObjectID(targetID), inv.ItemByObjectID(refinerID), inv.ItemByObjectID(gemstoneID)
	if target == nil || refiner == nil || gemstone == nil {
		return Check{}
	}
	tmpl, _ := inv.Templates().Get(target.TemplateID)
	msgs, _, ok := gemstoneValid(st, playerID, target, tmpl, refiner, gemstone)
	if !ok {
		return Check{Messages: append(msgs, MessageNotSuitable)}
	}
	if count != tmpl.Crystal.GemstoneCount() {
		return Check{Messages: []Message{MessageGemstoneQuantityIncorrect}}
	}
	return Check{OK: true}
}

// RefineRequest is one refine attempt by PlayerID on items held in Inv.
type RefineRequest struct {
	State      State
	PlayerID   int32
	Inv        *itemcontainer.Inventory
	TargetID   int32
	RefinerID  int32
	GemstoneID int32
	Count      int
}

// Refined is a refine attempt's outcome, in the order it happened.
type Refined struct {
	// Messages are the state refusals the checks answered with.
	Messages []Message
	// Unequipped lists what taking the worn target off the paperdoll
	// changed, before the materials were consumed.
	Unequipped []*item.Instance
	// Failed reports that the attempt failed; nothing was augmented.
	Failed bool
	// Target is the refined weapon and Augmentation what it now carries.
	Target       *item.Instance
	Augmentation item.Augmentation
	// Persist lists the item rows the attempt changed.
	Persist []inventory.Persist
}

// Refine augments the target with a new augmentation rolled for the life
// stone, consuming one life stone and the gemstones the weapon's grade
// asks for. A worn target comes off the paperdoll first.
func (s *Service) Refine(req RefineRequest) Refined {
	inv := req.Inv
	target, refiner, gemstone := inv.ItemByObjectID(req.TargetID), inv.ItemByObjectID(req.RefinerID), inv.ItemByObjectID(req.GemstoneID)
	if target == nil || refiner == nil || gemstone == nil {
		return Refined{Failed: true}
	}
	tmpl, _ := inv.Templates().Get(target.TemplateID)
	msgs, ls, ok := gemstoneValid(req.State, req.PlayerID, target, tmpl, refiner, gemstone)
	if !ok || req.Count != tmpl.Crystal.GemstoneCount() {
		return Refined{Messages: msgs, Failed: true}
	}

	out := Refined{}
	if target.Equipped() {
		out.Unequipped = inv.UnequipItem(target)
	}
	out.Persist = append(out.Persist, inventory.Update(target))
	refinerOwner := refiner.Snapshot().OwnerID
	consumed := inv.DestroyItem(refiner, 1)
	if consumed == nil {
		out.Failed = true
		return out
	}
	out.Persist = append(out.Persist, inventory.DestroyedOrUpdated(refinerOwner, consumed))
	gemstoneOwner := gemstone.Snapshot().OwnerID
	consumed = inv.DestroyItem(gemstone, req.Count)
	if consumed == nil {
		out.Failed = true
		return out
	}
	out.Persist = append(out.Persist, inventory.DestroyedOrUpdated(gemstoneOwner, consumed))

	rolled := s.table.Generate(ls.Level, ls.Grade, s.chances, s.rnd)
	aug := item.Augmentation{Attributes: rolled.ID}
	if rolled.Skill != nil && s.skillLoaded != nil && s.skillLoaded(rolled.Skill.SkillID, rolled.Skill.SkillLevel) {
		aug.SkillID, aug.SkillLevel = rolled.Skill.SkillID, int32(rolled.Skill.SkillLevel)
	}
	if !inv.SetAugmentation(target, aug) {
		out.Failed = true
		return out
	}
	out.Target, out.Augmentation = target, aug
	return out
}

// CancelPrice is the adena removing the augmentation from a weapon of tmpl
// at enchantLevel costs, by grade and crystal count. ok is false below C
// grade, which cannot carry one.
func CancelPrice(tmpl *item.Template, enchantLevel int) (price int, ok bool) {
	if tmpl == nil {
		return 0, false
	}
	crystals := tmpl.CrystalCountAt(enchantLevel)
	switch tmpl.Crystal {
	case item.CrystalC:
		switch {
		case crystals < 1720:
			return 95000, true
		case crystals < 2452:
			return 150000, true
		default:
			return 210000, true
		}
	case item.CrystalB:
		if crystals < 1746 {
			return 240000, true
		}
		return 270000, true
	case item.CrystalA:
		switch {
		case crystals < 2160:
			return 330000, true
		case crystals < 2824:
			return 390000, true
		default:
			return 420000, true
		}
	case item.CrystalS:
		return 480000, true
	}
	return 0, false
}

// CancelCheck is ConfirmCancel's outcome: on success, the item and the
// price of removing its augmentation.
type CancelCheck struct {
	Check
	Item  *item.Instance
	Price int
}

// ConfirmCancel prices removing the augmentation of objectID, held in inv
// and owned by playerID. An item not in inv, owned by someone else, or
// below C grade answers nothing at all.
func ConfirmCancel(playerID int32, inv *itemcontainer.Inventory, objectID int32) CancelCheck {
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return CancelCheck{}
	}
	s := inst.Snapshot()
	if s.OwnerID != playerID {
		return CancelCheck{}
	}
	if s.Augmentation == nil {
		return CancelCheck{Check: Check{Messages: []Message{MessageRemovalNeedsAugmentedItem}}}
	}
	tmpl, _ := inv.Templates().Get(s.TemplateID)
	price, ok := CancelPrice(tmpl, s.EnchantLevel)
	if !ok {
		return CancelCheck{}
	}
	return CancelCheck{Check: Check{OK: true}, Item: inst, Price: price}
}
