package enchant

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

const (
	chanceMagicWeapon       = 0.4
	chanceMagicWeapon15Plus = 0.2
	chanceWeapon            = 0.7
	chanceWeapon15Plus      = 0.35
	chanceArmor             = 0.66
	safeMax                 = 3
	safeMaxFull             = 4
	maxWeapon               = 0
	maxArmor                = 0
)

type scroll struct {
	weapon  bool
	blessed bool
	grade   item.CrystalType
}

var scrolls = map[int32]scroll{
	729:  {weapon: true, grade: item.CrystalA},
	947:  {weapon: true, grade: item.CrystalB},
	951:  {weapon: true, grade: item.CrystalC},
	955:  {weapon: true, grade: item.CrystalD},
	959:  {weapon: true, grade: item.CrystalS},
	730:  {grade: item.CrystalA},
	948:  {grade: item.CrystalB},
	952:  {grade: item.CrystalC},
	956:  {grade: item.CrystalD},
	960:  {grade: item.CrystalS},
	6569: {weapon: true, blessed: true, grade: item.CrystalA},
	6571: {weapon: true, blessed: true, grade: item.CrystalB},
	6573: {weapon: true, blessed: true, grade: item.CrystalC},
	6575: {weapon: true, blessed: true, grade: item.CrystalD},
	6577: {weapon: true, blessed: true, grade: item.CrystalS},
	6570: {blessed: true, grade: item.CrystalA},
	6572: {blessed: true, grade: item.CrystalB},
	6574: {blessed: true, grade: item.CrystalC},
	6576: {blessed: true, grade: item.CrystalD},
	6578: {blessed: true, grade: item.CrystalS},
	731:  {weapon: true, grade: item.CrystalA},
	949:  {weapon: true, grade: item.CrystalB},
	953:  {weapon: true, grade: item.CrystalC},
	957:  {weapon: true, grade: item.CrystalD},
	961:  {weapon: true, grade: item.CrystalS},
	732:  {grade: item.CrystalA},
	950:  {grade: item.CrystalB},
	954:  {grade: item.CrystalC},
	958:  {grade: item.CrystalD},
	962:  {grade: item.CrystalS},
}

// StepKind identifies one owner-visible step produced by an enchant workflow.
type StepKind uint8

const (
	// StepSystemMessage means Message should be shown to the player.
	StepSystemMessage StepKind = iota
	// StepEnchantResult means EnchantResult should be sent.
	StepEnchantResult
	// StepInventoryUpdate means the changed inventory should be sent.
	StepInventoryUpdate
	// StepBroadcastEquipment means other players need equipment refreshes.
	StepBroadcastEquipment
)

// MessageCode identifies a system-message template without depending on packet code.
type MessageCode uint8

const (
	MessageSelectItemToEnchant MessageCode = iota
	MessageEnchantScrollCancelled
	MessageInappropriateEnchantCondition
	MessageNotEnoughItems
	MessageS1SuccessfullyEnchanted
	MessageS1S2SuccessfullyEnchanted
	MessageBlessedEnchantFailed
	MessageEarnedS2S1S
	MessageEnchantmentFailedS1S2Evaporated
	MessageEnchantmentFailedS1Evaporated
)

// ResultCode identifies the enchant result packet payload.
type ResultCode uint8

const (
	// ResultCancelled means the enchant was canceled.
	ResultCancelled ResultCode = iota
	// ResultSuccess means the target was enchanted successfully.
	ResultSuccess
	// ResultUnsuccess means the enchant failed without breaking the item.
	ResultUnsuccess
	// ResultBrokenNoCrystals means the target broke without a crystal reward.
	ResultBrokenNoCrystals
	// ResultBrokenWithCrystals means the target broke and crystals were possible.
	ResultBrokenWithCrystals
)

// Message carries one system-message payload.
type Message struct {
	Code   MessageCode
	ItemID int32
	Number int32
}

// Step is one ordered client-visible enchant outcome.
type Step struct {
	Kind          StepKind
	Message       Message
	EnchantResult ResultCode
}

// Result carries ordered owner-visible outcomes plus persistence actions.
type Result struct {
	Steps   []Step
	Persist []inventory.Persist
}

// UseScrollResult carries the selected scroll item id and whether selection was newly opened.
type UseScrollResult struct {
	ScrollItemID int32
	FirstSelect  bool
}

// Service performs enchant mutations without knowing how clients are notified.
type Service struct {
	state *State
	ids   inventory.IDAllocator
	roll  func() float64
}

// NewService returns an enchant workflow service.
func NewService(state *State, ids inventory.IDAllocator, roll func() float64) *Service {
	if state == nil {
		state = NewState()
	}
	if roll == nil {
		roll = func() float64 { return rnd.GetFloat(1) }
	}
	return &Service{state: state, ids: ids, roll: roll}
}

// UseScroll selects an enchant scroll for playerID.
func (s *Service) UseScroll(playerID int32, inst *item.Instance) (UseScrollResult, bool) {
	if inst == nil {
		return UseScrollResult{}, false
	}
	if _, ok := scrolls[inst.TemplateID]; !ok {
		return UseScrollResult{}, false
	}
	return UseScrollResult{ScrollItemID: inst.TemplateID, FirstSelect: s.state.Select(playerID, inst.ObjectID)}, true
}

// Cancel clears an active selection and returns the cancellation steps.
func (s *Service) Cancel(playerID int32) Result {
	if !s.state.Clear(playerID) {
		return Result{}
	}
	return Result{Steps: []Step{
		resultStep(ResultCancelled),
		messageStep(Message{Code: MessageEnchantScrollCancelled}),
	}}
}

// EnchantItem applies the selected scroll to objectID.
func (s *Service) EnchantItem(playerID int32, inv *itemcontainer.Inventory, objectID int32) (Result, error) {
	if inv == nil || objectID == 0 {
		return Result{}, nil
	}

	target := inv.ItemByObjectID(objectID)
	scrollInst := inv.ItemByObjectID(s.state.Active(playerID))
	if target == nil || scrollInst == nil {
		return s.Cancel(playerID), nil
	}
	scrollDef, ok := scrolls[scrollInst.TemplateID]
	if !ok {
		return Result{}, nil
	}
	targetTemplate, ok := inv.Templates().Get(target.TemplateID)
	if !ok || !scrollDef.valid(target, targetTemplate) || !Enchantable(target, targetTemplate) {
		return s.failCondition(playerID), nil
	}

	out := Result{}
	destroyedScroll := inv.DestroyItem(scrollInst, 1)
	if destroyedScroll == nil {
		s.state.Clear(playerID)
		out.Steps = append(out.Steps, messageStep(Message{Code: MessageNotEnoughItems}), resultStep(ResultCancelled))
		return out, nil
	}
	out.Persist = append(out.Persist, inventory.DestroyedOrUpdated(destroyedScroll))

	chance := scrollDef.chance(target, targetTemplate)
	if target.Snapshot().OwnerID != playerID || !Enchantable(target, targetTemplate) || chance < 0 {
		failed := s.failCondition(playerID)
		failed.Persist = append(out.Persist, failed.Persist...)
		failed.Steps = append(failed.Steps, Step{Kind: StepInventoryUpdate})
		return failed, nil
	}

	var err error
	if s.roll() < chance {
		out = s.success(playerID, inv, target, out)
	} else if scrollDef.blessed {
		out = s.blessedFailure(playerID, inv, target, out)
	} else {
		out, err = s.normalFailure(playerID, inv, target, targetTemplate, out)
	}
	out.Steps = append(out.Steps, Step{Kind: StepBroadcastEquipment})
	s.state.Clear(playerID)
	return out, err
}

// Enchantable reports whether inst can be enchanted with its template.
func Enchantable(inst *item.Instance, tmpl *item.Template) bool {
	if inst == nil || tmpl == nil {
		return false
	}
	if tmpl.HeroItem() || inst.ShadowItem(tmpl) || tmpl.Kind == item.KindEtcItem {
		return false
	}
	if tmpl.Weapon != nil && tmpl.Weapon.Type == item.WeaponFishingRod {
		return false
	}
	st := inst.Snapshot()
	if st.Location != item.LocationInventory && st.Location != item.LocationPaperdoll {
		return false
	}
	if tmpl.Kind == item.KindWeapon {
		return tmpl.ID < 7822 || tmpl.ID > 7831
	}
	return true
}

// BreakCrystalCount returns how many crystals a broken item should produce.
func BreakCrystalCount(tmpl *item.Template, enchantLevel int) int {
	count := int(tmpl.CrystalCountAt(enchantLevel) - (tmpl.CrystalCount+1)/2)
	if count < 1 {
		return 1
	}
	return count
}

func (s *Service) failCondition(playerID int32) Result {
	s.state.Clear(playerID)
	return Result{Steps: []Step{
		messageStep(Message{Code: MessageInappropriateEnchantCondition}),
		resultStep(ResultCancelled),
	}}
}

func (s *Service) success(playerID int32, inv *itemcontainer.Inventory, target *item.Instance, out Result) Result {
	oldLevel := target.Snapshot().EnchantLevel
	if oldLevel == 0 {
		out.Steps = append(out.Steps, messageStep(Message{Code: MessageS1SuccessfullyEnchanted, ItemID: target.TemplateID}))
	} else {
		out.Steps = append(out.Steps, messageStep(Message{Code: MessageS1S2SuccessfullyEnchanted, ItemID: target.TemplateID, Number: int32(oldLevel)}))
	}
	if inv.SetEnchantLevel(target, oldLevel+1) {
		out.Persist = append(out.Persist, inventory.Update(target))
	}
	out.Steps = append(out.Steps, Step{Kind: StepInventoryUpdate}, resultStep(ResultSuccess))
	return out
}

func (s *Service) blessedFailure(playerID int32, inv *itemcontainer.Inventory, target *item.Instance, out Result) Result {
	out.Steps = append(out.Steps, messageStep(Message{Code: MessageBlessedEnchantFailed}))
	if inv.SetEnchantLevel(target, 0) {
		out.Persist = append(out.Persist, inventory.Update(target))
	}
	out.Steps = append(out.Steps, Step{Kind: StepInventoryUpdate}, resultStep(ResultUnsuccess))
	return out
}

func (s *Service) normalFailure(playerID int32, inv *itemcontainer.Inventory, target *item.Instance, tmpl *item.Template, out Result) (Result, error) {
	crystalID := tmpl.Crystal.ItemID()
	st := target.Snapshot()
	crystalCount := BreakCrystalCount(tmpl, st.EnchantLevel)
	targetLevel := st.EnchantLevel
	targetID := st.TemplateID

	if inv.DestroyItem(target, st.Count) == nil {
		s.state.Clear(playerID)
		out.Steps = append(out.Steps, resultStep(ResultCancelled))
		return out, nil
	}
	out.Persist = append(out.Persist, inventory.Delete(target.ObjectID))

	var err error
	if crystalID != 0 {
		if crystal, e := s.addCrystalReward(inv, crystalID, crystalCount); e != nil {
			err = e
		} else if crystal != nil {
			out.Persist = append(out.Persist, inventory.Save(crystal))
			out.Steps = append(out.Steps, messageStep(Message{Code: MessageEarnedS2S1S, ItemID: crystalID, Number: int32(crystalCount)}))
		}
	}

	if targetLevel > 0 {
		out.Steps = append(out.Steps, messageStep(Message{Code: MessageEnchantmentFailedS1S2Evaporated, ItemID: targetID, Number: int32(targetLevel)}))
	} else {
		out.Steps = append(out.Steps, messageStep(Message{Code: MessageEnchantmentFailedS1Evaporated, ItemID: targetID}))
	}
	out.Steps = append(out.Steps, Step{Kind: StepInventoryUpdate})
	if crystalID == 0 {
		out.Steps = append(out.Steps, resultStep(ResultBrokenNoCrystals))
	} else {
		out.Steps = append(out.Steps, resultStep(ResultBrokenWithCrystals))
	}
	return out, err
}

func (s *Service) addCrystalReward(inv *itemcontainer.Inventory, crystalID int32, count int) (*item.Instance, error) {
	if s.ids == nil {
		return nil, nil
	}
	if _, ok := inv.Templates().Get(crystalID); !ok {
		return nil, nil
	}
	objectID, err := s.ids.NextID()
	if err != nil {
		return nil, fmt.Errorf("allocate enchant crystal item id: %w", err)
	}
	return inv.AddNew(crystalID, count, objectID), nil
}

func messageStep(message Message) Step {
	return Step{Kind: StepSystemMessage, Message: message}
}

func resultStep(result ResultCode) Step {
	return Step{Kind: StepEnchantResult, EnchantResult: result}
}

func (s scroll) valid(inst *item.Instance, tmpl *item.Template) bool {
	if inst == nil || tmpl == nil {
		return false
	}
	switch tmpl.Kind {
	case item.KindWeapon:
		enchantLevel := inst.Snapshot().EnchantLevel
		if !s.weapon || (maxWeapon > 0 && enchantLevel >= maxWeapon) {
			return false
		}
	case item.KindArmor:
		enchantLevel := inst.Snapshot().EnchantLevel
		if s.weapon || (maxArmor > 0 && enchantLevel >= maxArmor) {
			return false
		}
	default:
		return false
	}
	return s.grade == tmpl.Crystal
}

func (s scroll) chance(inst *item.Instance, tmpl *item.Template) float64 {
	if !s.valid(inst, tmpl) {
		return -1
	}
	fullBody := tmpl.Slot == item.SlotFullArmor
	enchantLevel := inst.Snapshot().EnchantLevel
	if enchantLevel < safeMax || (fullBody && enchantLevel < safeMaxFull) {
		return 1
	}
	switch tmpl.Kind {
	case item.KindArmor:
		return math.Pow(chanceArmor, float64(enchantLevel-2))
	case item.KindWeapon:
		if tmpl.Weapon != nil && tmpl.Weapon.Magical {
			if enchantLevel > 14 {
				return chanceMagicWeapon15Plus
			}
			return chanceMagicWeapon
		}
		if enchantLevel > 14 {
			return chanceWeapon15Plus
		}
		return chanceWeapon
	default:
		return 0
	}
}
