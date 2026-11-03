package network

import (
	"slices"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// doorInfoPage is d's door page: its names and ids, residence, opening
// rules, combat stats and where it stands.
func (l *GameClientLink) doorInfoPage(d *door.Object) string {
	t := d.Template
	x, y, z := d.Position()
	initial := "Closed"
	if t.Opened {
		initial = "Opened"
	}
	return fillPage(l.adminHTML("doorinfo.htm"),
		"%name%", t.Name,
		"%objid%", strconv.Itoa(int(d.ObjectID())),
		"%doorid%", strconv.Itoa(t.ID),
		"%doortype%", t.Kind.String(),
		"%doorlvl%", strconv.Itoa(t.Level),
		"%residence%", l.doorResidenceName(t.Name),
		"%opentype%", t.OpenKind.String(),
		"%initial%", initial,
		"%ot%", strconv.Itoa(t.OpenTime),
		"%ct%", strconv.Itoa(t.CloseTime),
		"%rt%", strconv.Itoa(t.RandomTime),
		"%controlid%", strconv.Itoa(t.TriggeredID),
		"%hp%", strconv.Itoa(d.HP()),
		"%hpmax%", strconv.Itoa(d.MaxHP()),
		"%hpratio%", strconv.Itoa(d.HPUpgradeRatio()),
		"%pdef%", strconv.Itoa(d.PhysicalDefence()),
		"%mdef%", strconv.Itoa(d.MagicDefence()),
		"%spawn%", strconv.Itoa(x)+", "+strconv.Itoa(y)+", "+strconv.Itoa(z)+", "+strconv.Itoa(d.Heading()),
		"%height%", commons.FormatDouble(float64(d.Height())),
	)
}

// sealStrifeIndex is the Seal of Strife's place in a Record's seals.
var sealStrifeIndex = slices.Index(sevensigns.Seals[:], sevensigns.Strife)

// doorResidenceName names the residence whose gates list the door called
// name, "none" when no residence does. Clan halls are attached after
// castles, so a hall listing the door wins.
func (l *GameClientLink) doorResidenceName(name string) string {
	if h, ok := l.hallOfDoor(name); ok {
		return h.Name
	}
	for _, c := range l.castleData.All() {
		if slices.Contains(c.Gates, name) {
			return c.Name
		}
	}
	return "none"
}
