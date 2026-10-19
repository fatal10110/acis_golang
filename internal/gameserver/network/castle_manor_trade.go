package network

import (
	"cmp"
	"math"
	"slices"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// otherManorFee is the share of a crop sale's price kept as a fee when the
// crop goes to another castle's manor than the manor manager's own.
const otherManorFee = 0.05

// manorSeedWindow answers a manor manager's seed purchase window for castle
// castleID: only its own castle's seeds are sold there. A manager of no
// castle, or a manor that is off, answers nothing, as specified; a link
// click leaves no client action pending.
func (l *GameClientLink) manorSeedWindow(live *livePlayer, f *npc.Folk, castleID int) {
	own, _ := l.npcResidence(f.Instance)
	if own == nil {
		return
	}
	if castleID != own.ID {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageHereYouCanBuyOnlySeedsOfS1Manor, own.Name))
		return
	}
	inv := live.Inventory()
	if inv == nil || !l.castleManor.Manages(castleID) {
		return
	}
	var seeds []serverpackets.ManorListing
	for _, sp := range l.castleManor.SeedProduction(castleID, false) {
		if sp.Amount() > 0 && sp.Price > 0 {
			seeds = append(seeds, serverpackets.ManorListing{ID: sp.ID, Amount: sp.Amount(), Price: sp.Price})
		}
	}
	live.SendFrame(serverpackets.FrameBuyListSeed(int32(inv.Adena()), int32(castleID), seeds))
}

// frameManorSellCropList is the crop sale window for castle castleID: each
// crop of the manor live holds, with what castleID still buys of it. ok is
// false for a castle the manor holds no lists of.
func (l *GameClientLink) frameManorSellCropList(live *livePlayer, castleID int) (frame wire.Frame, ok bool) {
	inv := live.Inventory()
	if inv == nil || !l.castleManor.Manages(castleID) {
		return wire.Frame{}, false
	}
	wanted := map[int32]*castlemanor.Procure{}
	for _, cp := range l.castleManor.CropProcure(castleID, false) {
		if cp.Amount() > 0 {
			wanted[cp.ID] = cp
		}
	}
	seeds := l.manorSeeds()
	held := seeds.CropIDsWhere(func(cropID int) bool { return inv.ItemByTemplateID(int32(cropID)) != nil })
	crops := make([]serverpackets.ManorHeldCrop, 0, len(held))
	for _, cropID := range held {
		st := inv.ItemByTemplateID(int32(cropID)).Snapshot()
		seed, _ := seeds.SeedByCrop(cropID)
		row := serverpackets.ManorHeldCrop{ObjectID: st.ObjectID, CropID: st.TemplateID, Count: int32(st.Count), Seed: seed}
		if cp, ok := wanted[st.TemplateID]; ok {
			row.Procure = &serverpackets.ManorListing{Amount: cp.Amount(), Price: cp.Price, Reward: cp.Reward}
		}
		crops = append(crops, row)
	}
	return serverpackets.FrameExShowSellCropList(int32(castleID), crops), true
}

// frameManorProcureCropDetail is the crop sale detail window of cropID:
// each castle, in id order, still buying it over the running period.
func (l *GameClientLink) frameManorProcureCropDetail(cropID int32) wire.Frame {
	castles := l.castles.All()
	slices.SortFunc(castles, func(a, b *castle.Castle) int { return cmp.Compare(a.ID, b.ID) })
	var rows []serverpackets.ManorCastleProcure
	for _, c := range castles {
		if cp, ok := l.castleManor.CropProcureOf(c.ID, cropID, false); ok && cp.Amount() > 0 {
			rows = append(rows, serverpackets.ManorCastleProcure{CastleID: int32(c.ID), Amount: cp.Amount(), Price: cp.Price, Reward: cp.Reward})
		}
	}
	return serverpackets.FrameExShowProcureCropDetail(cropID, rows)
}

// manorManagerInReach is the manor manager live last talked to, when it is
// still within its reach.
func (l *GameClientLink) manorManagerInReach(live *livePlayer) (*npc.Folk, bool) {
	f := live.currentFolk.Load()
	if f == nil || f.Instance == nil || f.Instance.Kind != "ManorManagerNpc" || !l.playerCanDoInteract(live, f) {
		return nil, false
	}
	return f, true
}

// seedPurchase is one row of a seed purchase checked against the castle's
// seed list.
type seedPurchase struct {
	order clientpackets.SeedOrder
	sp    *castlemanor.Production
}

// requestBuySeed buys seeds from castle req.ManorID's running seed list at
// the manor manager live last talked to, which must be that castle's and
// within reach. Every row is checked first: a seed the castle sells, priced,
// with enough left, the total within the adena range, then the buyer's
// weight limit, free slots and adena. Each row then takes its seeds and its
// price; a row whose seeds ran out meanwhile is skipped and not paid. The
// adena paid goes to the castle's seed income.
//
// A refused list, the manor under maintenance, an unknown castle, no such
// manager, or a row failing its check answers ActionFailed; the weight,
// slot and adena checks answer their system messages. A manor that is off
// holds no list and answers nothing, as specified: the seed window's
// submit leaves no client action pending.
func (l *GameClientLink) requestBuySeed(live *livePlayer, req clientpackets.RequestBuySeed) {
	inv := live.Inventory()
	if req.Items == nil || inv == nil || l.castleManor.IsUnderMaintenance() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	manorID := int(req.ManorID)
	c, ok := l.castles.Get(manorID)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	f, ok := l.manorManagerInReach(live)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if own, _ := l.npcResidence(f.Instance); own != c {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !l.castleManor.Manages(manorID) {
		return
	}
	var total, weight, slots int64
	rows := make([]seedPurchase, 0, len(req.Items))
	for _, o := range req.Items {
		sp, ok := l.castleManor.SeedProduct(manorID, o.SeedID, false)
		if !ok || sp.Price <= 0 || sp.Amount() < o.Count || math.MaxInt32/o.Count < sp.Price {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		total += int64(sp.Price * o.Count)
		if total > math.MaxInt32 {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		tmpl, ok := inv.Templates().Get(o.SeedID)
		if !ok {
			l.log.Error().Int32("seed", o.SeedID).Msg("buy seed: no item template")
			return
		}
		weight += int64(o.Count) * int64(tmpl.Weight)
		switch {
		case !tmpl.Stackable:
			slots += int64(o.Count)
		case inv.ItemByTemplateID(o.SeedID) == nil:
			slots++
		}
		rows = append(rows, seedPurchase{order: o, sp: sp})
	}
	// A weight or slot count past the int range reads as over the limit.
	if weight > math.MaxInt32 || !inv.ValidateWeight(int(weight)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWeightLimitExceeded))
		return
	}
	if slots > math.MaxInt32 || !inv.ValidateCapacity(int(slots)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
		return
	}
	if int64(inv.Adena()) < total {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return
	}
	for _, r := range rows {
		price := r.sp.Price * r.order.Count
		if !r.sp.DecreaseAmount(r.order.Count) || !takeAdena(inv, int(price)) {
			total -= int64(price)
			continue
		}
		live.AddCreatedItem(r.order.SeedID, int(r.order.Count), l.nextObjectID)
	}
	if total > 0 {
		c.RiseSeedIncome(total)
		live.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1DisappearedAdena,
			serverpackets.SystemMessageParam{Type: serverpackets.SystemMessageParamItemNumber, Value: int32(total)}))
	}
}

// cropSale is one row of a crop sale checked against the held crop and the
// castle's crop list, with the reward item it pays.
type cropSale struct {
	row    clientpackets.CropSale
	cp     *castlemanor.Procure
	reward *item.Template
}

// requestProcureCropList sells held crops to the castles' running crop
// lists at the manor manager live last talked to, within reach. Every row
// is checked first: the held item is the crop, in that count, the castle
// still wants that many, then the reward items' weight and slots. Each row
// is then paid in its reward item, as many as its price buys at the
// reward's reference price; a crop sold to another castle than the
// manager's own costs a 5% fee in adena. A row whose price buys no reward,
// or whose fee the seller cannot pay, is named and skipped; one whose crops
// are no longer wanted is skipped.
//
// The manor under maintenance, no such manager, or a row failing its check
// answers ActionFailed; the weight and slot checks answer their system
// messages. A refused list, a manager of no castle, a castle the manor
// holds no list of, or an unknown reward item answer nothing, as
// specified: the sale window's submit leaves no client action pending.
func (l *GameClientLink) requestProcureCropList(live *livePlayer, req clientpackets.RequestProcureCropList) {
	inv := live.Inventory()
	if req.Items == nil || inv == nil {
		return
	}
	if l.castleManor.IsUnderMaintenance() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	f, ok := l.manorManagerInReach(live)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	own, _ := l.npcResidence(f.Instance)
	if own == nil {
		return
	}
	seeds := l.manorSeeds()
	var weight, slots int64
	sales := make([]cropSale, 0, len(req.Items))
	for _, row := range req.Items {
		inst := inv.ItemByObjectID(row.ObjectID)
		if inst == nil {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		if st := inst.Snapshot(); st.Count < int(row.Count) || st.TemplateID != row.CropID {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		if !l.castleManor.Manages(int(row.ManorID)) {
			return
		}
		cp, ok := l.castleManor.CropProcureOf(int(row.ManorID), row.CropID, false)
		if !ok || cp.Amount() < row.Count {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		rewardID := int32(seeds.SeedReward(int(cp.ID), int(cp.Reward)))
		reward, ok := inv.Templates().Get(rewardID)
		if !ok {
			l.log.Error().Int32("crop", cp.ID).Int32("reward", rewardID).Msg("sell crop: no reward item template")
			return
		}
		weight += int64(row.Count) * int64(reward.Weight)
		switch {
		case !reward.Stackable:
			slots += int64(row.Count)
		case inv.ItemByTemplateID(rewardID) == nil:
			slots++
		}
		sales = append(sales, cropSale{row: row, cp: cp, reward: reward})
	}
	// A weight or slot count past the int range reads as over the limit.
	if weight > math.MaxInt32 || !inv.ValidateWeight(int(weight)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWeightLimitExceeded))
		return
	}
	if slots > math.MaxInt32 || !inv.ValidateCapacity(int(slots)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
		return
	}
	for _, s := range sales {
		l.sellCrop(live, s, own.ID)
	}
}

// sellCrop pays one checked crop sale row in its reward item, taking the
// fee for a sale to another castle than ownCastleID, the crops wanted and
// the crops sold. The price is a 32-bit product, wrapping as int arithmetic
// does.
func (l *GameClientLink) sellCrop(live *livePlayer, s cropSale, ownCastleID int) {
	rewardPrice := s.reward.ReferencePrice
	if rewardPrice == 0 {
		return
	}
	price := s.row.Count * s.cp.Price
	rewardCount := price / rewardPrice
	if rewardCount < 1 {
		live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageFailedInTradingS2OfCropS1, s.row.CropID, s.row.Count))
		return
	}
	var fee int32
	if int(s.row.ManorID) != ownCastleID {
		fee = commons.TruncInt32Sat(float64(price) * otherManorFee)
	}
	if inv := live.Inventory(); fee != 0 && (inv == nil || inv.Adena() < int(fee)) {
		live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageFailedInTradingS2OfCropS1, s.row.CropID, s.row.Count))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return
	}
	if !s.cp.DecreaseAmount(s.row.Count) || (fee > 0 && !reduceAdena(live, int(fee))) || !destroyHeldObject(live, s.row.ObjectID, int(s.row.Count)) {
		return
	}
	live.AddCreatedItem(s.reward.ID, int(rewardCount), l.nextObjectID)
}

// destroyHeldObject destroys count units of item objectID out of live's
// inventory, naming what disappeared; an item no longer held destroys
// nothing and reads as not enough items.
func destroyHeldObject(live *livePlayer, objectID int32, count int) bool {
	inv := live.Inventory()
	var inst *item.Instance
	if inv != nil {
		inst = inv.ItemByObjectID(objectID)
	}
	if inst == nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return false
	}
	templateID := inst.Snapshot().TemplateID
	if inv.DestroyItem(inst, count) == nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return false
	}
	sendDestroyedMessage(live, templateID, count)
	return true
}
