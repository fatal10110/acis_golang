package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// manorMenu is a manor_menu_select command: the window asked for, the
// castle (negative: the NPC's own) and whether the next period is shown.
type manorMenu struct {
	ask, state int
	next       bool
}

// parseManorMenu reads the ask, state and time values from the first three
// '&'-separated tokens of s, each value being what follows the token's
// first '=' up to the next one. It reports false for a command short of a
// token or value, or an ask or state that is no int.
func parseManorMenu(s string) (manorMenu, bool) {
	tokens := strings.FieldsFunc(s, func(r rune) bool { return r == '&' })
	if len(tokens) < 3 {
		return manorMenu{}, false
	}
	var values [3]string
	for i := range values {
		parts := strings.Split(tokens[i], "=")
		for len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
		if len(parts) < 2 {
			return manorMenu{}, false
		}
		values[i] = parts[1]
	}
	ask, err := commons.ParseInt(values[0], 32)
	if err != nil {
		return manorMenu{}, false
	}
	state, err := commons.ParseInt(values[1], 32)
	if err != nil {
		return manorMenu{}, false
	}
	return manorMenu{ask: int(ask), state: int(state), next: values[2] == "1"}, true
}

// bypassManorMenu answers a manor_menu_select command on the player's
// target, at any distance and from any page: a manor manager's manor
// windows, and a castle chamberlain's or a wyvern manager's. A merchant,
// fisherman, gatekeeper, warehouse keeper, dungeon gatekeeper, castle
// blacksmith or auctioneer answers it as it answers any command it does
// not know: with whatever it says to every command first (a karma refusal
// page, a leading ActionFailed, a disabled-manor page, a trade or auction
// refusal), else nothing. Any other target answers nothing: its dialog
// knows no such command, and a link click leaves no client action pending.
func (l *GameClientLink) bypassManorMenu(live *livePlayer, command string) {
	f, ok := live.Target().(*npc.Folk)
	if !ok || f.Instance == nil {
		return
	}
	switch kind := f.Instance.Kind; kind {
	case "ManorManagerNpc":
		l.manorManagerMenu(live, f, command)
	case "CastleChamberlain", "WyvernManagerNpc":
		l.chamberlainManorMenu(live, f, kind, command)
	case "Auctioneer":
		l.auctioneerBypass(live, f, command)
	case "Merchant", "Fisherman", "Gatekeeper", "WarehouseKeeper", "CastleWarehouseKeeper", "DungeonGatekeeper", "CastleBlacksmith":
		reply := l.folkReply(live, f, command)
		if reply.Outcome == npc.BypassUnported {
			// No command of these dialogs reads a manor window.
			reply.Outcome = npc.BypassRefused
		}
		l.runFolkReply(live, f, command, reply)
	}
}

// refuseManorMaintenance turns live away while the manor period rolls
// over, reporting whether it did.
func (l *GameClientLink) refuseManorMaintenance(live *livePlayer) bool {
	if !l.castleManor.IsUnderMaintenance() {
		return false
	}
	live.SendFrame(serverpackets.FrameActionFailed())
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageManorUnderMaintenance))
	return true
}

// manorManagerMenu answers a manor manager's manor windows: the seeds or
// crops of a castle for a period, or the manor's crops, with the purchase
// buttons shown, the seed purchase, crop sale and crop sale detail windows,
// and the harvester shop. A command that does not parse, or
// an unknown ask, answers nothing, as specified; a link click leaves no
// client action pending.
func (l *GameClientLink) manorManagerMenu(live *livePlayer, f *npc.Folk, command string) {
	if l.refuseManorMaintenance(live) {
		return
	}
	menu, ok := parseManorMenu(command)
	if !ok {
		return
	}
	castleID := menu.state
	if menu.state < 0 {
		c, _ := l.npcResidence(f.Instance)
		if c == nil {
			return
		}
		castleID = c.ID
	}
	switch menu.ask {
	case 1:
		l.manorSeedWindow(live, f, castleID)
	case 2:
		// A castle the manor holds no lists of answers nothing.
		if frame, ok := l.frameManorSellCropList(live, castleID); ok {
			live.SendFrame(frame)
		}
	case 9:
		// The state names the crop; a manor that is off answers nothing.
		if l.castleManor.Enabled() {
			live.SendFrame(l.frameManorProcureCropDetail(int32(menu.state)))
		}
	case 3:
		live.SendFrame(l.frameManorSeedInfo(castleID, menu.next, false))
	case 4:
		live.SendFrame(l.frameManorCropInfo(castleID, menu.next, false))
	case 5:
		live.SendFrame(serverpackets.FrameExShowManorDefaultInfo(false, l.manorSeeds().Crops()))
	case 6:
		l.showBuyWindow(live, f, 300000+f.NpcID())
	}
}

// chamberlainManorMenu answers the manor windows of a castle chamberlain,
// or of a wyvern manager, which heeds only the talker who last selected it.
// A talker who may not run its castle's manor gets the refusal page, any
// talker at a chamberlain while the siege runs the busy page; see
// chamberlainManorWindows for the rest.
func (l *GameClientLink) chamberlainManorMenu(live *livePlayer, f *npc.Folk, kind npc.InstanceKind, command string) {
	if kind == "WyvernManagerNpc" && live.currentFolk.Load() != f {
		return
	}
	r := l.residenceTalk(live, f)
	if page := r.ChamberlainRefusal(kind, clan.PrivCastleManor); page != "" {
		sendFilledHTML(live, f.ObjectID(), l.setPage(page), 0)
		return
	}
	l.chamberlainManorWindows(live, r, command)
}

// chamberlainManorWindows answers a chamberlain dialog's manor windows for
// a talker allowed to run the manor of the castle r names: the seeds or
// crops of a castle for a period, or the manor's crops, with the purchase
// buttons hidden, and the seed and crop setting windows outside the
// approved period. A command that does not parse, or an unknown ask,
// answers nothing, as specified; a link click leaves no client action
// pending.
func (l *GameClientLink) chamberlainManorWindows(live *livePlayer, r npc.ResidenceTalk, command string) {
	if l.refuseManorMaintenance(live) {
		return
	}
	menu, ok := parseManorMenu(command[strings.IndexByte(command, '?')+1:])
	if !ok {
		return
	}
	castleID := menu.state
	if menu.state == -1 {
		castleID = r.CastleID
	}
	switch menu.ask {
	case 3:
		live.SendFrame(l.frameManorSeedInfo(castleID, menu.next, true))
	case 4:
		live.SendFrame(l.frameManorCropInfo(castleID, menu.next, true))
	case 5:
		live.SendFrame(serverpackets.FrameExShowManorDefaultInfo(true, l.manorSeeds().Crops()))
	case 7, 8:
		switch {
		case l.castleManor.IsManorApproved():
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageManorCannotBeSetUp))
		case menu.ask == 7:
			live.SendFrame(l.frameManorSeedSetting(castleID))
		default:
			live.SendFrame(l.frameManorCropSetting(castleID))
		}
	}
}

// manorSeeds is the manor's seed table, nil while the manor is off: a
// manor that is off loads no seeds.
func (l *GameClientLink) manorSeeds() *manor.Table {
	if !l.castleManor.Enabled() {
		return nil
	}
	return l.castleManor.Seeds()
}

// frameManorSeedInfo is the seeds castle castleID sells over the running
// period, or the next one, which shows none until it is approved.
func (l *GameClientLink) frameManorSeedInfo(castleID int, next, hideButtons bool) wire.Frame {
	var rows []serverpackets.ManorListing
	if !next || l.castleManor.IsManorApproved() {
		seeds := l.manorSeeds()
		for _, sp := range l.castleManor.SeedProduction(castleID, next) {
			s, _ := seeds.Seed(sp.ID)
			rows = append(rows, serverpackets.ManorListing{ID: sp.ID, Amount: sp.Amount(), StartAmount: sp.StartAmount, Price: sp.Price, Seed: s})
		}
	}
	return serverpackets.FrameExShowSeedInfo(int32(castleID), hideButtons, rows)
}

// frameManorCropInfo is the crops castle castleID buys over the running
// period, or the next one, which shows none until it is approved. Each
// crop shows the first seed of the manor growing it.
func (l *GameClientLink) frameManorCropInfo(castleID int, next, hideButtons bool) wire.Frame {
	var rows []serverpackets.ManorListing
	if !next || l.castleManor.IsManorApproved() {
		seeds := l.manorSeeds()
		for _, cp := range l.castleManor.CropProcure(castleID, next) {
			s, _ := seeds.SeedByCrop(int(cp.ID))
			rows = append(rows, serverpackets.ManorListing{
				ID: cp.ID, Amount: cp.Amount(), StartAmount: cp.StartAmount, Price: cp.Price, Reward: cp.Reward, Seed: s,
			})
		}
	}
	return serverpackets.FrameExShowCropInfo(int32(castleID), hideButtons, rows)
}

// frameManorSeedSetting is the seed setting window of castle castleID: each
// seed of its manor with what is set for it in the running and the next
// period.
func (l *GameClientLink) frameManorSeedSetting(castleID int) wire.Frame {
	m := l.castleManor
	var rows []serverpackets.ManorSetting
	for _, s := range l.manorSeeds().SeedsForCastle(castleID) {
		row := serverpackets.ManorSetting{Seed: s, Limit: m.SeedsLimit(s)}
		if sp, ok := m.SeedProduct(castleID, int32(s.SeedID), false); ok {
			row.Current = serverpackets.ManorPeriodSetting{StartAmount: sp.StartAmount, Price: sp.Price}
		}
		if sp, ok := m.SeedProduct(castleID, int32(s.SeedID), true); ok {
			row.Next = serverpackets.ManorPeriodSetting{StartAmount: sp.StartAmount, Price: sp.Price}
		}
		rows = append(rows, row)
	}
	return serverpackets.FrameExShowSeedSetting(int32(castleID), rows)
}

// frameManorCropSetting is the crop setting window of castle castleID: each
// seed of its manor with what is set for its crop in the running and the
// next period.
func (l *GameClientLink) frameManorCropSetting(castleID int) wire.Frame {
	m := l.castleManor
	var rows []serverpackets.ManorSetting
	for _, s := range l.manorSeeds().SeedsForCastle(castleID) {
		row := serverpackets.ManorSetting{Seed: s, Limit: m.CropsLimit(s)}
		if cp, ok := m.CropProcureOf(castleID, int32(s.CropID), false); ok {
			row.Current = serverpackets.ManorPeriodSetting{StartAmount: cp.StartAmount, Price: cp.Price, Reward: cp.Reward}
		}
		if cp, ok := m.CropProcureOf(castleID, int32(s.CropID), true); ok {
			row.Next = serverpackets.ManorPeriodSetting{StartAmount: cp.StartAmount, Price: cp.Price, Reward: cp.Reward}
		}
		rows = append(rows, row)
	}
	return serverpackets.FrameExShowCropSetting(int32(castleID), rows)
}

// mayRunManor reports whether live may set castle castleID's manor lists:
// its clan owns that castle, it holds the manor privilege, and the NPC it
// last talked to is within its reach.
func (l *GameClientLink) mayRunManor(live *livePlayer, castleID int32) bool {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok || cl.CastleID() != castleID || !cl.HasPrivilege(live.ObjectID(), clan.PrivCastleManor) {
		return false
	}
	f := live.currentFolk.Load()
	return f != nil && l.playerCanDoInteract(live, f)
}

// requestSetSeed sets the seeds castle req.ManorID sells over the next
// period, each seed kept only within the manor's limit and price bounds.
// Outside the modifiable period, or for a player who may not run the
// castle's manor, it answers ActionFailed and sets nothing. A list it sets
// answers nothing, as specified: the setting window's submit leaves no
// client action pending.
func (l *GameClientLink) requestSetSeed(live *livePlayer, req clientpackets.RequestSetSeed) {
	if !l.castleManor.IsModifiablePeriod() || !l.mayRunManor(live, req.ManorID) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	list := make([]*castlemanor.Production, 0, len(req.Items))
	for _, it := range req.Items {
		list = append(list, castlemanor.NewProduction(it.SeedID, it.Sales, it.Price, it.Sales))
	}
	if !l.castleManor.SubmitNextSeedProduction(int(req.ManorID), list) {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}

// requestSetCrop sets the crops castle req.ManorID buys over the next
// period, as requestSetSeed sets its seeds. A request whose every crop has
// no sales sets nothing and answers nothing, as specified: the setting
// window's submit leaves no client action pending.
func (l *GameClientLink) requestSetCrop(live *livePlayer, req clientpackets.RequestSetCrop) {
	if len(req.Items) == 0 {
		return
	}
	if !l.castleManor.IsModifiablePeriod() || !l.mayRunManor(live, req.ManorID) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	list := make([]*castlemanor.Procure, 0, len(req.Items))
	for _, it := range req.Items {
		list = append(list, castlemanor.NewProcure(it.CropID, it.Sales, it.Price, it.Sales, int32(it.Reward)))
	}
	if !l.castleManor.SubmitNextCropProcure(int(req.ManorID), list) {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}
