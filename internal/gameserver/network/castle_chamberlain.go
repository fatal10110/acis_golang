package network

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// The chamberlain dialog's pages, and what its certificate sale and crown
// hand over.
const (
	chamberlainPages = "data/html/chamberlain/"
	// certificateBundle certificates of certificateItemID sell together,
	// at certificatePrice adena each.
	certificateBundle = 10
	certificatePrice  = 1000
	certificateItemID = 6388
	lordCrownItemID   = 6841
	// castleBanishOffset is the random offset of a player thrown out of a
	// castle's grounds.
	castleBanishOffset = 20
	// javaDateLayout is how the siege date reads on the chamberlain's
	// pages.
	javaDateLayout = "Mon Jan 02 15:04:05 MST 2006"
)

// chamberlainDialog is one command of the castle chamberlain dialog: the
// talker live, of standing at the castle c that f serves, seeing r.
type chamberlainDialog struct {
	l        *GameClientLink
	live     *livePlayer
	f        *npc.Folk
	c        *castle.Castle
	r        npc.ResidenceTalk
	standing npc.ChamberlainStanding
}

// chamberlainBypass runs command, one of the castle chamberlain dialog's
// own commands, for live, of standing, at f. A command needing a clan
// privilege live lacks opens the refusal page alone. A number that does not
// parse, or a word a command needs that is missing, stops the handling.
// It reports false when the command aborted, so that nothing more is sent.
func (l *GameClientLink) chamberlainBypass(live *livePlayer, f *npc.Folk, command string, standing npc.ChamberlainStanding) bool {
	c, _ := l.npcResidence(f.Instance)
	if c == nil {
		return false
	}
	d := chamberlainDialog{l: l, live: live, f: f, c: c, r: l.residenceTalk(live, f), standing: standing}
	words := npc.ChamberlainWords(command)
	actual, val, rest := words[0], "", words[1:]
	if len(rest) > 0 {
		val, rest = rest[0], rest[1:]
	}
	is := func(name string) bool { return strings.EqualFold(actual, name) }
	switch {
	case is("banish_foreigner"):
		if d.allowed(clan.PrivCastleDismiss) {
			l.banishFromCastleGrounds(c)
			d.sendFile("banishafter.htm")
		}
	case is("banish_foreigner_show"):
		d.fileIfAllowed(clan.PrivCastleDismiss, "banishfore.htm")
	case is("manage_functions"):
		d.fileIfAllowed(clan.PrivCastleSetFuncs, "manage.htm")
	case is("products"):
		d.fileIfAllowed(clan.PrivCastleFunctions, "products.htm")
	case is("list_siege_clans"):
		if d.allowed(clan.PrivCastleSiege) {
			live.SendFrame(l.frameSiegeInfo(live, c))
		}
	case is("receive_report"):
		d.receiveReport()
	case is("items"):
		return d.items(val)
	case is("manage_vault"):
		return d.manageVault(val, rest)
	case is("operate_door"):
		return d.operateDoor(val, rest)
	case is("tax_set"):
		d.taxSet()
	case is("tax_adjust"):
		return d.taxAdjust(val)
	case is("manor"):
		return d.manor(val, rest)
	case strings.HasPrefix(command, "manor_menu_select"):
		if d.allowed(clan.PrivCastleManor) {
			l.chamberlainManorWindows(live, d.r, command)
		}
	case is("siege_change"):
		d.siegeChange()
	case is("siege_time_set"):
		return d.siegeTimeSet(val, rest)
	case actual == "give_crown":
		d.giveCrown()
	case actual == "manor_certificate":
		d.certificates(false)
	case actual == "validate_certificate":
		d.certificates(true)
	case is("castle_devices"):
		d.fileIfAllowed(clan.PrivCastleSetFuncs, "devices.htm")
	case is("doors_update"):
		return d.doorsUpdate(val, rest)
	case is("doors_choose_upgrade"):
		return d.doorsChooseUpgrade(val, rest)
	case is("doors_confirm_upgrade"):
		return d.doorsConfirmUpgrade(val, rest)
	case is("traps_update"):
		d.trapsUpdate(val)
	case is("traps_choose_upgrade"):
		return d.trapsChooseUpgrade(val, rest)
	case is("traps_confirm_upgrade"):
		return d.trapsConfirmUpgrade(val, rest)
	}
	return true
}

// allowed reports whether the talker holds privilege p, opening the
// refusal page when it does not.
func (d chamberlainDialog) allowed(p clan.Privilege) bool {
	if d.r.Holds(p) {
		return true
	}
	sendFilledHTML(d.live, d.f.ObjectID(), d.l.setPage(chamberlainPages+"noprivs.htm"), 0)
	return false
}

// fileIfAllowed opens file as sendFile does for a talker holding p.
func (d chamberlainDialog) fileIfAllowed(p clan.Privilege, file string) {
	if d.allowed(p) {
		d.sendFile(file)
	}
}

// sendFile opens the chamberlain page file naming the NPC, its template
// and the castle's siege date.
func (d chamberlainDialog) sendFile(file string) {
	d.send(file, "%npcId%", strconv.Itoa(d.f.NpcID()), "%time%", time.UnixMilli(d.c.SiegeDate()).Format(javaDateLayout))
}

// send opens the chamberlain page file, %objectId% naming the NPC, then
// pairs filled in order.
func (d chamberlainDialog) send(file string, pairs ...string) {
	page := fillPage(d.l.setPage(chamberlainPages+file), "%objectId%", objID(d.f))
	sendFilledHTML(d.live, d.f.ObjectID(), fillPage(page, pairs...), 0)
}

// sendRaw opens the chamberlain page file with only pairs filled.
func (d chamberlainDialog) sendRaw(file string, pairs ...string) {
	sendFilledHTML(d.live, d.f.ObjectID(), fillPage(d.l.setPage(chamberlainPages+file), pairs...), 0)
}

// parseInt reads s as a decimal int32.
func parseInt(s string) (int, bool) {
	n, err := commons.ParseInt(s, 32)
	return int(n), err == nil
}

// receiveReport opens the castle report to the lord: the owning clan, its
// leader, the castle, the Seven Signs period and each seal's owner. Another
// member gets the refusal page.
func (d chamberlainDialog) receiveReport() {
	if d.standing == npc.StandingMember {
		d.sendFile("noprivs.htm")
		return
	}
	var period sevensigns.Period
	var seals [3]sevensigns.Cabal
	if d.l.sevenSigns != nil {
		period, seals = d.l.sevenSigns.CurrentPeriod(), d.l.sevenSigns.SealOwners()
	}
	d.send("report.htm",
		"%clanname%", d.r.OwnerName, "%clanleadername%", d.r.OwnerLeader, "%castlename%", d.c.Name,
		"%ss_event%", period.Name(),
		"%ss_avarice%", sealHolder(seals[0]), "%ss_gnosis%", sealHolder(seals[1]), "%ss_strife%", sealHolder(seals[2]))
}

// sealHolder is how the castle report names a seal's owner.
func sealHolder(c sevensigns.Cabal) string {
	switch c {
	case sevensigns.Dawn:
		return "Lords of Dawn"
	case sevensigns.Dusk:
		return "Revolutionaries of Dusk"
	case sevensigns.NoCabal:
	}
	return "Not in Possession"
}

// items opens the castle's product buy window "items <n>" names: buylist
// <n>1. A command naming no list answers nothing.
func (d chamberlainDialog) items(val string) bool {
	if !d.allowed(clan.PrivCastleFunctions) || val == "" {
		return true
	}
	id, ok := parseInt(val + "1")
	if !ok {
		return false
	}
	d.l.showBuyWindow(d.live, d.f, id)
	return true
}

// manageVault answers "manage_vault [deposit|withdraw] <amount>", alone
// among the castle's purchases: a deposit
// the treasury takes below the int32 maximum moves the talker's adena in, a
// withdrawal the treasury covers moves it out, and one it does not cover
// opens vault-no.htm instead of the vault page. Both pages show the
// treasury, the tax revenue, the seed income and the amount asked.
func (d chamberlainDialog) manageVault(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleTaxes) {
		return true
	}
	amount := 0
	if len(rest) > 0 {
		n, ok := parseInt(rest[0])
		if !ok {
			return false
		}
		amount = n
	}
	file := "vault.htm"
	if amount > 0 {
		d.c.Purchase(func() {
			switch {
			case strings.EqualFold(val, "deposit"):
				if d.c.Treasury()+int64(amount) < math.MaxInt32 && reduceAdena(d.live, amount) {
					d.c.EditTreasury(int64(amount), true)
				}
			case strings.EqualFold(val, "withdraw"):
				if d.c.Treasury() < int64(amount) {
					file = "vault-no.htm"
				} else if d.c.EditTreasury(-int64(amount), true) {
					d.live.AddRewardItem(item.AdenaID, amount, false, d.l.nextObjectID)
				}
			}
		})
	}
	d.send(file,
		"%tax_income%", commons.FormatNumber(d.c.Treasury()),
		"%tax_income_reserved%", commons.FormatNumber(d.c.TaxRevenue()),
		"%seed_income%", commons.FormatNumber(d.c.SeedIncome()),
		"%withdraw_amount%", commons.FormatNumber(int64(amount)))
	return true
}

// operateDoor answers "operate_door": the castle's door page <npcId>-d.htm,
// or, with "1" or "0" and door ids, each of those gates of the castle
// opened or closed in turn, then the page saying so. An id that does not
// parse stops the handling once the gates before it are done.
func (d chamberlainDialog) operateDoor(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleEntryExit) {
		return true
	}
	if val == "" {
		d.send(strconv.Itoa(d.f.NpcID()) + "-d.htm")
		return true
	}
	choice, ok := parseInt(val)
	if !ok {
		return false
	}
	open := choice == 1
	for _, word := range rest {
		id, ok := parseInt(word)
		if !ok {
			return false
		}
		if d.live.ClanID() == d.c.OwnerID() {
			d.l.setGateOfID(d.c.Gates, id, open)
		}
	}
	if open {
		d.send("doors-open.htm")
	} else {
		d.send("doors-close.htm")
	}
	return true
}

// taxSet opens the tax page: tax-leader.htm, with the limit the Seal of
// Strife allows, to a talker allowed to set the taxes; tax-member.htm after
// the refusal page to anyone else. Both show the rate in force and the
// next one.
func (d chamberlainDialog) taxSet() {
	current, next := strconv.Itoa(d.c.CurrentTaxPercent()), strconv.Itoa(d.c.NextTaxPercent())
	if !d.allowed(clan.PrivCastleTaxes) {
		d.sendRaw("tax-member.htm", "%currentTaxRate%", current, "%nextTaxRate%", next, "%objectId%", objID(d.f))
		return
	}
	d.sendRaw("tax-leader.htm", "%currentTaxRate%", current, "%nextTaxRate%", next, "%objectId%", objID(d.f),
		"%taxLimit%", strconv.Itoa(sevensigns.CastleTaxCap(d.r.StrifeOwner)))
}

// taxAdjust answers "tax_adjust <rate>", 0 when it names none: a rate from
// 0 to the limit the Seal of Strife allows becomes the next tax rate,
// stored, with tax-after.htm; any other opens tax-toohigh.htm.
func (d chamberlainDialog) taxAdjust(val string) bool {
	limit := sevensigns.CastleTaxCap(d.r.StrifeOwner)
	rate := 0
	if val != "" {
		n, ok := parseInt(val)
		if !ok {
			return false
		}
		rate = n
	}
	if rate < 0 || rate > limit {
		d.sendRaw("tax-toohigh.htm", "%objectId%", objID(d.f), "%taxLimit%", strconv.Itoa(limit))
		return true
	}
	d.c.SetNextTaxPercent(rate, true)
	d.sendRaw("tax-after.htm", "%nextTaxRate%", strconv.Itoa(d.c.NextTaxPercent()), "%objectId%", objID(d.f))
	return true
}

// manor answers "manor 0" with the manor page and "manor 4 <n>" with its
// help page n; any other choice opens no.htm. While the manor is off, every
// choice opens the generic page.
func (d chamberlainDialog) manor(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleManor) {
		return true
	}
	path := "data/html/npcdefault.htm"
	if d.l.chatRules().AllowManor {
		choice, ok := parseInt(val)
		if !ok {
			return false
		}
		switch choice {
		case 0:
			path = chamberlainPages + "manor/manor.htm"
		case 4:
			if len(rest) == 0 {
				return false
			}
			path = chamberlainPages + "manor/manor_help00" + rest[0] + ".htm"
		default:
			path = chamberlainPages + "no.htm"
		}
	}
	sendFilledHTML(d.live, d.f.ObjectID(), fillPage(d.l.setPage(path), "%objectId%", objID(d.f)), 0)
	return true
}

// siegeChange opens the siege date page: siegetime1.htm once the
// registrations have ended, a day before the siege; siegetime2.htm once
// the date is closed to change; siegetime3.htm otherwise.
func (d chamberlainDialog) siegeChange() {
	if !d.allowed(clan.PrivCastleSiege) {
		return
	}
	const dayMs = int64(24 * time.Hour / time.Millisecond)
	switch {
	case d.c.SiegeDate()-dayMs < d.l.sieges.Now().UnixMilli():
		d.sendFile("siegetime1.htm")
	case d.c.IsTimeRegistrationOver():
		d.sendFile("siegetime2.htm")
	default:
		d.sendFile("siegetime3.htm")
	}
}

// siegeTimeSet answers "siege_time_set 1 <hour>", which makes hour the
// NPC's siege hour choice, and "siege_time_set <other>", which keeps it.
// A choice other than the default moves the siege to 12 hours past it,
// closing the date to change, with siegetime8.htm; the default opens
// siegetime6.htm.
func (d chamberlainDialog) siegeTimeSet(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleSiege) {
		return true
	}
	choice, ok := parseInt(val)
	if !ok {
		return false
	}
	if choice == 1 {
		if len(rest) == 0 {
			return false
		}
		hour, ok := parseInt(rest[0])
		if !ok {
			return false
		}
		d.f.SetSiegePreHour(hour)
	}
	if hour := d.f.SiegePreHour(); hour != npc.DefaultSiegePreHour {
		d.c.SetSiegeHour(hour + 12)
		d.sendFile("siegetime8.htm")
		return true
	}
	d.sendFile("siegetime6.htm")
	return true
}

// giveCrown hands the lord the Lord's Crown it does not hold yet, with
// gavecrown.htm naming it and the castle, or hascrown.htm when it holds
// one; another member gets the refusal page.
func (d chamberlainDialog) giveCrown() {
	if d.standing != npc.StandingLord {
		sendFilledHTML(d.live, d.f.ObjectID(), d.l.setPage(chamberlainPages+"noprivs.htm"), 0)
		return
	}
	if inv := d.live.Inventory(); inv != nil && inv.HasItem(lordCrownItemID) {
		sendFilledHTML(d.live, d.f.ObjectID(), d.l.setPage(chamberlainPages+"hascrown.htm"), 0)
		return
	}
	d.live.AddCreatedItem(lordCrownItemID, 1, d.l.nextObjectID)
	d.sendRaw("gavecrown.htm", "%CharName%", d.live.CharacterName(), "%FeudName%", d.c.Name)
}

// certificates answers manor_certificate, the certificate sale page, and,
// with buy set, validate_certificate, which sells a bundle: during the seal
// validation period, to a talker not of the dusk, while the castle has
// certificates left. The sale takes the bundle's adena, opening
// not-enough-adena.htm to a talker short of it, and hands the bundle over;
// sell-dawn-ticket.htm then shows what is left.
func (d chamberlainDialog) certificates(buy bool) {
	if !d.allowed(clan.PrivCastleFunctions) {
		return
	}
	d.c.Purchase(func() {
		file, left := "not-dawn-or-event.htm", d.c.LeftCertificates()
		switch {
		case !d.r.SealValidation, d.l.sevenSigns != nil && d.l.sevenSigns.PlayerCabal(d.live.ObjectID()) == sevensigns.Dusk:
		case left == 0:
			file = "not-enough-ticket.htm"
		case !buy:
			file = "sell-dawn-ticket.htm"
		case reduceAdena(d.live, certificateBundle*certificatePrice):
			d.live.AddCreatedItem(certificateItemID, certificateBundle, d.l.nextObjectID)
			file, left = "sell-dawn-ticket.htm", d.c.TakeCertificates(certificateBundle)
		default:
			file = "not-enough-adena.htm"
		}
		pairs := []string{"%objectId%", objID(d.f)}
		if file == "sell-dawn-ticket.htm" {
			pairs = append([]string{"%left%", strconv.Itoa(left), "%bundle%", strconv.Itoa(certificateBundle), "%price%", strconv.Itoa(certificatePrice)}, pairs...)
		}
		d.sendRaw(file, pairs...)
	})
}

// doorsUpdate opens the gate upgrade page <npcId>-gu.htm, or with a gate
// and its type doors-update.htm naming both.
func (d chamberlainDialog) doorsUpdate(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleSetFuncs) {
		return true
	}
	if val == "" {
		d.send(strconv.Itoa(d.f.NpcID()) + "-gu.htm")
		return true
	}
	if len(rest) == 0 {
		return false
	}
	d.sendRaw("doors-update.htm", "%id%", val, "%type%", rest[0], "%objectId%", objID(d.f))
	return true
}

// doorsChooseUpgrade answers "doors_choose_upgrade <gate> <type> <ratio>"
// with doors-confirm.htm naming them and the upgrade's price.
func (d chamberlainDialog) doorsChooseUpgrade(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleSetFuncs) {
		return true
	}
	if len(rest) < 2 {
		return false
	}
	gateType, ok := parseInt(rest[0])
	if !ok {
		return false
	}
	ratio, ok := parseInt(rest[1])
	if !ok {
		return false
	}
	price := castle.DoorUpgradeCost(gateType, ratio, d.r.StrifeOwner)
	d.send("doors-confirm.htm", "%id%", val, "%level%", rest[1], "%type%", rest[0], "%price%", strconv.Itoa(price))
	return true
}

// doorsConfirmUpgrade answers "doors_confirm_upgrade <gate> <type>
// <ratio>": an unpriced upgrade, or a door that is not one of the castle's
// gates, answers nothing; a gate already at that ratio or above opens
// doors-already-updated.htm naming its ratio in percent; a talker short of
// the price not-enough-adena.htm; otherwise the price is paid, the gate
// upgraded and stored, and doors-success.htm opens.
func (d chamberlainDialog) doorsConfirmUpgrade(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleSetFuncs) {
		return true
	}
	if len(rest) < 2 {
		return false
	}
	gateType, ok := parseInt(rest[0])
	if !ok {
		return false
	}
	ratio, ok := parseInt(rest[1])
	if !ok {
		return false
	}
	price := castle.DoorUpgradeCost(gateType, ratio, d.r.StrifeOwner)
	if price == 0 {
		return true
	}
	id, ok := parseInt(val)
	if !ok {
		return false
	}
	d.c.Purchase(func() {
		current, ok := d.c.GateUpgradeRatio(id)
		switch {
		case !ok:
		case current >= ratio:
			d.send("doors-already-updated.htm", "%level%", strconv.Itoa(current*100))
		case !reduceAdena(d.live, price):
			d.send("not-enough-adena.htm")
		default:
			d.c.UpgradeDoor(id, ratio, true)
			d.send("doors-success.htm")
		}
	})
	return true
}

// trapsUpdate opens the trap upgrade page <npcId>-tu.htm, or with a trap
// traps-update.htm naming it, traps-update1.htm at Aden.
func (d chamberlainDialog) trapsUpdate(val string) {
	if !d.allowed(clan.PrivCastleSetFuncs) {
		return
	}
	if val == "" {
		d.send(strconv.Itoa(d.f.NpcID()) + "-tu.htm")
		return
	}
	file := "traps-update.htm"
	if strings.EqualFold(d.c.Name, "aden") {
		file = "traps-update1.htm"
	}
	d.sendRaw(file, "%trapIndex%", val, "%objectId%", objID(d.f))
}

// trapsChooseUpgrade answers "traps_choose_upgrade <trap> <level>" with
// traps-confirm.htm naming them and the upgrade's price.
func (d chamberlainDialog) trapsChooseUpgrade(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleSetFuncs) {
		return true
	}
	if len(rest) == 0 {
		return false
	}
	level, ok := parseInt(rest[0])
	if !ok {
		return false
	}
	price := castle.TrapUpgradeCost(level, d.r.StrifeOwner)
	d.send("traps-confirm.htm", "%trapIndex%", val, "%level%", rest[0], "%price%", strconv.Itoa(price))
	return true
}

// trapsConfirmUpgrade answers "traps_confirm_upgrade <trap> <level>": an
// unpriced level answers nothing and a trap naming no control tower stops
// the handling; a trap already at that level or above opens
// traps-already-updated.htm naming its level; a talker short of the price
// not-enough-adena.htm; otherwise the price is paid, the trap upgraded and
// stored, and traps-success.htm opens.
func (d chamberlainDialog) trapsConfirmUpgrade(val string, rest []string) bool {
	if !d.allowed(clan.PrivCastleSetFuncs) {
		return true
	}
	if len(rest) == 0 {
		return false
	}
	level, ok := parseInt(rest[0])
	if !ok {
		return false
	}
	price := castle.TrapUpgradeCost(level, d.r.StrifeOwner)
	if price == 0 {
		return true
	}
	index, ok := parseInt(val)
	if !ok {
		return false
	}
	aborted := false
	d.c.Purchase(func() {
		current, ok := d.c.TrapUpgrade(index)
		switch {
		case !ok:
			aborted = true
		case current >= level:
			d.send("traps-already-updated.htm", "%level%", strconv.Itoa(current))
		case !reduceAdena(d.live, price):
			d.send("not-enough-adena.htm")
		default:
			d.c.SetTrapUpgrade(index, level, true)
			d.send("traps-success.htm")
		}
	})
	return !aborted
}

// wireCastleZones has every castle's grounds throw a player out to a
// banish point of the castle.
func (l *GameClientLink) wireCastleZones(ix *zone.Index) {
	for _, grounds := range zone.OfKind[*zone.Castle](ix) {
		castleID := grounds.ResidenceID
		grounds.Banish = func(a zone.Actor) { l.banishFromCastle(castleID, a) }
	}
}

// banishFromCastleGrounds throws every player who is not a member of the
// clan owning c out of its grounds: the castle zones naming it.
func (l *GameClientLink) banishFromCastleGrounds(c *castle.Castle) {
	if l.zones == nil {
		return
	}
	owner := c.OwnerID()
	for _, grounds := range zone.OfKind[*zone.Castle](l.zones) {
		if grounds.ResidenceID == c.ID {
			grounds.BanishForeigners(owner)
		}
	}
}

// banishFromCastle teleports a, a player thrown out of castle castleID's
// grounds, to a random banish point of the castle, within
// castleBanishOffset. The teleport runs on the player's own queue, after
// the zone lock the banishment ran under is released. A castle without a
// banish point leaves the player where it is.
func (l *GameClientLink) banishFromCastle(castleID int, a zone.Actor) {
	za, ok := a.(*liveZoneActor)
	if !ok {
		return
	}
	c, ok := l.castles.Get(castleID)
	if !ok {
		return
	}
	spawns := c.Spawns[residence.SpawnBanish]
	if len(spawns) == 0 {
		l.log.Warn().Int("castle_id", castleID).Msg("castle: no banish point to throw a player out to")
		return
	}
	dest := spawns[rnd.Get(len(spawns))]
	live := za.live
	postLive(live, func() { l.teleportLivePlayer(live, dest, castleBanishOffset) })
}
