package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// castleCertificates is the certificate count //castle certificates puts
// a castle back to.
const castleCertificates = 300

// adminCastle answers //castle [set|remove|certificates|tax] <alias>:
//
//   - //castle <alias> opens the castle's page;
//   - set gives the castle to the selected player's clan, unless that clan
//     already owns one;
//   - remove takes the castle from its owner;
//   - certificates puts its left certificates back to 300 and stores them;
//   - tax closes its tax period (castle.Castle.UpdateTaxes).
//
// Each then opens the castle's page; an unknown action is answered with
// the usage first. Without exactly one or two arguments, or with an alias
// no castle has, the castle list would open instead.
func (l *GameClientLink) adminCastle(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	var param string
	var c *castle.Castle
	switch len(args) {
	case 1:
		c, _ = l.castles.ByAlias(args[0])
	case 2:
		param = args[0]
		c, _ = l.castles.ByAlias(args[1])
	}
	if c == nil {
		// ponytail: the castle list shows each castle's siege status, which
		// needs the siege engine (#234).
		l.log.Warn().Str("command", "admin_castle").Msg("admin: castle list not implemented yet")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	switch param {
	case "":
	case "set":
		l.adminCastleSet(gm, c)
	case "remove":
		if c.OwnerID() > 0 {
			l.removeCastleOwner(gm, c)
		} else {
			sendText(gm, "This castle does not have an owner.")
		}
	case "certificates":
		c.SetLeftCertificates(castleCertificates, true)
		sendText(gm, c.Name+"'s castle certificates are reset.")
	case "tax":
		c.UpdateTaxes()
		sendText(gm, c.Name+"'s taxes have been updated.")
	default:
		sendText(gm, "Usage: //castle [set|remove|certificates|tax castleName].")
	}
	l.showAdminCastle(gm, c)
}

// adminCastleSet gives c to the clan of gm's selected player. A selection
// that is no player in a clan is refused as an incorrect target.
func (l *GameClientLink) adminCastleSet(gm *livePlayer, c *castle.Castle) {
	target := adminTargetPlayer(gm, false)
	if target == nil {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
		return
	}
	cl, ok := l.clanService().ClanOf(target.Character)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
		return
	}
	if !l.setCastleOwner(gm, c, cl) {
		sendText(gm, target.Name+"'s clan already owns a castle.")
	}
}

// showAdminCastle opens c's page for gm.
//
// The page counts the life control towers standing, which outside a siege
// is every one the castle has. During a siege only those still alive
// count; tracking them needs the siege engine (#234). No mercenary ticket
// can be dropped yet (#238), so the dropped ticket count is 0.
func (l *GameClientLink) showAdminCastle(gm *livePlayer, c *castle.Castle) {
	const file = "data/html/admin/castle.htm"
	page, ok := l.html.Get(file)
	if !ok {
		page = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	lifeTowers := 0
	for _, t := range c.ControlTowers {
		if t.Type == castledata.TowerLifeControl {
			lifeTowers++
		}
	}
	var artifacts, towers strings.Builder
	for i, a := range c.Artifacts {
		adminTeleportLink(&artifacts, i+1, a.Position.X, a.Position.Y, a.Position.Z, a.Heading)
	}
	for i, t := range c.ControlTowers {
		adminTeleportLink(&towers, i+1, t.Position.X, t.Position.Y, t.Position.Z, 0)
	}
	page = strings.NewReplacer(
		"%castleName%", c.Name,
		"%castleAlias%", c.Alias,
		"%circletId%", strconv.Itoa(c.CircletID),
		"%ticketsNumber%", strconv.Itoa(len(c.Tickets)),
		"%droppedTicketsNumber%", "0",
		"%npcsNumber%", strconv.Itoa(len(c.NPCs)),
		"%certificates%", strconv.Itoa(c.LeftCertificates()),
		"%parent%", strconv.Itoa(c.ParentID),
		"%aliveLifeTowers%", strconv.Itoa(lifeTowers),
		"%defaultTax%", strconv.Itoa(c.Tax.Rate),
		"%currentTax%", strconv.Itoa(c.CurrentTaxPercent()),
		"%nextTax%", strconv.Itoa(c.NextTaxPercent()),
		"%taxSysgetRate%", strconv.Itoa(c.Tax.SysgetRate),
		"%taxRevenue%", groupThousands(c.TaxRevenue()),
		"%tributeRate%", groupThousands(int64(c.Tax.TributeRate)),
		"%seedIncome%", groupThousands(c.SeedIncome()),
		"%treasury%", groupThousands(c.Treasury()),
		"%artifacts%", artifacts.String(),
		"%ct%", towers.String(),
	).Replace(page)
	sendValidatedHTML(gm, 0, page, 0)
}

// adminTeleportLink writes the n-th teleport link to x y z heading.
func adminTeleportLink(b *strings.Builder, n, x, y, z, heading int) {
	b.WriteString(`<a action="bypass -h admin_teleport `)
	b.WriteString(strconv.Itoa(x) + " " + strconv.Itoa(y) + " " + strconv.Itoa(z) + " " + strconv.Itoa(heading))
	b.WriteString(`">[` + strconv.Itoa(n) + `]</a>&nbsp;&nbsp;`)
}

// groupThousands writes n with a comma between each group of three
// digits.
func groupThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return sign + b.String()
}
