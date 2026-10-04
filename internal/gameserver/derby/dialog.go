package derby

import (
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// TicketItemID is the monster race ticket: its enchant level is the race,
// its first custom type the lane and its second the price in hundreds of
// adena.
const TicketItemID = 4443

// ticketPrices are the eight ticket prices, picked by their 1-based step.
var ticketPrices = [Lanes]int{100, 500, 1000, 5000, 10000, 20000, 50000, 100000}

// Command reports whether command is one a race manager answers itself.
func Command(command string) bool {
	switch command {
	case "ShowOdds", "ShowInfo", "ViewHistory":
		return true
	}
	return strings.HasPrefix(command, "BuyTicket") || strings.HasPrefix(command, "ShowTicket") || strings.HasPrefix(command, "CalculateWin")
}

// Pages reads a datapack page as an HTML window takes it, reporting
// whether it exists.
type Pages interface {
	Get(path string) (string, bool)
}

// Picks is the ticket a player is choosing: the lane, then the price step
// (1 to 8), 0 when not chosen.
type Picks struct {
	Lane  int
	Price int
}

// Ticket is an item the race manager reads as a ticket: its enchant level
// is the race, its first custom type the lane and its second the price in
// hundreds of adena.
type Ticket struct {
	ObjectID int32
	Race     int
	Lane     int
	Hundreds int
}

// Holder is the inventory of the player talking to a race manager.
type Holder interface {
	// Tickets returns every race ticket held, in inventory order.
	Tickets() []Ticket
	// Item returns the held item objectID, read as a ticket whatever it is.
	Item(objectID int32) (Ticket, bool)
}

// Purchase is a ticket bought: Price adena taken, then a ticket for Lane of
// Race handed over and Price added to Lane's stake.
type Purchase struct {
	Race  int
	Lane  int
	Price int
}

// Redemption trades ticket ObjectID for Payout adena.
type Redemption struct {
	ObjectID int32
	Payout   int32
}

// Reply is the race manager's answer to one command, carried out in field
// order: Notice, then Buy or Redeem, then Page or the first chat page.
type Reply struct {
	// Aborted is a command too malformed to answer: nothing at all is
	// sent, not even the dialog's closing ActionFailed.
	Aborted bool
	// Notice is a system message id to send first, 0 for none.
	Notice int
	Buy    *Purchase
	Redeem *Redemption
	// Page is a window to open, followed by ActionFailed.
	Page string
	// Chat ends the answer with the NPC's first chat page.
	Chat bool
}

// dialog is one command being answered, under the track's lock.
type dialog struct {
	t        *Track
	pages    Pages
	npcID    int
	objectID int32
}

// Bypass answers command, the part of a dialog link after the race
// manager's object id, for race manager npcID (objectID). picks is the
// talker's ticket being chosen, and holder its inventory.
func (t *Track) Bypass(pages Pages, npcID int, objectID int32, picks *Picks, holder Holder, command string) Reply {
	t.mu.Lock()
	defer t.mu.Unlock()
	d := dialog{t: t, pages: pages, npcID: npcID, objectID: objectID}
	switch {
	case strings.HasPrefix(command, "BuyTicket"):
		return d.buyTicket(picks, command)
	case command == "ShowOdds":
		return d.showOdds()
	case command == "ShowInfo":
		page, ok := d.runnerPage(6)
		if !ok {
			return Reply{Aborted: true}
		}
		return Reply{Page: d.fill(page)}
	case command == "ShowTickets":
		return d.showTickets(holder)
	case strings.HasPrefix(command, "ShowTicket"):
		return d.showTicket(holder, command)
	case strings.HasPrefix(command, "CalculateWin"):
		return d.calculateWin(holder, command)
	default: // ViewHistory
		return d.viewHistory()
	}
}

// buyTicket walks the ticket purchase: BuyTicket 0 (or 1 to 9, picking a
// lane) lists the runners, 10 (or 11 to 18, picking a price) the prices,
// 20 the summary, and anything past 20 buys the ticket. Tickets sell only
// while bets are accepted. A step taken before the lane (or price) it needs
// was picked starts over, or answers nothing.
func (d dialog) buyTicket(picks *Picks, command string) Reply {
	t := d.t
	if t.state != acceptingBets {
		return Reply{Notice: MsgTicketsNotAvailable, Chat: true}
	}
	val, ok := commandInt(command, 10)
	if !ok {
		return Reply{Aborted: true}
	}
	if val == 0 {
		*picks = Picks{}
	}
	if (val == 10 && picks.Lane == 0) || (val == 20 && picks.Lane == 0 && picks.Price == 0) {
		val = 0
	}
	var page string
	switch {
	case val < 10:
		var ok bool
		if page, ok = d.runnerPage(2); !ok {
			return Reply{Aborted: true}
		}
		if val == 0 {
			page = replace(page, "No1", "")
		} else {
			page = replace(page, "No1", strconv.Itoa(val))
			picks.Lane = val
		}
	case val < 20:
		if picks.Lane == 0 {
			return Reply{}
		}
		name, ok := d.runnerName(picks.Lane - 1)
		if !ok || (val != 10 && val-11 >= len(ticketPrices)) {
			return Reply{Aborted: true}
		}
		page = d.load(3)
		page = replace(page, "0place", strconv.Itoa(picks.Lane))
		page = replace(page, "Mob1", name)
		if val == 10 {
			page = replace(page, "0adena", "")
		} else {
			page = replace(page, "0adena", strconv.Itoa(ticketPrices[val-11]))
			picks.Price = val - 10
		}
	case val == 20:
		if picks.Lane == 0 || picks.Price == 0 {
			return Reply{}
		}
		name, ok := d.runnerName(picks.Lane - 1)
		if !ok {
			return Reply{Aborted: true}
		}
		price := ticketPrices[picks.Price-1]
		page = d.load(4)
		page = replace(page, "0place", strconv.Itoa(picks.Lane))
		page = replace(page, "Mob1", name)
		page = replace(page, "0adena", strconv.Itoa(price))
		page = replace(page, "0tax", "0")
		page = replace(page, "0total", strconv.Itoa(price))
	default:
		if picks.Lane == 0 || picks.Price == 0 {
			return Reply{}
		}
		// A lane past the eight runners would stake on a lane no race
		// runs, which the odds are not drawn for.
		if picks.Lane < 1 || picks.Lane > Lanes {
			return Reply{Aborted: true}
		}
		return Reply{Buy: &Purchase{Race: t.raceNumber, Lane: picks.Lane, Price: ticketPrices[picks.Price-1]}, Chat: true}
	}
	page = replace(page, "1race", strconv.Itoa(t.raceNumber))
	return Reply{Page: d.fill(page)}
}

// showOdds lists the runners with their odds, once bets are closed.
func (d dialog) showOdds() Reply {
	t := d.t
	if t.state == acceptingBets {
		return Reply{Notice: MsgNoPayoutInfo, Chat: true}
	}
	page := d.load(5)
	for i := range Lanes {
		n := strconv.Itoa(i + 1)
		name, ok := d.runnerName(i)
		if !ok || i >= len(t.odds) {
			return Reply{Aborted: true}
		}
		page = replace(page, "Mob"+n, name)
		odd := "&$804;"
		if t.odds[i] > 0 {
			odd = commons.JavaFixed(t.odds[i], 1)
		}
		page = replace(page, "Odd"+n, odd)
	}
	page = replace(page, "1race", strconv.Itoa(t.raceNumber))
	return Reply{Page: d.fill(page)}
}

// showTickets lists the talker's tickets of every race but the current one.
func (d dialog) showTickets(holder Holder) Reply {
	var rows strings.Builder
	for _, ticket := range holder.Tickets() {
		if ticket.Race == d.t.raceNumber {
			continue
		}
		rows.WriteString(`<tr><td><a action="bypass -h npc_%objectId%_ShowTicket ` + itoa32(ticket.ObjectID) + `">` + strconv.Itoa(ticket.Race) +
			` Race Number</a></td><td align=right><font color="LEVEL">` + strconv.Itoa(ticket.Lane) +
			`</font> Number</td><td align=right><font color="LEVEL">` + strconv.Itoa(ticket.Hundreds*100) + `</font> Adena</td></tr>`)
	}
	page := replace(d.load(7), "%tickets%", rows.String())
	return Reply{Page: d.fill(page)}
}

// showTicket shows what ticket ShowTicket <objectId> pays: the winner's
// odds on the winning lane, 0.01 on any other.
func (d dialog) showTicket(holder Holder, command string) Reply {
	val, ok := commandInt(command, 11)
	if !ok {
		return Reply{Aborted: true}
	}
	ticket, h, ok := d.ticketRecord(holder, val)
	if !ok {
		return Reply{Chat: true}
	}
	odd := "0.01"
	if ticket.Lane == h.First+1 {
		odd = commons.JavaFixed(h.OddRate, 2)
	}
	page := d.load(8)
	page = replace(page, "%raceId%", strconv.Itoa(ticket.Race))
	page = replace(page, "%lane%", strconv.Itoa(ticket.Lane))
	page = replace(page, "%bet%", strconv.Itoa(ticket.Hundreds*100))
	page = replace(page, "%firstLane%", strconv.Itoa(h.First+1))
	page = replace(page, "%odd%", odd)
	page = replace(page, "%objectId%", itoa32(d.objectID))
	page = replace(page, "%ticketObjectId%", strconv.Itoa(val))
	return Reply{Page: page}
}

// calculateWin trades ticket CalculateWin <objectId> for its price times
// the winner's odds on the winning lane, a hundredth of it on any other,
// then opens the first chat page.
func (d dialog) calculateWin(holder Holder, command string) Reply {
	val, ok := commandInt(command, 13)
	if !ok {
		return Reply{Aborted: true}
	}
	ticket, h, ok := d.ticketRecord(holder, val)
	if !ok {
		return Reply{Chat: true}
	}
	rate := 0.01
	if ticket.Lane == h.First+1 {
		rate = h.OddRate
	}
	payout := commons.JavaInt(float64(ticket.Hundreds*100) * rate)
	return Reply{Redeem: &Redemption{ObjectID: ticket.ObjectID, Payout: payout}, Chat: true}
}

// ticketRecord finds held item objectID and the record of its race. It
// reports false for an object id of 0, an item not held, or a race with no
// record.
func (d dialog) ticketRecord(holder Holder, objectID int) (Ticket, History, bool) {
	if objectID == 0 {
		return Ticket{}, History{}, false
	}
	ticket, ok := holder.Item(int32(objectID))
	if !ok {
		return Ticket{}, History{}, false
	}
	h, ok := d.t.history[ticket.Race]
	if !ok {
		return Ticket{}, History{}, false
	}
	return ticket, *h, true
}

// viewHistory lists the latest eight races, newest first; the current race
// shows no winner.
func (d dialog) viewHistory() Reply {
	var rows strings.Builder
	for _, h := range d.t.lastHistoryLocked() {
		first, second := h.First+1, h.Second+1
		if h.RaceID == d.t.raceNumber {
			first, second = 0, 0
		}
		rows.WriteString(`<tr><td><font color="LEVEL">` + strconv.Itoa(h.RaceID) + `</font> th</td><td><font color="LEVEL">` + strconv.Itoa(first) +
			`</font> Lane </td><td><font color="LEVEL">` + strconv.Itoa(second) + `</font> Lane</td><td align=right><font color=00ffff>` +
			commons.JavaFixed(h.OddRate, 2) + `</font> Times</td></tr>`)
	}
	page := replace(d.load(9), "%infos%", rows.String())
	return Reply{Page: d.fill(page)}
}

// runnerPage loads page val with the eight runners' names. It reports false
// before the first race.
func (d dialog) runnerPage(val int) (string, bool) {
	page := d.load(val)
	for i := range Lanes {
		name, ok := d.runnerName(i)
		if !ok {
			return "", false
		}
		page = replace(page, "Mob"+strconv.Itoa(i+1), name)
	}
	return page, true
}

// runnerName is the name of the current race's runner in lane index i (0
// to 7); false for any other index or before the first race.
func (d dialog) runnerName(i int) (string, bool) {
	if i < 0 || i >= len(d.t.chosen) {
		return "", false
	}
	return d.t.chosen[i].Name, true
}

// load reads the race manager's page val: default/<npcId>-<val>.htm, or
// the default NPC page when it does not exist, or the missing-page notice.
func (d dialog) load(val int) string {
	path := "data/html/default/" + strconv.Itoa(d.npcID) + "-" + strconv.Itoa(val) + ".htm"
	if _, ok := d.pages.Get(path); !ok {
		path = "data/html/npcdefault.htm"
	}
	if page, ok := d.pages.Get(path); ok {
		return page
	}
	return "<html><body>My html is missing:<br>" + path + "</body></html>"
}

// fill sets the race manager's object id in page.
func (d dialog) fill(page string) string {
	return replace(page, "%objectId%", itoa32(d.objectID))
}

// replace replaces every pattern in page with value. Every pattern the
// race manager fills is plain text.
func replace(page, pattern, value string) string {
	return strings.ReplaceAll(page, pattern, value)
}

func itoa32(n int32) string { return strconv.Itoa(int(n)) }

// commandInt reads the number command carries from its begin-th character
// on, counting characters as the client encodes them (one per UTF-16
// unit). ok is false when the command is too short or the rest is no
// number.
func commandInt(command string, begin int) (int, bool) {
	units := utf16.Encode([]rune(command))
	if begin > len(units) {
		return 0, false
	}
	n, err := commons.ParseInt(string(utf16.Decode(units[begin:])), 32)
	if err != nil {
		return 0, false
	}
	return int(n), true
}
