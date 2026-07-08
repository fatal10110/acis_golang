package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

func TestLoadBoatRoutes(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "boatRoutes.xml"))

	itineraries, err := LoadBoatRoutes(path)
	if err != nil {
		t.Fatalf("LoadBoatRoutes(%q) error: %v", path, err)
	}

	if got, want := len(itineraries), 5; got != want {
		t.Fatalf("len(itineraries) = %d, want %d", got, want)
	}
	if got, want := countBoatRoutes(itineraries), 9; got != want {
		t.Fatalf("boat route count = %d, want %d", got, want)
	}
	if got, want := countBoatNodes(itineraries), 170; got != want {
		t.Fatalf("boat node count = %d, want %d", got, want)
	}

	first := itineraries[0]
	if first.Heading != 60800 || len(first.Routes) != 2 {
		t.Fatalf("first itinerary = %+v", first)
	}
	if first.Routes[0].Dock != route.DockGiran || first.Routes[0].ItemID != 3946 || len(first.Routes[0].Nodes) != 20 {
		t.Fatalf("first route = %+v", first.Routes[0])
	}
	if got := first.Routes[0].Nodes[0].DepartureMessages; len(got) != 1 || got[0] != 1162 {
		t.Fatalf("first node departure = %v, want [1162]", got)
	}
	last := first.Routes[0].Nodes[len(first.Routes[0].Nodes)-1]
	if last.BusyMessage != 1487 || len(last.ArrivalMessages) != 2 || len(last.Scheduled) != 4 {
		t.Fatalf("last first-route node = %+v", last)
	}
	if itineraries[2].Routes[0].Dock != route.DockInnadril || len(itineraries[2].Routes) != 1 {
		t.Fatalf("one-way itinerary = %+v", itineraries[2])
	}
}

func TestLoadWalkerRoutes(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "walkerRoutes.xml"))

	routes, err := LoadWalkerRoutes(path)
	if err != nil {
		t.Fatalf("LoadWalkerRoutes(%q) error: %v", path, err)
	}

	if got, want := len(routes), 132; got != want {
		t.Fatalf("len(routes) = %d, want %d", got, want)
	}
	if got, want := routes.NPCCount(), 480; got != want {
		t.Fatalf("NPCCount() = %d, want %d", got, want)
	}
	if got, want := routes.NodeCount(), 3624; got != want {
		t.Fatalf("NodeCount() = %d, want %d", got, want)
	}

	gordon := routes["gordon"]["gordon"]
	if len(gordon) == 0 {
		t.Fatal("gordon route not loaded")
	}
	if gordon[5].DelayMillis != 4000 || gordon[5].SocialID != 1 {
		t.Fatalf("gordon node 5 = %+v", gordon[5])
	}
	remy := routes["porter_remy"]["porter_remy"]
	if remy[3].NPCStringID != 1010202 {
		t.Fatalf("porter_remy node 3 NPCStringID = %d, want 1010202", remy[3].NPCStringID)
	}
}

func TestLoadBoatRoutesErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boatRoutes.xml")
	writeXMLFixture(t, path, `<list><itinerary dock1="NOPE" heading="1"><route><node x="1" y="2" z="3"/></route></itinerary></list>`)

	if _, err := LoadBoatRoutes(path); err == nil {
		t.Fatal("LoadBoatRoutes() error = nil, want error")
	}
}

func countBoatRoutes(itineraries []route.BoatItinerary) int {
	var n int
	for _, itinerary := range itineraries {
		n += len(itinerary.Routes)
	}
	return n
}

func countBoatNodes(itineraries []route.BoatItinerary) int {
	var n int
	for _, itinerary := range itineraries {
		for _, r := range itinerary.Routes {
			n += len(r.Nodes)
		}
	}
	return n
}
