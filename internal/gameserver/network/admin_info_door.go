package network

import (
	"slices"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// doorUpgradeHPRatio is the HP upgrade ratio every door shows and multiplies
// its maximum HP by: castle door upgrades, which raise it, are not ported
// yet (#236).
const doorUpgradeHPRatio = 1

// doorInfoPage is d's door page: its names and ids, residence, opening
// rules, combat stats and where it stands.
func (l *GameClientLink) doorInfoPage(d *door.Object) string {
	t := d.Template
	x, y, z := d.Position()
	initial := "Closed"
	if t.Opened {
		initial = "Opened"
	}
	pDef, mDef := l.doorDefences(t)
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
		"%hpmax%", strconv.Itoa(d.MaxHP()*doorUpgradeHPRatio),
		"%hpratio%", strconv.Itoa(doorUpgradeHPRatio),
		"%pdef%", strconv.Itoa(pDef),
		"%mdef%", strconv.Itoa(mDef),
		"%spawn%", strconv.Itoa(x)+", "+strconv.Itoa(y)+", "+strconv.Itoa(z)+", "+strconv.Itoa(d.Heading()),
		"%height%", commons.JavaDouble(float64(d.Height())),
	)
}

// doorDefences returns t's P.Def and M.Def under the Seal of Strife: the
// template values raised by a fifth while Dawn owns the seal, cut to 30%
// while Dusk does, truncated.
func (l *GameClientLink) doorDefences(t *door.Template) (pDef, mDef int) {
	factor := 1.0
	if l.sevenSigns != nil {
		switch l.sevenSigns.Record(0).Seals[sealStrifeIndex].Owner {
		case sevensigns.Dawn:
			factor = 1.2
		case sevensigns.Dusk:
			factor = 0.3
		}
	}
	return int(float64(t.PDef) * factor), int(float64(t.MDef) * factor)
}

// sealStrifeIndex is the Seal of Strife's place in a Record's seals.
var sealStrifeIndex = slices.Index(sevensigns.Seals[:], sevensigns.Strife)

// doorResidenceName names the residence whose gates list the door called
// name, "none" when no residence does. Clan halls are attached after
// castles, so a hall listing the door wins.
func (l *GameClientLink) doorResidenceName(name string) string {
	for _, h := range l.clanHallData.All() {
		if slices.Contains(h.Gates, name) {
			return h.Name
		}
	}
	for _, c := range l.castleData.All() {
		if slices.Contains(c.Gates, name) {
			return c.Name
		}
	}
	return "none"
}
