package xml

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
)

// The residence loaders fold a castle's attributes with every <tax> child,
// and a clan hall's with every <agit> child and then every <tax> child, into
// one attribute set: a later value replaces an earlier one of the same name,
// a name a later child omits keeps its earlier value, and only the final
// value is parsed.

func loadCastleFixture(t *testing.T, content string) (*residenceCastleResult, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "castles.xml")
	writeXMLFixture(t, path, `<list>`+content+`</list>`)
	table, err := LoadCastles(path)
	if err != nil {
		return nil, err
	}
	c, ok := table.Get(1)
	if !ok {
		t.Fatal("Get(1) returned no castle")
	}
	res := &residenceCastleResult{tax: c.Tax, name: c.Name}
	for _, tw := range c.ControlTowers {
		res.towerZones = append(res.towerZones, tw.Zones)
	}
	return res, nil
}

type residenceCastleResult struct {
	tax        residence.Tax
	name       string
	towerZones [][]string
}

func TestLoadCastlesFoldsEveryTaxChild(t *testing.T) {
	t.Parallel()
	got, err := loadCastleFixture(t, `<castle id="1" alias="c" parentId="0" name="C" circletId="1">
		<tax taxRate="10" taxSysgetRate="20" tributeRate="30"/>
		<tax taxRate="15"/>
	</castle>`)
	if err != nil {
		t.Fatalf("LoadCastles error: %v", err)
	}
	if want := (residence.Tax{Rate: 15, SysgetRate: 20, TributeRate: 30}); got.tax != want {
		t.Fatalf("Tax = %+v, want %+v (later taxRate wins, omitted rates kept)", got.tax, want)
	}
}

func TestLoadCastlesTaxFoldSharesTheCastleAttributeSet(t *testing.T) {
	t.Parallel()
	// The castle element may carry a rate itself, and a <tax> child may
	// replace a castle attribute; an earlier malformed value replaced by a
	// later one is never parsed.
	got, err := loadCastleFixture(t, `<castle id="1" alias="c" parentId="0" name="C" circletId="1" taxSysgetRate="5">
		<tax taxRate="bad" tributeRate="30"/>
		<tax taxRate="7" name="Renamed"/>
	</castle>`)
	if err != nil {
		t.Fatalf("LoadCastles error: %v", err)
	}
	if want := (residence.Tax{Rate: 7, SysgetRate: 5, TributeRate: 30}); got.tax != want {
		t.Fatalf("Tax = %+v, want %+v", got.tax, want)
	}
	if got.name != "Renamed" {
		t.Fatalf("Name = %q, want the <tax> child's %q", got.name, "Renamed")
	}

	if _, err := loadCastleFixture(t, `<castle id="1" alias="c" parentId="0" name="C" circletId="1">
		<tax taxRate="10" taxSysgetRate="20" tributeRate="30"/>
		<tax tributeRate="bad"/>
	</castle>`); err == nil {
		t.Fatal("LoadCastles accepted a malformed final tributeRate")
	}
}

func TestLoadCastlesControlTowerLastZonesWins(t *testing.T) {
	t.Parallel()
	got, err := loadCastleFixture(t, `<castle id="1" alias="c" parentId="0" name="C" circletId="1">
		<controlTowers>
			<controlTower alias="t1" type="LIFE_CONTROL">
				<position x="1" y="2" z="3"/>
				<stats hp="1" pDef="1" mDef="1"/>
				<zones val="a;b"/>
				<zones val="c"/>
			</controlTower>
			<controlTower alias="t2" type="LIFE_CONTROL">
				<position x="1" y="2" z="3"/>
				<stats hp="1" pDef="1" mDef="1"/>
				<zones val="d;e"/>
			</controlTower>
		</controlTowers>
		<tax taxRate="0" taxSysgetRate="0" tributeRate="0"/>
	</castle>`)
	if err != nil {
		t.Fatalf("LoadCastles error: %v", err)
	}
	if want := [][]string{{"c"}, {"d", "e"}}; !reflect.DeepEqual(got.towerZones, want) {
		t.Fatalf("tower zones = %v, want %v (last <zones> child wins)", got.towerZones, want)
	}
}

func loadHallFixture(t *testing.T, content string) (hallResult, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clanHalls.xml")
	writeXMLFixture(t, path, `<list>`+content+`</list>`)
	table, err := LoadClanHalls(path)
	if err != nil {
		return hallResult{}, err
	}
	h, ok := table.Get(1)
	if !ok {
		t.Fatal("Get(1) returned no hall")
	}
	return hallResult{
		Desc: h.Description, Town: h.Town,
		AuctionMin: h.AuctionMin, Deposit: h.Deposit, Lease: h.Lease, Size: h.Size, Grade: h.Grade,
		SiegeLength: h.SiegeLength, Siegable: h.Siegable, ScheduleConfig: h.ScheduleConfig,
		Tax: h.Tax,
	}, nil
}

type hallResult struct {
	Desc, Town                              string
	AuctionMin, Deposit, Lease, Size, Grade int
	SiegeLength                             int64
	Siegable                                bool
	ScheduleConfig                          []int
	Tax                                     residence.Tax
}

func TestLoadClanHallsFoldsEveryAgitAndTaxChild(t *testing.T) {
	t.Parallel()
	got, err := loadHallFixture(t, `<clanHall id="1" alias="h" parentId="0" name="H">
		<agit desc="First" loc="Gludio" auctionMin="100" deposit="200" lease="300" size="3" grade="1"/>
		<agit desc="Second" auctionMin="150" siegeLength="3600000" scheduleConfig="14;0;0;12;0"/>
		<tax taxRate="10" taxSysgetRate="20" tributeRate="30"/>
		<tax tributeRate="50"/>
	</clanHall>`)
	if err != nil {
		t.Fatalf("LoadClanHalls error: %v", err)
	}
	want := hallResult{
		Desc: "Second", Town: "Gludio",
		AuctionMin: 150, Deposit: 200, Lease: 300, Size: 3, Grade: 1,
		SiegeLength: 3600000, Siegable: true, ScheduleConfig: []int{14, 0, 0, 12, 0},
		Tax: residence.Tax{Rate: 10, SysgetRate: 20, TributeRate: 50},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hall = %+v\nwant   %+v", got, want)
	}
}

func TestLoadClanHallsFoldOrder(t *testing.T) {
	t.Parallel()
	// Every <agit> folds before every <tax>, whatever their document
	// interleaving, so a <tax> key beats an <agit> key written after it.
	got, err := loadHallFixture(t, `<clanHall id="1" alias="h" parentId="0" name="H" desc="Parent" loc="Dion">
		<tax taxRate="0" taxSysgetRate="0" tributeRate="0" lease="9"/>
		<agit lease="4" grade="2"/>
	</clanHall>`)
	if err != nil {
		t.Fatalf("LoadClanHalls error: %v", err)
	}
	if got.Desc != "Parent" || got.Town != "Dion" {
		t.Fatalf("desc/loc = %q/%q, want the clan hall's own Parent/Dion", got.Desc, got.Town)
	}
	if got.Lease != 9 || got.Grade != 2 {
		t.Fatalf("lease/grade = %d/%d, want 9 (tax after agit) and 2", got.Lease, got.Grade)
	}
	if got.Siegable {
		t.Fatal("hall without siegeLength must not be siegable")
	}
}

func TestLoadClanHallsWithoutAgitChild(t *testing.T) {
	t.Parallel()
	got, err := loadHallFixture(t, `<clanHall id="1" alias="h" parentId="0" name="H" desc="Hall" loc="Aden">
		<tax taxRate="0" taxSysgetRate="0" tributeRate="0"/>
	</clanHall>`)
	if err != nil {
		t.Fatalf("LoadClanHalls rejected desc/loc on the clan hall itself: %v", err)
	}
	if got.Desc != "Hall" || got.Town != "Aden" || got.AuctionMin != 0 {
		t.Fatalf("hall = %+v", got)
	}

	if _, err := loadHallFixture(t, `<clanHall id="1" alias="h" parentId="0" name="H" desc="Hall">
		<tax taxRate="0" taxSysgetRate="0" tributeRate="0"/>
	</clanHall>`); err == nil {
		t.Fatal("LoadClanHalls accepted a hall with no loc anywhere")
	}
}

func TestLoadClanHallsFoldedValueChecks(t *testing.T) {
	t.Parallel()
	const tax = `<tax taxRate="0" taxSysgetRate="0" tributeRate="0"/>`
	cases := []struct {
		name string
		hall string
		ok   bool
	}{
		{"malformed auctionMin", `<agit desc="d" loc="l" auctionMin="x"/>`, false},
		{"malformed deposit replaced later", `<agit desc="d" loc="l" deposit="x"/><agit deposit="5"/>`, true},
		{"siegable without scheduleConfig", `<agit desc="d" loc="l" siegeLength="1"/>`, false},
		{"siegable with empty scheduleConfig", `<agit desc="d" loc="l" siegeLength="1" scheduleConfig=""/>`, false},
		{"unread scheduleConfig on a plain hall", `<agit desc="d" loc="l" scheduleConfig="bad"/>`, true},
		{"malformed final siegeLength", `<agit desc="d" loc="l" siegeLength="1" scheduleConfig="1"/><agit siegeLength="x"/>`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadHallFixture(t, `<clanHall id="1" alias="h" parentId="0" name="H">`+c.hall+tax+`</clanHall>`)
			if (err == nil) != c.ok {
				t.Fatalf("LoadClanHalls error = %v, want ok=%v", err, c.ok)
			}
		})
	}
}
