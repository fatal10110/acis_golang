// Package shadowweapon is the class masters' shadow weapon exchange: a
// player holding a D-grade or C-grade exchange coupon is offered the
// multisell of the coupons it holds.
package shadowweapon

import (
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/firstclasschange"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/secondclasschange"
)

// The exchange coupons.
const (
	dGradeCoupon = 8869
	cGradeCoupon = 8870
)

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir:   "feature",
		Bind:  script.Bindings{script.EventTalked: slices.Concat(firstclasschange.Masters(), secondclasschange.Masters())},
		Hooks: script.Hooks{OnTalk: onTalk},
	}
}

// onTalk opens the exchange page, its multisell the one for both coupons,
// the C-grade one alone or the D-grade one alone, as the player holds them;
// a player with neither is told it has none.
func onTalk(s *script.Script, e script.Talk) string {
	hasD := e.Player.HasItem(dGradeCoupon)
	hasC := e.Player.HasItem(cGradeCoupon)
	if !hasD && !hasC {
		return "exchange-no.htm"
	}
	multisell := "306893003"
	if !hasD {
		multisell = "306893002"
	} else if !hasC {
		multisell = "306893001"
	}
	return strings.ReplaceAll(s.HTMLText("exchange.htm"), "%msid%", multisell)
}
