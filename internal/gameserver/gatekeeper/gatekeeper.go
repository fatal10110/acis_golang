// Package gatekeeper runs the teleport rules of civilian NPCs: listing the
// destinations an NPC offers, and taking a player to one of them for its
// price or for free. It decides and mutates; the caller turns the returned
// notices into client packets and moves the player.
package gatekeeper

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
)

// ScatterRadius is how far around a destination a teleported player lands.
const ScatterRadius = 20

// Service applies the teleport rules against the loaded destinations.
type Service struct {
	// tables are the destinations; //reload teleport swaps them.
	tables atomic.Pointer[destinations]
	free   bool
	now    func() time.Time
}

// destinations is one loaded snapshot of both destination tables.
type destinations struct {
	teleports travel.TeleportTable
	instants  travel.InstantTable
}

// NewService returns a Service over teleports and instants. free waives
// every price (npcs.properties FreeTeleport). now supplies the local wall
// clock the weekend half-price hours are read from; nil means time.Now.
func NewService(teleports travel.TeleportTable, instants travel.InstantTable, free bool, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	s := &Service{free: free, now: now}
	s.SetTables(teleports, instants)
	return s
}

// SetTables replaces the destinations with teleports and instants, at once
// for every later request: TeleportData.reload and
// InstantTeleportData.reload. A request already running keeps the tables
// it started with.
func (s *Service) SetTables(teleports travel.TeleportTable, instants travel.InstantTable) {
	s.tables.Store(&destinations{teleports: teleports, instants: instants})
}

// Notices a teleport reports, in the order they happen.
type (
	// NotEnoughAdena refuses a trip the player cannot pay in adena for.
	NotEnoughAdena struct{}
	// AdenaSpent names the adena a trip took.
	AdenaSpent struct{ Count int }
	// NotEnoughItems refuses a trip the player holds too few of the price
	// item for.
	NotEnoughItems struct{}
	// ItemsSpent names the price items a trip took.
	ItemsSpent struct {
		ItemID int32
		Count  int
	}
)

// Trip is the outcome of one teleport request.
type Trip struct {
	// Notices are the messages to send, in order, before anything else.
	Notices []any
	// Depart moves the player to Destination, scattered by ScatterRadius.
	Depart      bool
	Destination location.Location
	// Release answers ActionFailed of the request's own, after the move
	// when there is one.
	Release bool
	// Unported names a destination priced in ancient adena, whose price
	// depends on the Seven Signs seal owners, not in place yet: nothing is
	// taken and nobody moves.
	Unported bool
}

// Window returns the page listing the destinations of kind npcID offers,
// for the NPC objectID: each one a link to its "teleport <index>" command,
// index counting every destination of npcID, with its price unless
// teleports are free. ok is false when npcID offers no destination, and
// unported is true when a listed price is in ancient adena.
func (s *Service) Window(objectID int32, npcID int, kind travel.Kind) (page string, ok, unported bool) {
	list, ok := s.tables.Load().teleports[npcID]
	if !ok {
		return "", false, false
	}
	id := strconv.Itoa(int(objectID))
	now := s.now()
	var b strings.Builder
	b.WriteString("<html><body>&$556;<br><br>")
	for index, t := range list {
		if t.Kind != kind {
			continue
		}
		b.WriteString(`<a action="bypass -h npc_` + id + `_teleport ` + strconv.Itoa(index) + `" msg="811;` + t.Description + `">` + t.Description)
		if !s.free {
			price, priced := s.price(t, now)
			if !priced {
				return "", true, true
			}
			if price > 0 {
				b.WriteString(" - " + strconv.Itoa(price) + " &#" + strconv.Itoa(t.PriceID) + ";")
			}
		}
		b.WriteString("</a><br1>")
	}
	b.WriteString("</body></html>")
	return b.String(), true, false
}

// Teleport takes c to destination index of npcID's list. An NPC without a
// list, or an index past its end, answers nothing; a negative index, or
// one equal to the list's length, is only released. A priced destination,
// unless teleports are free, is paid first: in adena, or in its price item.
// A trip c cannot pay for is refused and released; a paid or free trip
// departs, then releases.
func (s *Service) Teleport(c *player.Character, npcID, index int) Trip {
	list, ok := s.tables.Load().teleports[npcID]
	if !ok || index > len(list) {
		return Trip{}
	}
	if index < 0 || index == len(list) {
		return Trip{Release: true}
	}
	t := list[index]
	// ponytail: a destination tied to a castle (CastleID) is refused with
	// CANNOT_PORT_VILLAGE_IN_SIEGE while that castle's siege runs (#3375).
	trip := Trip{Depart: true, Destination: t.Location, Release: true}
	if s.free || t.PriceCount == 0 {
		return trip
	}
	price, priced := s.price(t, s.now())
	if !priced {
		return Trip{Unported: true}
	}
	trip.Notices, trip.Depart = pay(c, int32(t.PriceID), price)
	return trip
}

// Instant takes the player to instant destination index of npcID's list,
// free and unreleased. An NPC without a list, or an index past its end,
// answers nothing; a negative index, or one equal to the list's length, is
// only released.
func (s *Service) Instant(npcID, index int) Trip {
	list, ok := s.tables.Load().instants[npcID]
	if !ok || index > len(list) {
		return Trip{}
	}
	if index < 0 || index == len(list) {
		return Trip{Release: true}
	}
	return Trip{Depart: true, Destination: list[index]}
}

// price is t's price at now. ok is false for a price in ancient adena,
// which depends on the talker's Seven Signs standing (#2948).
func (s *Service) price(t travel.Teleport, now time.Time) (int, bool) {
	if t.PriceID == int(item.AncientAdenaID) {
		return 0, false
	}
	return t.CalculatedPrice(now), true
}

// pay takes count of itemID from c, reporting whether it did and what to
// say about it. The check and the take are one step on c's own queue, so
// nothing spent in between can buy a free trip.
func pay(c *player.Character, itemID int32, count int) ([]any, bool) {
	inv := c.Inventory()
	if itemID == item.AdenaID {
		if inv == nil || count > inv.Adena() {
			return []any{NotEnoughAdena{}}, false
		}
		if inv.DestroyByTemplateID(item.AdenaID, count) == nil {
			return nil, false
		}
		return []any{AdenaSpent{Count: count}}, true
	}
	if inv == nil || inv.DestroyByTemplateID(itemID, count) == nil {
		return []any{NotEnoughItems{}}, false
	}
	return []any{ItemsSpent{ItemID: itemID, Count: count}}, true
}
