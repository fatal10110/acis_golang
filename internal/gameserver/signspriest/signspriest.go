// Package signspriest runs the "SevenSigns <n> ..." dialog commands of the
// Seven Signs priests and the Mammon NPCs (SignsPriest.onBypassFeedback):
// buying the Record of Seven Signs, signing up for a cabal, turning seal
// stones in or exchanging them for ancient adena, the Black Marketeer's
// ancient adena exchange, collecting the stones' reward and the seal status
// page. It decides and mutates the talker's inventory and the Seven Signs
// state; the caller turns the returned notices into client packets, opens
// the page and moves the talker. Every call runs on the talker's own queue,
// so a check and the change it allows see the same inventory.
package signspriest

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// Items and prices of the dialog (SevenSignsManager).
const (
	RecordID                int32 = 5707
	CertificateOfApprovalID int32 = 6388
	RecordCost                    = 500
	JoinDawnCost                  = 50000

	BlueStoneID  int32 = 6360
	GreenStoneID int32 = 6361
	RedStoneID   int32 = 6362
)

// Points one stone of each color is worth (SevenSignsManager.SEAL_STONE_*_VALUE).
const (
	blueValue  = 3
	greenValue = 5
	redValue   = 10
)

// Config is the events.properties Seven Signs settings the dialog reads.
type Config struct {
	// MaxPlayerContrib caps the contribution score one player's stones can
	// earn in a cycle (MaxPlayerContrib).
	MaxPlayerContrib int
	// BypassPrerequisites lets anyone join either cabal for free
	// (SevenSignsBypassPrerequisites).
	BypassPrerequisites bool
}

// DefaultConfig is the shipped events.properties.
func DefaultConfig() Config {
	return Config{MaxPlayerContrib: 1000000}
}

// Service runs the dialog against the Seven Signs state.
type Service struct {
	state *sevensigns.State
	cfg   Config
}

// New returns the dialog over state.
func New(state *sevensigns.State, cfg Config) *Service {
	return &Service{state: state, cfg: cfg}
}

// Talker is what the dialog reads of, and changes on, the player sending a
// command.
type Talker struct {
	ObjectID  int32
	Inventory *itemcontainer.Inventory
	// ClassLevel counts the occupation changes of the talker's active
	// class: 0 for a starting class.
	ClassLevel int
	// CastleClan is set when the talker's clan owns a castle.
	CastleClan bool
	// NextID allocates the object id of an item the dialog creates.
	NextID func() (int32, error)
}

// Notices a command reports, in the order they happen.
type (
	// SlotsFull refuses an item the inventory has no slot for.
	SlotsFull struct{}
	// NotEnoughAdena refuses a payment in adena or ancient adena.
	NotEnoughAdena struct{}
	// AdenaSpent names the adena a payment took.
	AdenaSpent struct{ Count int }
	// NotEnoughItems refuses to take items the talker holds too few of.
	NotEnoughItems struct{}
	// ItemsSpent names the items a payment or turn-in took.
	ItemsSpent struct {
		ItemID int32
		Count  int
	}
	// ItemPickedUp names an item handed over as picked up.
	ItemPickedUp struct {
		ItemID int32
		Count  int
	}
	// AdenaEarned names the adena handed over.
	AdenaEarned struct{ Count int }
	// AncientAdenaEarned names the ancient adena handed over.
	AncientAdenaEarned struct{ Count int }
	// Joined says which cabal the talker joined.
	Joined struct{ Cabal sevensigns.Cabal }
	// SealChosen says which seal the talker fights for.
	SealChosen struct{ Seal sevensigns.Seal }
	// ContribExceeded refuses stones once the contribution score is capped.
	ContribExceeded struct{}
	// ContribIncreased names the points the stones were worth.
	ContribIncreased struct{ Score int }
)

// Reply is the outcome of one command.
type Reply struct {
	// Notices are the messages to send, in order, before anything else.
	Notices []any
	// Page is the data/html/seven_signs/ page to open, Fill its
	// placeholders as name and value pairs, filled in order before
	// %objectId%. HTML is a page built instead.
	Page string
	Fill []string
	HTML string
	// Release follows the page with ActionFailed: a chat window.
	Release bool
	// Depart moves the talker to Destination, unscattered.
	Depart      bool
	Destination location.Location
	// Save asks for the talker's sign-up row to be saved: a sign-up was
	// made, stones worth points were turned in or their reward collected,
	// and the change only lives in memory until then. A command that changed
	// nothing leaves it unset.
	Save bool
	// Taken are the stacks the command took items from. Their removal is
	// written lazily, so a save writes them first: a crash must never keep a
	// contribution while handing back the stones it was paid with.
	Taken []*item.Instance
	// SignUpErr is the failed insert of a new sign-up's row: the sign-up
	// holds in memory regardless, and the error is only to be logged.
	SignUpErr error
	// Aborted stops the handling outright, as a malformed command does:
	// nothing more is sent, not even the dispatcher's ActionFailed.
	Aborted bool
}

// Command runs "SevenSigns <n> ..." for t at the NPC npcObjectID. dawn is set
// for a Priest of Dawn, whose pages carry the dawn suffix where any other
// NPC's carry the dusk one.
func (s *Service) Command(ctx context.Context, t Talker, npcObjectID int32, dawn bool, command string) Reply {
	h := handler{s: s, t: t, side: "dusk", ctx: ctx}
	if dawn {
		h.side = "dawn"
	}
	h.tokens = strings.FieldsFunc(strings.TrimFunc(command, javaSpace), tokenSpace)
	h.next = 1
	value, ok := h.int()
	if !ok {
		return Reply{Aborted: true}
	}
	switch value {
	case 2:
		return h.buyRecord()
	case 33:
		return h.participate()
	case 34:
		return h.participationFee()
	case 3, 8:
		cabal, ok := h.cabal()
		if !ok {
			return Reply{Aborted: true}
		}
		return chat(signs(value, cabalShortName(cabal)))
	case 4:
		return h.signUp()
	case 5:
		if s.state.PlayerCabal(t.ObjectID) == sevensigns.NoCabal {
			return chat(signs(5, h.side+"_no"))
		}
		return chat(signs(5, h.side))
	case 6:
		return h.contributeStones()
	case 7:
		return h.exchangeAncientAdena(command)
	case 9:
		return h.collectReward()
	case 11:
		return h.huntingGroundTeleport()
	case 16:
		return chat(signs(16, h.side))
	case 17:
		return h.exchangeStonesPage(command)
	case 18:
		return h.exchangeStones(command)
	case 19:
		cabal, ok := h.cabal()
		if !ok {
			return Reply{Aborted: true}
		}
		seal, ok := h.seal()
		if !ok {
			return Reply{Aborted: true}
		}
		return chat(signs(19, sealShortName(seal)+"_"+cabalShortName(cabal)))
	case 20:
		return Reply{HTML: h.sealStatus(npcObjectID)}
	case 21:
		return h.contributeAmount(command)
	}
	return chat(signs(value, ""))
}

// handler is one command's reading of its words and its talker.
type handler struct {
	s      *Service
	t      Talker
	ctx    context.Context
	side   string
	tokens []string
	next   int
	reply  Reply
}

// int reads the next word as a number; ok is false when there is none or
// it does not parse.
func (h *handler) int() (int, bool) {
	if h.next >= len(h.tokens) {
		return 0, false
	}
	n, err := commons.ParseInt(h.tokens[h.next], 32)
	h.next++
	return int(n), err == nil
}

// cabal reads the next word as a cabal's ordinal.
func (h *handler) cabal() (sevensigns.Cabal, bool) {
	n, ok := h.int()
	if !ok || n < int(sevensigns.NoCabal) || n > int(sevensigns.Dawn) {
		return 0, false
	}
	return sevensigns.Cabal(n), true
}

// seal reads the next word as a seal's ordinal, NoSeal included.
func (h *handler) seal() (sevensigns.Seal, bool) {
	n, ok := h.int()
	if !ok || n < int(sevensigns.NoSeal) || n > int(sevensigns.Strife) {
		return 0, false
	}
	return sevensigns.Seal(n), true
}

func (h *handler) notice(n any) { h.reply.Notices = append(h.reply.Notices, n) }

// chat ends the reply with page as a chat window.
func (h *handler) chat(page string) Reply {
	h.reply.Page, h.reply.Release = page, true
	return h.reply
}

// buyRecord sells a Record of Seven Signs ("SevenSigns 2"): it needs a free
// slot, then the price.
func (h *handler) buyRecord() Reply {
	inv := h.t.Inventory
	if inv == nil {
		return Reply{}
	}
	if !inv.ValidateCapacity(1) {
		h.notice(SlotsFull{})
		return h.reply
	}
	if !h.reduceAdena(RecordCost, true) {
		return h.chat(signs(2, h.side+"_no"))
	}
	h.pickUp(RecordID, 1)
	return h.chat(signs(2, h.side))
}

// participate answers "I wish to participate." ("SevenSigns 33 <cabal>"): a
// member of either cabal is told so; unless the prerequisites are waived, a
// starting class may not join, and past the second occupation the castle
// owners' clans may not join the Dusk while everyone else pays to join the
// Dawn.
func (h *handler) participate() Reply {
	cabal, ok := h.cabal()
	if !ok {
		return Reply{Aborted: true}
	}
	if h.s.state.PlayerCabal(h.t.ObjectID) != sevensigns.NoCabal {
		return chat(signs(33, h.side+"_member"))
	}
	if !h.s.cfg.BypassPrerequisites {
		if h.t.ClassLevel == 0 {
			return chat(signs(33, h.side+"_firstclass"))
		}
		if h.t.ClassLevel > 1 {
			if cabal == sevensigns.Dusk && h.t.CastleClan {
				return chat("signs_33_dusk_no.htm")
			}
			if cabal == sevensigns.Dawn && !h.t.CastleClan {
				return chat("signs_33_dawn_fee.htm")
			}
		}
	}
	return chat(signs(33, h.side))
}

// participationFee answers "Pay the participation fee."
// ("SevenSigns 34 <cabal>"): whether the talker holds the Dawn's fee or a
// Certificate of Approval.
func (h *handler) participationFee() Reply {
	inv := h.t.Inventory
	if inv != nil && (inv.Adena() >= JoinDawnCost || inv.HasItem(CertificateOfApprovalID)) {
		return chat("signs_33_dawn.htm")
	}
	return chat("signs_33_dawn_no.htm")
}

// signUp signs the talker up ("SevenSigns 4 <cabal> <seal>"). Unless the
// prerequisites are waived, a castle owner's clan member may not join the
// Dusk, and anyone else joining the Dawn gives up a Certificate of Approval
// or else pays the fee, silently.
func (h *handler) signUp() Reply {
	cabal, ok := h.cabal()
	if !ok {
		return Reply{Aborted: true}
	}
	seal, ok := h.seal()
	// The reference counts the choice toward the seal's votes, which no
	// seal of NoSeal has, and stops with an error once the sign-up is
	// made; no page links it, and it is refused here before anything is
	// taken.
	if !ok || seal == sevensigns.NoSeal {
		return Reply{Aborted: true}
	}
	if !h.s.cfg.BypassPrerequisites {
		if cabal == sevensigns.Dusk && h.t.CastleClan {
			return chat("signs_33_dusk_no.htm")
		}
		if cabal == sevensigns.Dawn && !h.t.CastleClan && !h.destroyItems(CertificateOfApprovalID, 1, false) && !h.reduceAdena(JoinDawnCost, false) {
			return chat("signs_33_dawn_no.htm")
		}
	}
	h.reply.SignUpErr = h.s.state.SetPlayerInfo(h.ctx, h.t.ObjectID, cabal, seal)
	// A new sign-up's row is inserted at once, but a changed one lives in
	// memory until the next save, which the reference's periodic save
	// (#172) would bring within minutes.
	h.reply.Save = true
	h.notice(Joined{Cabal: cabal})
	h.notice(SealChosen{Seal: seal})
	return h.chat(signs(4, cabalShortName(cabal)))
}

// contributeStones answers "SevenSigns 6 <stone>": a capped contribution is
// refused; one color opens the form to turn that color in, and 4 turns in
// every stone the cap leaves room for, red first, then green, then blue.
func (h *handler) contributeStones() Reply {
	stoneType, ok := h.int()
	if !ok {
		return Reply{Aborted: true}
	}
	inv := h.t.Inventory
	if inv == nil {
		return Reply{}
	}
	blue, green, red := firstCount(inv, BlueStoneID), firstCount(inv, GreenStoneID), firstCount(inv, RedStoneID)
	score, signed := h.s.state.PlayerContribScore(h.t.ObjectID)
	limit := h.s.cfg.MaxPlayerContrib
	if score == limit {
		h.notice(ContribExceeded{})
		return h.reply
	}
	var color string
	var count int
	var id int32
	switch stoneType {
	case 1:
		color, id, count = "Blue", BlueStoneID, blue
	case 2:
		color, id, count = "Green", GreenStoneID, green
	case 3:
		color, id, count = "Red", RedStoneID, red
	case 4:
		// The reference adds the stones to a sign-up it then fails to
		// find, losing them; no page offers the command to a talker who
		// never signed up.
		if !signed {
			return Reply{Aborted: true}
		}
		redCount := min((limit-score)/redValue, red)
		left := score + redCount*redValue
		greenCount := min((limit-left)/greenValue, green)
		left += greenCount * greenValue
		blueCount := min((limit-left)/blueValue, blue)
		found := false
		if redCount > 0 {
			found = h.destroyItems(RedStoneID, redCount, true) || found
		}
		if greenCount > 0 {
			found = h.destroyItems(GreenStoneID, greenCount, true) || found
		}
		if blueCount > 0 {
			found = h.destroyItems(BlueStoneID, blueCount, true) || found
		}
		if !found {
			return h.chat(signs(6, h.side+"_no_stones"))
		}
		points := h.addContrib(blueCount, greenCount, redCount)
		h.notice(ContribIncreased{Score: points})
		h.reply.Save = points > 0
		return h.chat(signs(6, h.side))
	}
	h.reply.Page = "signs_6_" + h.side + "_contribute.htm"
	h.reply.Fill = []string{"%stoneColor%", color, "%stoneCount%", itoa(count), "%stoneItemId%", itoa(int(id))}
	return h.reply
}

// contributeAmount turns in the stones the contribution form names
// ("SevenSigns 21 <stoneId> <amount>"), as many as the talker holds and the
// cap leaves room for.
func (h *handler) contributeAmount(command string) Reply {
	itemID, ok := h.int()
	amount, ok2 := commandNumber(command, 19)
	if !ok || !ok2 {
		return chat(signs(6, h.side+"_failure"))
	}
	inv := h.t.Inventory
	if inv == nil {
		return Reply{}
	}
	score, signed := h.s.state.PlayerContribScore(h.t.ObjectID)
	if !signed && isStone(int32(itemID)) {
		// As for contributeStones' 4: the reference loses the stones.
		return Reply{Aborted: true}
	}
	count := min(amount, inv.ItemCount(int32(itemID), -1, true))
	limit := h.s.cfg.MaxPlayerContrib
	most := 0
	switch int32(itemID) {
	case BlueStoneID:
		most = (limit - score) / blueValue
	case GreenStoneID:
		most = (limit - score) / greenValue
	case RedStoneID:
		most = (limit - score) / redValue
	}
	count = min(count, most)
	if !h.destroyItems(int32(itemID), count, true) {
		return h.chat(signs(6, h.side+"_low_stones"))
	}
	switch int32(itemID) {
	case BlueStoneID:
		score = h.addContrib(count, 0, 0)
	case GreenStoneID:
		score = h.addContrib(0, count, 0)
	case RedStoneID:
		score = h.addContrib(0, 0, count)
	}
	h.notice(ContribIncreased{Score: score})
	// A turn-in of no stones, or of none the cap leaves room for, changes
	// nothing to save.
	h.reply.Save = score > 0
	return h.chat(signs(6, h.side))
}

// addContrib turns the stones in, returning the points they were worth, or
// -1 when they would pass the cap, as the reference does.
func (h *handler) addContrib(blue, green, red int) int {
	points, ok := h.s.state.AddPlayerStoneContrib(h.t.ObjectID, blue, green, red, h.s.cfg.MaxPlayerContrib)
	if !ok {
		return -1
	}
	return points
}

// exchangeAncientAdena trades the Black Marketeer's form amount of ancient
// adena for as much adena ("SevenSigns 7 <amount>").
func (h *handler) exchangeAncientAdena(command string) Reply {
	amount, ok := commandNumber(command, 13)
	if !ok || amount < 1 {
		return chat("blkmrkt_3.htm")
	}
	inv := h.t.Inventory
	if inv == nil {
		return Reply{}
	}
	if firstCount(inv, item.AncientAdenaID) < amount {
		return chat("blkmrkt_4.htm")
	}
	if h.reduceAncientAdena(amount) {
		h.notice(AdenaEarned{Count: amount})
		h.add(item.AdenaID, amount)
	}
	return h.chat("blkmrkt_5.htm")
}

// collectReward pays out the ancient adena the talker's stones are worth
// ("SevenSigns 9"), only during seal validation and to the winning cabal.
func (h *handler) collectReward() Reply {
	st := h.s.state
	if st.CurrentPeriod() != sevensigns.SealValidation || st.PlayerCabal(h.t.ObjectID) != st.WinningCabal() {
		return Reply{}
	}
	if _, signed := st.PlayerContribScore(h.t.ObjectID); !signed {
		// The reference fails on the missing sign-up of a talker of no
		// cabal while no cabal won.
		return Reply{Aborted: true}
	}
	reward := st.TakeAncientAdenaReward(h.t.ObjectID)
	if reward <= 0 {
		return h.chat(signs(9, h.side+"_b"))
	}
	h.reply.Save = true
	h.addAncientAdena(reward)
	return h.chat(signs(9, h.side+"_a"))
}

// huntingGroundTeleport takes the talker to x, y, z for its price in
// ancient adena ("SevenSigns 11 <x> <y> <z> <price>"). A malformed command
// is only logged by the reference; here it does nothing.
func (h *handler) huntingGroundTeleport() Reply {
	x, okX := h.int()
	y, okY := h.int()
	z, okZ := h.int()
	cost, okCost := h.int()
	if !okX || !okY || !okZ || !okCost {
		return Reply{}
	}
	if cost > 0 && !h.reduceAncientAdena(cost) {
		return h.reply
	}
	h.reply.Depart, h.reply.Destination = true, location.Location{X: x, Y: y, Z: z}
	return h.reply
}

// exchangeStonesPage answers "SevenSigns 17 <stone>": one color opens the
// form to exchange it, and 4 exchanges every stone held.
func (h *handler) exchangeStonesPage(command string) Reply {
	arg, ok := commandChars(command, 14)
	if !ok {
		return Reply{Aborted: true}
	}
	stoneType, err := commons.Atoi(arg)
	if err != nil {
		return Reply{Aborted: true}
	}
	inv := h.t.Inventory
	if inv == nil {
		return Reply{}
	}
	var color string
	var id int32
	value := 0
	switch stoneType {
	case 1:
		color, id, value = "blue", BlueStoneID, blueValue
	case 2:
		color, id, value = "green", GreenStoneID, greenValue
	case 3:
		color, id, value = "red", RedStoneID, redValue
	case 4:
		blue, green, red := firstCount(inv, BlueStoneID), firstCount(inv, GreenStoneID), firstCount(inv, RedStoneID)
		reward := sevensigns.StoneScore(blue, green, red)
		if reward == 0 {
			return chat(signs(18, h.side+"_no_stones"))
		}
		for _, stone := range [...]struct {
			id    int32
			count int
		}{{BlueStoneID, blue}, {GreenStoneID, green}, {RedStoneID, red}} {
			if stone.count > 0 {
				h.destroyItems(stone.id, stone.count, true)
			}
		}
		h.addAncientAdena(reward)
		return h.chat(signs(18, h.side))
	}
	count := 0
	if id != 0 {
		count = firstCount(inv, id)
	}
	h.reply.Page = "signs_17_" + h.side + ".htm"
	h.reply.Fill = []string{"%stoneColor%", color, "%stoneValue%", itoa(value), "%stoneCount%", itoa(count), "%stoneItemId%", itoa(int(id))}
	return h.reply
}

// exchangeStones trades the exchange form's amount of one color of stones
// for ancient adena ("SevenSigns 18 <stoneId> <amount>").
func (h *handler) exchangeStones(command string) Reply {
	itemID, ok := h.int()
	if !ok {
		return Reply{Aborted: true}
	}
	amount, ok := commandNumber(command, 19)
	if !ok {
		return chat(signs(18, h.side+"_failed"))
	}
	inv := h.t.Inventory
	if inv == nil {
		return Reply{}
	}
	held := inv.ItemByTemplateID(int32(itemID))
	if held == nil {
		return chat(signs(18, h.side+"_no_stones"))
	}
	if amount <= 0 || amount > held.CountValue() {
		return chat(signs(18, h.side+"_low_stones"))
	}
	reward := 0
	switch int32(itemID) {
	case BlueStoneID:
		reward = sevensigns.StoneScore(amount, 0, 0)
	case GreenStoneID:
		reward = sevensigns.StoneScore(0, amount, 0)
	case RedStoneID:
		reward = sevensigns.StoneScore(0, 0, amount)
	}
	if !h.destroyItems(int32(itemID), amount, true) {
		return h.reply
	}
	h.addAncientAdena(reward)
	return h.chat(signs(18, h.side))
}

// sealStatus builds the seal status page ("SevenSigns 20 <cabal>"), each
// seal with its owner, linking back to the NPC's first page.
func (h *handler) sealStatus(npcObjectID int32) string {
	var b strings.Builder
	if h.side == "dawn" {
		b.WriteString(`<html><body>Priest of Dawn:<br><font color="LEVEL">[ Seal Status ]</font><br>`)
	} else {
		b.WriteString(`<html><body>Dusk Priestess:<br><font color="LEVEL">[ Status of the Seals ]</font><br>`)
	}
	owners := h.s.state.SealOwners()
	for i, seal := range sevensigns.Seals {
		owner := "Nothingness"
		if owners[i] != sevensigns.NoCabal {
			owner = cabalFullName(owners[i])
		}
		b.WriteString("[" + sealFullName(seal) + ": " + owner + "]<br>")
	}
	b.WriteString(`<a action="bypass -h npc_` + itoa(int(npcObjectID)) + `_Chat 0">Go back.</a></body></html>`)
	return b.String()
}

// reduceAdena takes count adena, saying so when say is set
// (Player.reduceAdena).
func (h *handler) reduceAdena(count int, say bool) bool {
	inv := h.t.Inventory
	if inv == nil || count > inv.Adena() {
		if say {
			h.notice(NotEnoughAdena{})
		}
		return false
	}
	if count <= 0 {
		return true
	}
	taken := inv.DestroyByTemplateID(item.AdenaID, count)
	if taken == nil {
		return false
	}
	h.reply.Taken = append(h.reply.Taken, taken)
	if say {
		h.notice(AdenaSpent{Count: count})
	}
	return true
}

// reduceAncientAdena takes count ancient adena, saying so
// (Player.reduceAncientAdena).
func (h *handler) reduceAncientAdena(count int) bool {
	inv := h.t.Inventory
	if inv == nil || count > firstCount(inv, item.AncientAdenaID) {
		h.notice(NotEnoughAdena{})
		return false
	}
	if count <= 0 {
		return true
	}
	if inv.DestroyByTemplateID(item.AncientAdenaID, count) == nil {
		return false
	}
	h.notice(ItemsSpent{ItemID: item.AncientAdenaID, Count: count})
	return true
}

// destroyItems takes count of itemID, saying so when say is set
// (Player.destroyItemByItemId): adena goes through reduceAdena, and holding
// too few, or none, refuses. A count of 0 takes nothing from a held stack
// and still names it. A negative count is refused like too few items: the
// reference would add the units to the stack instead.
func (h *handler) destroyItems(itemID int32, count int, say bool) bool {
	if itemID == item.AdenaID {
		return h.reduceAdena(count, say)
	}
	inv := h.t.Inventory
	var held *item.Instance
	if inv != nil {
		held = inv.ItemByTemplateID(itemID)
	}
	if held == nil || count < 0 || held.CountValue() < count || (count > 0 && inv.DestroyItem(held, count) == nil) {
		if say {
			h.notice(NotEnoughItems{})
		}
		return false
	}
	if count > 0 {
		h.reply.Taken = append(h.reply.Taken, held)
	}
	if say {
		h.notice(ItemsSpent{ItemID: itemID, Count: count})
	}
	return true
}

// pickUp hands count of itemID over as picked up (Player.addItem).
func (h *handler) pickUp(itemID int32, count int) {
	if h.t.Inventory == nil {
		return
	}
	if _, ok := h.t.Inventory.Templates().Get(itemID); !ok {
		return
	}
	h.notice(ItemPickedUp{ItemID: itemID, Count: count})
	h.add(itemID, count)
}

// addAncientAdena hands count ancient adena over, naming it even when there
// is none to hand (Player.addAncientAdena).
func (h *handler) addAncientAdena(count int) {
	h.notice(AncientAdenaEarned{Count: count})
	h.add(item.AncientAdenaID, count)
}

// add puts count of itemID into the talker's inventory, nothing for a
// count below 1.
func (h *handler) add(itemID int32, count int) {
	inv := h.t.Inventory
	if inv == nil || count < 1 || h.t.NextID == nil {
		return
	}
	id, err := h.t.NextID()
	if err != nil {
		return
	}
	inv.AddNew(itemID, count, id)
}

// isStone reports whether id is a seal stone.
func isStone(id int32) bool { return id == BlueStoneID || id == GreenStoneID || id == RedStoneID }

// firstCount is the count of the first stack of templateID held, 0 for
// none (getItemByItemId(id).getCount()).
func firstCount(inv *itemcontainer.Inventory, templateID int32) int {
	if held := inv.ItemByTemplateID(templateID); held != nil {
		return held.CountValue()
	}
	return 0
}

// chat is a reply opening page as a chat window.
func chat(page string) Reply { return Reply{Page: page, Release: true} }

// signs names page signs_<value>[_<suffix>].htm.
func signs(value int, suffix string) string {
	name := "signs_" + itoa(value)
	if suffix != "" {
		name += "_" + suffix
	}
	return name + ".htm"
}

// commandNumber reads the number from character begin of command on, as
// the reference does for an edit box's value, which may be padded with
// spaces. ok is false when command is too short or the text does not
// parse.
func commandNumber(command string, begin int) (int, bool) {
	arg, ok := commandChars(command, begin)
	if !ok {
		return 0, false
	}
	n, err := commons.ParseInt(strings.TrimFunc(arg, javaSpace), 32)
	return int(n), err == nil
}

// commandChars returns command from character begin on, counting characters
// as the client encodes them: one per UTF-16 unit. ok is false when command
// is shorter than begin.
func commandChars(command string, begin int) (string, bool) {
	units := utf16.Encode([]rune(command))
	if begin > len(units) {
		return "", false
	}
	return string(utf16.Decode(units[begin:])), true
}

// javaSpace is what String.trim strips: every character up to and including
// the space.
func javaSpace(r rune) bool { return r <= ' ' }

// tokenSpace is what StringTokenizer splits on by default.
func tokenSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

func itoa(n int) string { return strconv.Itoa(n) }

// cabalShortName is the page suffix naming c (CabalType.getShortName).
func cabalShortName(c sevensigns.Cabal) string {
	switch c {
	case sevensigns.Dusk:
		return "dusk"
	case sevensigns.Dawn:
		return "dawn"
	}
	return "No Cabal"
}

// cabalFullName is c's name on the seal status page.
func cabalFullName(c sevensigns.Cabal) string {
	switch c {
	case sevensigns.Dusk:
		return "Revolutionaries of Dusk"
	case sevensigns.Dawn:
		return "Lords of Dawn"
	}
	return "No Cabal"
}

// sealShortName is the page part naming s (SealType.getShortName).
func sealShortName(s sevensigns.Seal) string {
	switch s {
	case sevensigns.Avarice:
		return "Avarice"
	case sevensigns.Gnosis:
		return "Gnosis"
	case sevensigns.Strife:
		return "Strife"
	}
	return ""
}

// sealFullName is s's name on the seal status page.
func sealFullName(s sevensigns.Seal) string {
	if name := sealShortName(s); name != "" {
		return "Seal of " + name
	}
	return ""
}
