package network

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// npcInfoRefresh is the Refresh button the aggro and desire pages carry,
// reopening sub.
func npcInfoRefresh(sub string) string {
	return `<button value="Refresh" action="bypass -h admin_info ` + sub + `" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2">`
}

// adminNpcInfo answers //info [sub-command [args]] on the NPC obj, the
// instance inst: no argument or a page number opens a general page, and
// each sub-command its own page. A sub-command whose page reads state Go
// does not keep yet logs the gap and releases the client.
func (l *GameClientLink) adminNpcInfo(gm *livePlayer, obj world.Tracked, inst *npc.Instance, args []string) {
	if len(args) == 0 {
		sendFilledHTML(gm, 0, l.npcGeneralPage(obj, inst, 0), 0)
		return
	}
	switch sub := args[0]; sub {
	case "ai":
		page := 0
		if len(args) > 1 {
			if n, err := commons.ParseInt(args[1], 32); err == nil {
				page = int(n)
			}
		}
		switch page {
		case 1, 2:
			l.npcInfoGap(gm, "ai "+strconv.Itoa(page), 3396)
		default:
			l.npcInfoGap(gm, "ai "+strconv.Itoa(page), 3398)
		}
	case "aggro":
		sendFilledHTML(gm, 0, l.npcDefaultPage(npcAggroContent(obj)), 0)
	case "desire":
		h, ok := obj.(*npc.Hostile)
		if !ok {
			l.npcInfoGap(gm, sub, 3397)
			return
		}
		sendFilledHTML(gm, 0, l.npcDefaultPage(npcDesireContent(h)), 0)
	case "drop", "spoil":
		l.adminNpcDrops(gm, inst, sub == "drop", args[1:])
	case "script":
		l.npcInfoGap(gm, sub, 3398)
	case "shop", "skill":
		l.npcInfoGap(gm, sub, 3396)
	case "spawn":
		l.npcInfoGap(gm, sub, 3397)
	case "stat":
		status, ok := obj.(npcStatus)
		if !ok {
			l.npcInfoGap(gm, sub, 3397)
			return
		}
		sendFilledHTML(gm, 0, l.npcStatPage(status, inst.Template), 0)
	default:
		page := 0
		if isDigits(sub) {
			n, ok := parseJavaInt(sub)
			if !ok {
				// Integer.valueOf throws past the int range: the command
				// ends with nothing sent.
				l.log.Warn().Str("page", sub).Msg("admin: //info general page out of the int range")
				return
			}
			page = int(n)
		}
		sendFilledHTML(gm, 0, l.npcGeneralPage(obj, inst, page), 0)
	}
}

// npcInfoGap logs that the //info NPC page sub is not ported yet, tracked
// by issue, and releases gm's client.
func (l *GameClientLink) npcInfoGap(gm *livePlayer, sub string, issue int) {
	l.log.Warn().Str("page", sub).Int("issue", issue).Msg("admin: //info NPC page not implemented yet")
	gm.SendFrame(serverpackets.FrameActionFailed())
}

// npcDefaultPage is npcinfo/default.htm showing content.
func (l *GameClientLink) npcDefaultPage(content string) string {
	return strings.ReplaceAll(l.adminHTML("npcinfo/default.htm"), "%content%", content)
}

// npcGeneralPage is general page index of NPC obj: page 1 its rewards,
// attack range, race and aggro clans, page 2 its behavior flags and
// residence, and any other index page 0, who it is and how it looks.
func (l *GameClientLink) npcGeneralPage(obj world.Tracked, inst *npc.Instance, index int) string {
	t := inst.Template
	switch index {
	case 1:
		clans, ignored := "none", "none"
		if t.Clans != nil {
			clans = "[" + strings.Join(t.Clans, ", ") + "]"
		}
		if t.IgnoredIDs != nil {
			ignored = javaIntArray(t.IgnoredIDs)
		}
		return fillPage(l.adminHTML("npcinfo/general-1.htm"),
			"%exp%", commons.JavaDouble(t.RewardExp),
			"%sp%", commons.JavaDouble(t.RewardSp),
			"%baseAttackRange%", strconv.Itoa(t.BaseAttackRange),
			"%baseDamageRange%", javaIntArray(t.BaseDamageRange),
			"%baseRandomDamage%", strconv.Itoa(t.BaseRandomDamage),
			"%race%", t.Race.EnumName(),
			"%clan%", clans,
			"%clanRange%", strconv.Itoa(t.ClanRange),
			"%ignoredIds%", ignored,
		)
	case 2:
		return fillPage(l.adminHTML("npcinfo/general-2.htm"),
			"%isUndying%", strconv.FormatBool(t.Undying),
			"%canBeAttacked%", strconv.FormatBool(t.CanBeAttacked),
			"%isNoSleepMode%", strconv.FormatBool(t.NoSleepMode),
			"%aggroRange%", strconv.Itoa(t.AggroRange),
			"%canMove%", strconv.FormatBool(t.CanMove),
			"%isSeedable%", strconv.FormatBool(t.Seedable),
			"%residence%", l.npcResidenceName(t.ID),
		)
	default:
		return fillPage(l.adminHTML("npcinfo/general-0.htm"),
			"%objectId%", strconv.Itoa(int(obj.ObjectID())),
			"%npcId%", strconv.Itoa(t.ID),
			"%idTemplate%", strconv.Itoa(t.TemplateID),
			"%name%", t.Name,
			"%title%", t.Title,
			"%alias%", t.Alias,
			"%usingServerSideName%", strconv.FormatBool(t.UsingServerSideName),
			"%usingServerSideTitle%", strconv.FormatBool(t.UsingServerSideTitle),
			"%type%", npcClassName(inst),
			"%level%", strconv.Itoa(t.Level),
			"%radius%", commons.JavaDouble(t.CollisionRadius),
			"%height%", commons.JavaDouble(t.CollisionHeight),
			"%hitTimeFactor%", commons.JavaDouble(t.HitTimeFactor),
			"%rHand%", strconv.Itoa(t.RightHand),
			"%lHand%", strconv.Itoa(t.LeftHand),
		)
	}
}

// npcClassName names the reference class an NPC of inst is an instance of:
// the one its template type names.
func npcClassName(inst *npc.Instance) string {
	if inst.Kind != "" {
		return string(inst.Kind)
	}
	return inst.Template.Type
}

// npcResidenceName names the residence an NPC of template id serves: the
// first castle listing it, else the first clan hall, else "none".
func (l *GameClientLink) npcResidenceName(id int) string {
	for _, c := range l.castleData.All() {
		if slices.Contains(c.NPCs, id) {
			return c.Name
		}
	}
	for _, h := range l.clanHallData.All() {
		if slices.Contains(h.NPCs, id) {
			return h.Name
		}
	}
	return "none"
}

// javaIntArray formats v the way Arrays.toString(int[]) does: "[1, 2]",
// "null" for no array.
func javaIntArray(v []int) string {
	if v == nil {
		return "null"
	}
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// npcStatus is the live status an NPC stat page reads.
type npcStatus interface {
	HP() float64
	MaxHPValue() float64
	MPValue() float64
	MaxMPValue() float64
	PAtk() float64
	MAtk() float64
	PDef() float64
	MDef() float64
	Evasion() int
	MoveSpeed() float64
	AttackSpeed() int
	MagicAttackSpeed() int
	STR() int
	DEX() int
	CON() int
	INT() int
	WIT() int
	MEN() int
	CalcStat(s stat.Stat, base float64) float64
}

// npcStatPage is the stat page of an NPC of template t: its vitals,
// combat stats, base stats and element defences.
func (l *GameClientLink) npcStatPage(s npcStatus, t *npc.Template) string {
	itoa := strconv.Itoa
	element := func(res stat.Stat) string { return commons.JavaDouble(s.CalcStat(res, 1)) }
	return fillPage(l.adminHTML("npcinfo/stat.htm"),
		"%hp%", itoa(int(s.HP())),
		"%hpmax%", itoa(int(s.MaxHPValue())),
		"%mp%", itoa(int(s.MPValue())),
		"%mpmax%", itoa(int(s.MaxMPValue())),
		"%patk%", itoa(int(s.PAtk())),
		"%matk%", itoa(int(s.MAtk())),
		"%pdef%", itoa(int(s.PDef())),
		"%mdef%", itoa(int(s.MDef())),
		"%accu%", itoa(int(s.CalcStat(stat.AccuracyCombat, 0))),
		"%evas%", itoa(s.Evasion()),
		"%crit%", itoa(min(int(s.CalcStat(stat.CriticalRate, t.CritRate)), 500)),
		"%rspd%", itoa(int(s.MoveSpeed())),
		"%aspd%", itoa(s.AttackSpeed()),
		"%cspd%", itoa(s.MagicAttackSpeed()),
		"%str%", itoa(s.STR()),
		"%dex%", itoa(s.DEX()),
		"%con%", itoa(s.CON()),
		"%int%", itoa(s.INT()),
		"%wit%", itoa(s.WIT()),
		"%men%", itoa(s.MEN()),
		"%ele_fire%", element(stat.FireRes),
		"%ele_water%", element(stat.WaterRes),
		"%ele_wind%", element(stat.WindRes),
		"%ele_earth%", element(stat.EarthRes),
		"%ele_holy%", element(stat.HolyRes),
		"%ele_dark%", element(stat.DarkRes),
	)
}

// npcAggroContent lists the fifteen most hated attackers of NPC obj with
// the damage each dealt, most hated first. An NPC that is not attackable
// builds no aggro.
func npcAggroContent(obj world.Tracked) string {
	h, ok := obj.(*npc.Hostile)
	if !ok {
		return "This NPC can't build aggro towards targets.<br>" + npcInfoRefresh("aggro")
	}
	threats := h.AI().Threats().Snapshot()
	if len(threats) == 0 {
		return "This NPC's AggroList is empty.<br>" + npcInfoRefresh("aggro")
	}
	slices.SortStableFunc(threats, func(a, b attackable.Threat) int {
		if c := cmp.Compare(b.Hate, a.Hate); c != 0 {
			return c
		}
		return cmp.Compare(a.Attacker.ObjectID(), b.Attacker.ObjectID())
	})
	var b strings.Builder
	b.WriteString(npcInfoRefresh("aggro"))
	b.WriteString(`<br><table width="280"><tr><td><font color="LEVEL">Attacker</font></td><td><font color="LEVEL">Damage</font></td><td><font color="LEVEL">Hate</font></td></tr>`)
	for _, th := range threats[:min(len(threats), 15)] {
		b.WriteString("<tr><td>" + attackerName(th.Attacker) + "</td><td>" + commons.JavaDouble(th.Damage) + "</td><td>" + commons.JavaDouble(th.Hate) + "</td></tr>")
	}
	b.WriteString(`</table><img src="L2UI.SquareGray" width=280 height=1>`)
	return b.String()
}

// attackerName is the name an admin page prints for c: an unnamed summon
// has none, which the reference prints as "null".
func attackerName(c attackable.Combatant) string {
	if s, ok := c.(*summon.Actor); ok && !s.IsNamed() {
		return "null"
	}
	return c.CharacterName()
}

// npcDesireContent lists every desire h's AI holds with its weight.
func npcDesireContent(h *npc.Hostile) string {
	desires := h.AI().Desires().Snapshot()
	if len(desires) == 0 {
		return "This NPC's Desires are empty.<br>" + npcInfoRefresh("desire")
	}
	var b strings.Builder
	b.WriteString(npcInfoRefresh("desire"))
	b.WriteString(`<br><table width="280"><tr><td><font color="LEVEL">Type</font></td><td><font color="LEVEL">Weight</font></td></tr>`)
	for _, d := range desires {
		b.WriteString("<tr><td>" + d.Kind.EnumName() + "</td><td>" + commons.JavaDouble(d.Weight) + "</td></tr>")
	}
	b.WriteString(`</table><img src="L2UI.SquareGray" width=280 height=1>`)
	return b.String()
}
