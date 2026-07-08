package route

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Dock names a configured boat dock.
type Dock string

// Known boat dock names.
const (
	DockTalkingIsland Dock = "TALKING_ISLAND"
	DockGludin        Dock = "GLUDIN"
	DockRune          Dock = "RUNE"
	DockGiran         Dock = "GIRAN"
	DockPrimeval      Dock = "PRIMEVAL"
	DockInnadril      Dock = "INNADRIL"
)

var docks = map[Dock]struct{}{
	DockTalkingIsland: {},
	DockGludin:        {},
	DockRune:          {},
	DockGiran:         {},
	DockPrimeval:      {},
	DockInnadril:      {},
}

// ParseDock validates a dock name.
func ParseDock(s string) (Dock, error) {
	d := Dock(s)
	if _, ok := docks[d]; !ok {
		return "", fmt.Errorf("route: unknown dock %q", s)
	}
	return d, nil
}

// ScheduledMessage is a boat system message sent after Delay seconds.
type ScheduledMessage struct {
	ID, Delay int
}

// BoatLocation is one node in a boat route.
type BoatLocation struct {
	location.Location
	Speed, Rotation   int
	BusyMessage       int
	ArrivalMessages   []int
	DepartureMessages []int
	Scheduled         []ScheduledMessage
}

// NewBoatLocation builds a BoatLocation from set. x, y and z are required;
// speed defaults to 350 and rotation defaults to 4000.
func NewBoatLocation(set *commons.StatSet) (BoatLocation, error) {
	loc, err := location.NewLocation(set)
	if err != nil {
		return BoatLocation{}, fmt.Errorf("route: boat location: %w", err)
	}
	speed, err := set.GetIntDefault("speed", 350)
	if err != nil {
		return BoatLocation{}, fmt.Errorf("route: boat location: %w", err)
	}
	rotation, err := set.GetIntDefault("rotation", 4000)
	if err != nil {
		return BoatLocation{}, fmt.Errorf("route: boat location: %w", err)
	}
	busy, err := set.GetIntDefault("busy", 0)
	if err != nil {
		return BoatLocation{}, fmt.Errorf("route: boat location: %w", err)
	}
	arrival, err := parseMessageList(set.GetStringArrayDefault("arrival", nil))
	if err != nil {
		return BoatLocation{}, fmt.Errorf("route: boat location arrival: %w", err)
	}
	departure, err := parseMessageList(set.GetStringArrayDefault("departure", nil))
	if err != nil {
		return BoatLocation{}, fmt.Errorf("route: boat location departure: %w", err)
	}
	scheduled, err := parseScheduled(set.GetStringDefault("scheduled", ""))
	if err != nil {
		return BoatLocation{}, fmt.Errorf("route: boat location scheduled: %w", err)
	}
	return BoatLocation{
		Location:          loc,
		Speed:             speed,
		Rotation:          rotation,
		BusyMessage:       busy,
		ArrivalMessages:   arrival,
		DepartureMessages: departure,
		Scheduled:         scheduled,
	}, nil
}

func parseMessageList(parts []string) ([]int, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func parseScheduled(raw string) ([]ScheduledMessage, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ";")
	out := make([]ScheduledMessage, 0, len(parts))
	for _, p := range parts {
		id, delay, err := parseDashPair(p)
		if err != nil {
			return nil, err
		}
		out = append(out, ScheduledMessage{ID: id, Delay: delay})
	}
	return out, nil
}

func parseDashPair(raw string) (int, int, error) {
	left, right, ok := strings.Cut(raw, "-")
	if !ok {
		return 0, 0, fmt.Errorf("%q must be formatted id-delay", raw)
	}
	a, err := strconv.Atoi(left)
	if err != nil {
		return 0, 0, err
	}
	b, err := strconv.Atoi(right)
	if err != nil {
		return 0, 0, err
	}
	return a, b, nil
}

// BoatRoute is one directional route between docks.
type BoatRoute struct {
	Dock   Dock
	ItemID int
	Nodes  []BoatLocation
}

// BoatItinerary is one boat itinerary. Two routes means a round trip; one
// route means one-way service.
type BoatItinerary struct {
	Heading int
	Routes  []BoatRoute
}

// WalkerLocation is one node in a walking NPC route.
type WalkerLocation struct {
	location.Location
	DelayMillis int
	NPCStringID int
	SocialID    int
}

// NewWalkerLocation builds a WalkerLocation from set. x, y and z are
// required; delay seconds are converted to milliseconds.
func NewWalkerLocation(set *commons.StatSet) (WalkerLocation, error) {
	loc, err := location.NewLocation(set)
	if err != nil {
		return WalkerLocation{}, fmt.Errorf("route: walker location: %w", err)
	}
	delay, err := set.GetIntDefault("delay", 0)
	if err != nil {
		return WalkerLocation{}, fmt.Errorf("route: walker location: %w", err)
	}
	fstring, err := set.GetIntDefault("fstring", 0)
	if err != nil {
		return WalkerLocation{}, fmt.Errorf("route: walker location: %w", err)
	}
	socialID, err := set.GetIntDefault("socialId", 0)
	if err != nil {
		return WalkerLocation{}, fmt.Errorf("route: walker location: %w", err)
	}
	return WalkerLocation{
		Location:    loc,
		DelayMillis: delay * 1000,
		NPCStringID: fstring,
		SocialID:    socialID,
	}, nil
}

// WalkerRoutes stores walking routes keyed by route name then npc name.
type WalkerRoutes map[string]map[string][]WalkerLocation

// NPCCount returns how many named NPC route entries are loaded.
func (r WalkerRoutes) NPCCount() int {
	var n int
	for _, byNPC := range r {
		n += len(byNPC)
	}
	return n
}

// NodeCount returns how many walker nodes are loaded.
func (r WalkerRoutes) NodeCount() int {
	var n int
	for _, byNPC := range r {
		for _, nodes := range byNPC {
			n += len(nodes)
		}
	}
	return n
}
