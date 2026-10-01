package enchant

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Config holds the players.properties enchant rates and limits.
type Config struct {
	// ChanceMagicWeapon and ChanceMagicWeapon15Plus are a magic weapon's
	// success chance below +15 and from +15 on.
	ChanceMagicWeapon       float64
	ChanceMagicWeapon15Plus float64
	// ChanceWeapon and ChanceWeapon15Plus are any other weapon's.
	ChanceWeapon       float64
	ChanceWeapon15Plus float64
	// ChanceArmor is the armor and jewelry chance base, raised to the power
	// of the enchant level minus 2.
	ChanceArmor float64
	// MaxWeapon and MaxArmor cap the enchant level a scroll still accepts;
	// 0 leaves it uncapped.
	MaxWeapon int
	MaxArmor  int
	// SafeMax is the level below which an enchant cannot fail; SafeMaxFull
	// is the same for full-body armor.
	SafeMax     int
	SafeMaxFull int
}

// DefaultConfig returns the enchant settings players.properties ships with.
func DefaultConfig() Config {
	return Config{
		ChanceMagicWeapon:       0.4,
		ChanceMagicWeapon15Plus: 0.2,
		ChanceWeapon:            0.7,
		ChanceWeapon15Plus:      0.35,
		ChanceArmor:             0.66,
		SafeMax:                 3,
		SafeMaxFull:             4,
	}
}

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
	// StepBroadcastEquipment means other players need equipment refreshes.
	StepBroadcastEquipment
	// StepGrantEnchantSkill means the equipped weapon Template reached +4:
	// its +4 enchant skill is added and SkillList resent.
	StepGrantEnchantSkill
	// StepRevokeEnchantSkill means the equipped weapon Template at +4 or
	// higher failed: its +4 enchant skill is removed and SkillList resent.
	StepRevokeEnchantSkill
	// StepGrantArmorSetSkill means a worn armor piece reached +6 with the
	// worn set at +6 or higher throughout: the set's +6 skill SkillID is
	// added and SkillList resent.
	StepGrantArmorSetSkill
	// StepRevokeArmorSetSkill means a worn armor piece at +6 or higher
	// failed while the worn set was at +6 or higher throughout: the set's
	// +6 skill SkillID is removed and SkillList resent.
	StepRevokeArmorSetSkill
	// StepUnequipped means a broken item left the paperdoll: Unequipped
	// lists every instance the removal took off, whose equip side effects
	// have to be undone.
	StepUnequipped
	// StepCancelTrade means the player's active trade is cancelled.
	StepCancelTrade
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
	MessageCannotEnchantWhileStore
	MessageTradeAttemptFailed
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
	// Template is the weapon of a StepGrantEnchantSkill or
	// StepRevokeEnchantSkill.
	Template *item.Template
	// SkillID is the armor set +6 skill of a StepGrantArmorSetSkill or
	// StepRevokeArmorSetSkill.
	SkillID int32
	// Unequipped lists what a StepUnequipped took off the paperdoll.
	Unequipped []*item.Instance
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
	cfg   Config
	// armorSets resolves the worn armor set whose +6 skill an armor
	// enchant grants or revokes; nil grants none.
	armorSets *armorset.Table
}

// NewService returns an enchant workflow service using cfg's rates and
// limits.
func NewService(state *State, ids inventory.IDAllocator, roll func() float64, cfg Config) *Service {
	if state == nil {
		state = NewState()
	}
	if roll == nil {
		roll = func() float64 { return rnd.GetFloat(1) }
	}
	return &Service{state: state, ids: ids, roll: roll, cfg: cfg}
}

// SetArmorSets makes an armor enchant grant and revoke the worn armor
// set's +6 skill. Call it before the service handles any request.
func (s *Service) SetArmorSets(t *armorset.Table) {
	s.armorSets = t
}

// UseScroll selects the enchant scroll inst, held in inv, for playerID.
// FirstSelect reports that no scroll was selected before, which is when the
// selection prompt message goes out.
func (s *Service) UseScroll(playerID int32, inv *itemcontainer.Inventory, inst *item.Instance) (UseScrollResult, bool) {
	if inst == nil {
		return UseScrollResult{}, false
	}
	if _, ok := scrolls[inst.TemplateID]; !ok {
		return UseScrollResult{}, false
	}
	first := s.selectedScroll(playerID, inv) == nil
	s.state.Select(playerID, inst.ObjectID)
	return UseScrollResult{ScrollItemID: inst.TemplateID, FirstSelect: first}, true
}

// Selected reports whether playerID has a scroll selected that inv still
// holds — the selection the restart and logout guards refuse on.
func (s *Service) Selected(playerID int32, inv *itemcontainer.Inventory) bool {
	return s.selectedScroll(playerID, inv) != nil
}

// selectedScroll returns playerID's selected scroll while inv still holds
// it. A selected scroll that has since left inv no longer counts as a
// selection — the reference drops it the moment its item leaves the
// inventory — so it is cleared here without a word to the client.
func (s *Service) selectedScroll(playerID int32, inv *itemcontainer.Inventory) *item.Instance {
	active := s.state.Active(playerID)
	if active == 0 {
		return nil
	}
	var scroll *item.Instance
	if inv != nil {
		scroll = inv.ItemByObjectID(active)
	}
	if scroll == nil {
		s.state.ClearIf(playerID, active)
	}
	return scroll
}

// Cancel clears playerID's active selection of a scroll inv still holds and
// returns the cancellation steps; with no such selection it returns none.
func (s *Service) Cancel(playerID int32, inv *itemcontainer.Inventory) Result {
	if s.selectedScroll(playerID, inv) == nil || !s.state.Clear(playerID) {
		return Result{}
	}
	return Result{Steps: []Step{
		resultStep(ResultCancelled),
		messageStep(Message{Code: MessageEnchantScrollCancelled}),
	}}
}

// Request is one enchant attempt: the item playerID asked to enchant, in
// inv, with the player state the attempt is gated on.
type Request struct {
	PlayerID int32
	Inv      *itemcontainer.Inventory
	ObjectID int32
	// Busy reports that the player runs a private store or workshop, or is
	// tied up in a trade or a trade request.
	Busy bool
	// TradeActive reports, once the scroll is consumed, whether the player
	// has an open trade window.
	TradeActive func() bool
}

// EnchantItem applies the selected scroll to the requested item.
func (s *Service) EnchantItem(req Request) (Result, error) {
	playerID, inv, objectID := req.PlayerID, req.Inv, req.ObjectID
	if inv == nil || objectID == 0 {
		return Result{}, nil
	}
	if req.Busy {
		s.state.Clear(playerID)
		return Result{Steps: []Step{
			messageStep(Message{Code: MessageCannotEnchantWhileStore}),
			resultStep(ResultCancelled),
		}}, nil
	}

	target := inv.ItemByObjectID(objectID)
	scrollInst := s.selectedScroll(playerID, inv)
	if target == nil || scrollInst == nil {
		return s.Cancel(playerID, inv), nil
	}
	scrollDef, ok := scrolls[scrollInst.TemplateID]
	if !ok {
		return Result{}, nil
	}
	targetTemplate, ok := inv.Templates().Get(target.TemplateID)
	if !ok || !scrollDef.valid(target, targetTemplate, s.cfg) || !Enchantable(target, targetTemplate) {
		return s.failCondition(playerID), nil
	}

	out := Result{}
	// Read the scroll's owner before consuming it: a fully consumed stack
	// comes back from DestroyItem with its owner zeroed, and the row's write
	// has to stay on the lane its earlier writes used.
	scrollOwnerID := scrollInst.Snapshot().OwnerID
	destroyedScroll := inv.DestroyItem(scrollInst, 1)
	if destroyedScroll == nil {
		s.state.Clear(playerID)
		out.Steps = append(out.Steps, messageStep(Message{Code: MessageNotEnoughItems}), resultStep(ResultCancelled))
		return out, nil
	}
	out.Persist = append(out.Persist, inventory.DestroyedOrUpdated(scrollOwnerID, destroyedScroll))

	// An open trade window stops the attempt with the scroll already spent.
	// The selection stays, as in the reference, unless that was the last
	// scroll.
	if req.TradeActive != nil && req.TradeActive() {
		out.Steps = append(out.Steps, Step{Kind: StepCancelTrade}, messageStep(Message{Code: MessageTradeAttemptFailed}))
		return out, nil
	}

	chance := scrollDef.chance(target, targetTemplate, s.cfg)
	if target.Snapshot().OwnerID != playerID || !Enchantable(target, targetTemplate) || chance < 0 {
		failed := s.failCondition(playerID)
		failed.Persist = append(out.Persist, failed.Persist...)
		return failed, nil
	}

	var err error
	if s.roll() < chance {
		out = s.success(inv, target, targetTemplate, out)
	} else {
		// An equipped weapon at +4 or higher loses its +4 enchant skill,
		// and an equipped armor piece at +6 or higher its worn set's +6
		// skill, before the failure takes its level or the item itself.
		if st := target.Snapshot(); st.Equipped() && hasEnchant4Skill(targetTemplate) && st.EnchantLevel >= item.Enchant4SkillLevel {
			out.Steps = append(out.Steps, Step{Kind: StepRevokeEnchantSkill, Template: targetTemplate})
		} else if st.Equipped() && targetTemplate.Kind == item.KindArmor && st.EnchantLevel >= armorSetEnchantLevel {
			if skillID := s.wornSetEnchant6Skill(inv); skillID > 0 {
				out.Steps = append(out.Steps, Step{Kind: StepRevokeArmorSetSkill, SkillID: skillID})
			}
		}
		if scrollDef.blessed {
			out = s.blessedFailure(inv, target, out)
		} else {
			out, err = s.normalFailure(playerID, inv, target, targetTemplate, out)
		}
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

func (s *Service) success(inv *itemcontainer.Inventory, target *item.Instance, tmpl *item.Template, out Result) Result {
	oldLevel := target.Snapshot().EnchantLevel
	if oldLevel == 0 {
		out.Steps = append(out.Steps, messageStep(Message{Code: MessageS1SuccessfullyEnchanted, ItemID: target.TemplateID}))
	} else {
		out.Steps = append(out.Steps, messageStep(Message{Code: MessageS1S2SuccessfullyEnchanted, ItemID: target.TemplateID, Number: int32(oldLevel)}))
	}
	if inv.SetEnchantLevel(target, oldLevel+1) {
		out.Persist = append(out.Persist, inventory.Update(target))
	}
	// Reaching exactly +4 on an equipped weapon grants its +4 enchant skill;
	// reaching exactly +6 on an equipped armor piece grants the worn set's
	// +6 skill once every set piece is at +6 or higher. The piece need not
	// belong to the set.
	if st := target.Snapshot(); st.Equipped() && st.EnchantLevel == item.Enchant4SkillLevel && hasEnchant4Skill(tmpl) {
		out.Steps = append(out.Steps, Step{Kind: StepGrantEnchantSkill, Template: tmpl})
	} else if st.Equipped() && tmpl.Kind == item.KindArmor && st.EnchantLevel == armorSetEnchantLevel {
		if skillID := s.wornSetEnchant6Skill(inv); skillID > 0 {
			out.Steps = append(out.Steps, Step{Kind: StepGrantArmorSetSkill, SkillID: skillID})
		}
	}
	out.Steps = append(out.Steps, resultStep(ResultSuccess))
	return out
}

// armorSetEnchantLevel is the enchant level from which a worn armor set's
// pieces grant its +6 skill.
const armorSetEnchantLevel = 6

// wornSetEnchant6Skill returns the +6 skill of the armor set inv's worn
// chest belongs to while every piece of it is at +6 or higher, or 0.
func (s *Service) wornSetEnchant6Skill(inv *itemcontainer.Inventory) int32 {
	set, ok := s.armorSets.Worn(inv)
	if !ok || !set.Enchanted6(inv) {
		return 0
	}
	return set.Enchant6Skill
}

// hasEnchant4Skill reports whether tmpl is a weapon with a +4 enchant skill.
func hasEnchant4Skill(tmpl *item.Template) bool {
	return tmpl != nil && tmpl.Weapon != nil && tmpl.Weapon.Enchant4Skill != nil
}

func (s *Service) blessedFailure(inv *itemcontainer.Inventory, target *item.Instance, out Result) Result {
	out.Steps = append(out.Steps, messageStep(Message{Code: MessageBlessedEnchantFailed}))
	if inv.SetEnchantLevel(target, 0) {
		out.Persist = append(out.Persist, inventory.Update(target))
	}
	out.Steps = append(out.Steps, resultStep(ResultUnsuccess))
	return out
}

func (s *Service) normalFailure(playerID int32, inv *itemcontainer.Inventory, target *item.Instance, tmpl *item.Template, out Result) (Result, error) {
	crystalID := tmpl.Crystal.ItemID()
	st := target.Snapshot()
	crystalCount := BreakCrystalCount(tmpl, st.EnchantLevel)
	targetLevel := st.EnchantLevel
	targetID := st.TemplateID

	// A worn item comes off the paperdoll as it is destroyed, taking a bow's
	// or rod's arrows or lure with it; its equip side effects are undone
	// right there, ahead of the crystal reward.
	var unequipped []*item.Instance
	if st.Equipped() {
		unequipped = inv.UnequipItem(target)
	}
	if inv.DestroyItem(target, st.Count) == nil {
		s.state.Clear(playerID)
		if len(unequipped) > 0 {
			out.Steps = append(out.Steps, Step{Kind: StepUnequipped, Unequipped: unequipped})
		}
		out.Steps = append(out.Steps, resultStep(ResultCancelled))
		return out, nil
	}
	out.Persist = append(out.Persist, inventory.Delete(st.OwnerID, st.ObjectID))
	if len(unequipped) > 0 {
		out.Steps = append(out.Steps, Step{Kind: StepUnequipped, Unequipped: unequipped})
	}

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

func (s scroll) valid(inst *item.Instance, tmpl *item.Template, cfg Config) bool {
	if inst == nil || tmpl == nil {
		return false
	}
	switch tmpl.Kind {
	case item.KindWeapon:
		enchantLevel := inst.Snapshot().EnchantLevel
		if !s.weapon || (cfg.MaxWeapon > 0 && enchantLevel >= cfg.MaxWeapon) {
			return false
		}
	case item.KindArmor:
		enchantLevel := inst.Snapshot().EnchantLevel
		if s.weapon || (cfg.MaxArmor > 0 && enchantLevel >= cfg.MaxArmor) {
			return false
		}
	default:
		return false
	}
	return s.grade == tmpl.Crystal
}

func (s scroll) chance(inst *item.Instance, tmpl *item.Template, cfg Config) float64 {
	if !s.valid(inst, tmpl, cfg) {
		return -1
	}
	fullBody := tmpl.Slot == item.SlotFullArmor
	enchantLevel := inst.Snapshot().EnchantLevel
	if enchantLevel < cfg.SafeMax || (fullBody && enchantLevel < cfg.SafeMaxFull) {
		return 1
	}
	switch tmpl.Kind {
	case item.KindArmor:
		return math.Pow(cfg.ChanceArmor, float64(enchantLevel-2))
	case item.KindWeapon:
		if tmpl.Weapon != nil && tmpl.Weapon.Magical {
			if enchantLevel > 14 {
				return cfg.ChanceMagicWeapon15Plus
			}
			return cfg.ChanceMagicWeapon
		}
		if enchantLevel > 14 {
			return cfg.ChanceWeapon15Plus
		}
		return cfg.ChanceWeapon
	default:
		return 0
	}
}
