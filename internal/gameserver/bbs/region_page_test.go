package bbs

import (
	"testing"
	"time"
)

// Reference: RegionBBSManager.showRegion (RegionBBSManager.java:61-90)
// lists the clan halls of the castle's town under a header row, each with
// its owner's clan link and leader, or "None" twice for a free hall; with
// no hall the list is empty.
func TestRenderRegionCastleHalls(t *testing.T) {
	t.Parallel()
	owner := &RegionOwner{ID: 268435473, Name: "Lords", LeaderName: "Newbie"}
	c := RegionCastle{
		ID: 1, Name: "Gludio Castle", TaxPercent: 12,
		SiegeDate: time.Date(2026, 10, 11, 20, 0, 0, 0, time.Local).UnixMilli(),
	}
	page := "%castleName%|%tax%|%lord%|%clanName%|%allyName%|%siegeDate%|%tax%|%hallsList%"

	got := RenderRegionCastle(page, c, []RegionHall{{Name: "Moonstone Hall", Owner: owner}, {Name: "Onyx Hall"}})
	want := "Gludio Castle|12|None|None|None|2026-10-11 20:00|12|" +
		`<br><br><table width=610 bgcolor=A7A19A><tr><td width=5></td><td width=200>Clan Hall Name</td><td width=200>Owning Clan</td><td width=200>Clan Leader Name</td><td width=5></td></tr></table><br1>` +
		`<table><tr><td width=5></td><td width=200>Moonstone Hall</td><td width=200><a action="bypass _bbsclan;home;268435473">Lords</a></td><td width=200>Newbie</td><td width=5></td></tr></table><br1><img src="L2UI.Squaregray" width=605 height=1><br1>` +
		`<table><tr><td width=5></td><td width=200>Onyx Hall</td><td width=200>None</td><td width=200>None</td><td width=5></td></tr></table><br1><img src="L2UI.Squaregray" width=605 height=1><br1>`
	if got != want {
		t.Fatalf("castle page =\n%s\nwant\n%s", got, want)
	}

	if got := RenderRegionCastle("%hallsList%", c, nil); got != "" {
		t.Fatalf("hall list without halls = %q, want empty", got)
	}
}
