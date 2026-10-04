package lottery

import (
	"math"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Lottery seller pages, data/html/default/<npcId>-<page>.htm.
const (
	PageIntro        = 1 // what a ticket costs, the way to the form
	PageInstructions = 2 // the rules and each place's share
	PageJackpot      = 3 // the running round's jackpot and drawing date
	PageClaims       = 4 // the talker's tickets of past rounds
	PageForm         = 5 // the ticket form's twenty number buttons
)

// Dialog commands after "Loto ": 0 opens the intro and clears the form,
// 1 to 20 press a number on the form and 21 opens it, 22 buys the ticket
// the form holds, 23 to 25 open the jackpot, claims and instructions
// pages, and a larger one claims the ticket of that object id.
const (
	CommandIntro        = 0
	CommandForm         = 21
	CommandBuy          = 22
	CommandJackpot      = 23
	CommandClaims       = 24
	CommandInstructions = 25
)

// Command returns the number a "Loto <n>" dialog command carries: 0 when
// there is none or it does not parse.
func Command(command string) int {
	if len(command) <= len("Loto ") {
		return 0
	}
	n, err := commons.ParseInt(command[len("Loto "):], 32)
	if err != nil {
		return 0
	}
	return int(n)
}

// Fill sets the placeholders every lottery page shares: the seller's object
// id, then the round, its jackpot, the ticket price and the drawing date.
func (l *Lottery) Fill(page string, objectID int32) string {
	st := l.Status()
	page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(objectID)))
	page = strings.ReplaceAll(page, "%race%", strconv.Itoa(int(st.Round)))
	page = strings.ReplaceAll(page, "%adena%", strconv.Itoa(int(st.Prize)))
	page = strings.ReplaceAll(page, "%ticket_price%", strconv.Itoa(int(l.cfg.TicketPrice)))
	return strings.ReplaceAll(page, "%enddate%", l.FormatDate(st.EndDate))
}

// Instructions sets the instructions page's prizes: each of the first three
// places' share of the jackpot as a percentage, and the fourth place's
// fixed prize.
func (l *Lottery) Instructions(page string) string {
	page = strings.ReplaceAll(page, "%prize5%", pageDouble(l.cfg.FiveNumberRate*100))
	page = strings.ReplaceAll(page, "%prize4%", pageDouble(l.cfg.FourNumberRate*100))
	page = strings.ReplaceAll(page, "%prize3%", pageDouble(l.cfg.ThreeNumberRate*100))
	return strings.ReplaceAll(page, "%prize2%", strconv.Itoa(int(l.cfg.TwoAndOneNumberPrize)))
}

// HeldTicket is a lottery ticket in a player's inventory.
type HeldTicket struct {
	ObjectID int32
	Round    int32
	Numbers  Numbers
}

// ClaimList lists the tickets of rounds before the running one, in the
// order given, as the claims page's links: each names its round, its
// numbers and what it won, and claims the ticket. With none it says so.
func (l *Lottery) ClaimList(tickets []HeldTicket) string {
	round := l.Status().Round
	var sb strings.Builder
	for _, t := range tickets {
		if t.Round >= round {
			continue
		}
		sb.WriteString(`<a action="bypass -h npc_%objectId%_Loto ` + strconv.Itoa(int(t.ObjectID)) + `">` + strconv.Itoa(int(t.Round)) + " Event Number ")
		for _, n := range Decode(t.Numbers) {
			sb.WriteString(strconv.Itoa(n) + " ")
		}
		if place, prize := l.Check(t.Round, t.Numbers); place > 0 {
			sb.WriteString("- " + placeLabels[place] + " " + strconv.Itoa(int(prize)) + "a.")
		}
		sb.WriteString("</a><br>")
	}
	if sb.Len() == 0 {
		return "There is no winning lottery ticket...<br>"
	}
	return sb.String()
}

// pageDouble writes v the way a page shows a floating-point value: the
// shortest decimal that reads back as v, with at least one digit after the
// point, in scientific form ("1.0E7") outside [0.001, 10000000).
func pageDouble(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	if a := math.Abs(v); a == 0 || (a >= 1e-3 && a < 1e7) {
		s := strconv.FormatFloat(v, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	mantissa, exp, _ := strings.Cut(strconv.FormatFloat(v, 'E', -1, 64), "E")
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	e, _ := strconv.Atoi(exp)
	return mantissa + "E" + strconv.Itoa(e)
}
