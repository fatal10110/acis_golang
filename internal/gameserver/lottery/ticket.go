package lottery

import (
	"strconv"
	"strings"
)

// TicketID is the Lottery Ticket item. A ticket keeps its round in custom
// type 1 and its five numbers as two bit masks: numbers 1 to 16 in its
// enchant level, 17 to 20 in custom type 2.
const TicketID int32 = 4442

// Numbers holds five lottery numbers as the two masks a ticket and a drawing
// keep: bit n-1 of Low for a number n up to 16, bit n-17 of High for 17 to
// 20.
type Numbers struct {
	Low, High int32
}

// numbersOf returns the masks of nums, each between 1 and 20.
func numbersOf(nums []int) Numbers {
	var n Numbers
	for _, v := range nums {
		if v < 17 {
			n.Low += 1 << (v - 1)
		} else {
			n.High += 1 << (v - 17)
		}
	}
	return n
}

// Decode lists the numbers n holds, lowest first, in five slots; a slot no
// number fills reads 0. Numbers past the fifth are not listed.
func Decode(n Numbers) [5]int {
	var out [5]int
	id := 0
	add := func(mask int32, first int) {
		for nr := first; mask > 0; nr++ {
			if mask%2 != 0 && id < len(out) {
				out[id] = nr
				id++
			}
			mask /= 2
		}
	}
	add(n.Low, 1)
	add(n.High, 17)
	return out
}

// matches counts the numbers ticket shares with drawn, reading the low
// sixteen places of each mask the way a halving loop does.
func matches(ticket, drawn Numbers) int {
	low, high := ticket.Low&drawn.Low, ticket.High&drawn.High
	if low == 0 && high == 0 {
		return 0
	}
	count := 0
	for range 16 {
		if low%2 != 0 {
			count++
		}
		if high%2 != 0 {
			count++
		}
		low /= 2
		high /= 2
	}
	return count
}

// Draw is a finished round's winning numbers and the prize each of the first
// three places pays per ticket.
type Draw struct {
	Numbers
	Prize1, Prize2, Prize3 int32
}

// Place is what a ticket won: 1 to 3 for five, four or three matching
// numbers, 4 for one or two, 0 for none.
type Place int

// placeLabels name each winning place in the claim list.
var placeLabels = map[Place]string{1: "1st Prize", 2: "2nd Prize", 3: "3th Prize", 4: "4th Prize"}

// Picks is a player's choice of up to five numbers on the ticket form, in
// the order they were pressed; an empty slot is 0.
type Picks [5]int

// Press toggles number val, 1 to 20: a chosen number is dropped, another one
// takes the first empty slot unless five are chosen. 21, the form's own
// page, changes nothing.
func (p *Picks) Press(val int) {
	count, found := 0, false
	for i, v := range p {
		switch {
		case v == val:
			p[i] = 0
			found = true
		case v > 0:
			count++
		}
	}
	if count >= len(p) || found || val > 20 {
		return
	}
	for i, v := range p {
		if v == 0 {
			p[i] = val
			return
		}
	}
}

// Numbers returns the masks of the five chosen numbers; ok is false while a
// slot is empty.
func (p Picks) Numbers() (Numbers, bool) {
	for _, v := range p {
		if v == 0 {
			return Numbers{}, false
		}
	}
	return numbersOf(p[:]), true
}

// Mark shows the chosen numbers checked on the ticket form page: each one's
// button swaps its two images, and with all five chosen the form's return
// link becomes the purchase link.
func (p Picks) Mark(page string) string {
	count := 0
	for _, v := range p {
		if v <= 0 {
			continue
		}
		count++
		button := strconv.Itoa(v)
		if v < 10 {
			button = "0" + button
		}
		page = strings.ReplaceAll(page,
			`fore="L2UI.lottoNum`+button+`" back="L2UI.lottoNum`+button+`a_check"`,
			`fore="L2UI.lottoNum`+button+`a_check" back="L2UI.lottoNum`+button+`"`)
	}
	if count == len(p) {
		page = strings.ReplaceAll(page, `0">Return`, `22">The winner selected the numbers above.`)
	}
	return page
}
