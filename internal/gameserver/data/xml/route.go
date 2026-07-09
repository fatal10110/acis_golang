package xml

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

type boatRouteFile struct {
	Itineraries []boatItineraryElement `xml:"itinerary"`
}

type boatItineraryElement struct {
	Dock1   string             `xml:"dock1,attr"`
	Dock2   string             `xml:"dock2,attr"`
	Item1   int                `xml:"item1,attr"`
	Item2   int                `xml:"item2,attr"`
	Heading int                `xml:"heading,attr"`
	Routes  []boatRouteElement `xml:"route"`
}

type boatRouteElement struct {
	Nodes []attrsElement `xml:"node"`
}

// LoadBoatRoutes parses boat route itineraries.
func LoadBoatRoutes(path string) ([]route.BoatItinerary, error) {
	var doc boatRouteFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("boat routes: %w", err)
	}

	itineraries := make([]route.BoatItinerary, 0, len(doc.Itineraries))
	for _, el := range doc.Itineraries {
		itinerary, err := buildBoatItinerary(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		itineraries = append(itineraries, itinerary)
	}
	return itineraries, nil
}

func buildBoatItinerary(el boatItineraryElement) (route.BoatItinerary, error) {
	dock1, err := route.ParseDock(el.Dock1)
	if err != nil {
		return route.BoatItinerary{}, err
	}
	routeCount := 1
	docks := []route.Dock{dock1}
	items := []int{el.Item1}
	if el.Dock2 != "" {
		dock2, err := route.ParseDock(el.Dock2)
		if err != nil {
			return route.BoatItinerary{}, err
		}
		routeCount = 2
		docks = append(docks, dock2)
		items = append(items, el.Item2)
	}
	if len(el.Routes) != routeCount {
		return route.BoatItinerary{}, fmt.Errorf("boat itinerary %s: got %d routes, want %d", el.Dock1, len(el.Routes), routeCount)
	}

	routes := make([]route.BoatRoute, 0, len(el.Routes))
	for i, routeEl := range el.Routes {
		nodes := make([]route.BoatLocation, 0, len(routeEl.Nodes))
		for _, node := range routeEl.Nodes {
			loc, err := route.NewBoatLocation(commons.StatSetFromXMLAttrs(node.Attrs))
			if err != nil {
				return route.BoatItinerary{}, err
			}
			nodes = append(nodes, loc)
		}
		routes = append(routes, route.BoatRoute{Dock: docks[i], ItemID: items[i], Nodes: nodes})
	}
	return route.BoatItinerary{Heading: el.Heading, Routes: routes}, nil
}

type walkerRouteFile struct {
	Routes []walkerRouteElement `xml:"route"`
}

type walkerRouteElement struct {
	Name string             `xml:"name,attr"`
	NPCs []walkerNPCElement `xml:"npc"`
}

type walkerNPCElement struct {
	Name  string         `xml:"name,attr"`
	Nodes []attrsElement `xml:"node"`
}

// LoadWalkerRoutes parses walking NPC route nodes.
func LoadWalkerRoutes(path string) (route.WalkerRoutes, error) {
	var doc walkerRouteFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("walker routes: %w", err)
	}

	routes := make(route.WalkerRoutes, len(doc.Routes))
	for _, routeEl := range doc.Routes {
		if _, exists := routes[routeEl.Name]; exists {
			return nil, fmt.Errorf("xml: %s: duplicate walker route %q", path, routeEl.Name)
		}
		byNPC := make(map[string][]route.WalkerLocation, len(routeEl.NPCs))
		for _, npcEl := range routeEl.NPCs {
			if _, exists := byNPC[npcEl.Name]; exists {
				return nil, fmt.Errorf("xml: %s: duplicate walker route %q npc %q", path, routeEl.Name, npcEl.Name)
			}
			nodes := make([]route.WalkerLocation, 0, len(npcEl.Nodes))
			for _, node := range npcEl.Nodes {
				loc, err := route.NewWalkerLocation(commons.StatSetFromXMLAttrs(node.Attrs))
				if err != nil {
					return nil, fmt.Errorf("xml: %s: route %q npc %q: %w", path, routeEl.Name, npcEl.Name, err)
				}
				nodes = append(nodes, loc)
			}
			byNPC[npcEl.Name] = nodes
		}
		routes[routeEl.Name] = byNPC
	}
	return routes, nil
}
