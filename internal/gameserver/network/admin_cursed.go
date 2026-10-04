package network

import (
	"math"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/cursedweapon"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
)

// cursedUsage is what //cw answers a command it cannot read.
const cursedUsage = "Usage: //cw [set|remove|teleportto itemid|name]"

// adminCursedWeapons answers //cw (AdminCursedWeapon.java):
//
//	//cw                         the cursed weapons panel
//	//cw set <id|name>           the weapon, not out yet, given to the selected player, gm without one
//	//cw remove <id|name>        the weapon ended wherever it is
//	//cw teleportto <id|name>    gm to the weapon's holder, or to where it lies
//
// A name is matched against the weapons' names, underscores read as
// spaces, ignoring case, the first weapon in the reference's order whose
// name contains it winning. A weapon no id or name finds is answered as
// unknown; a command without a weapon, or naming an id that is no int, is
// answered with the usage. Any other command word only reopens the panel,
// which every command but those ends on.
func (l *GameClientLink) adminCursedWeapons(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		l.sendCursedWeaponsPage(gm)
		return
	}
	if len(args) < 2 {
		sendText(gm, cursedUsage)
		return
	}
	itemID, ok := l.cursedWeaponNamed(args[1])
	if !ok {
		sendText(gm, cursedUsage)
		return
	}
	if !l.cursed.IsCursed(itemID) {
		sendText(gm, "Unknown cursed weapon ID.")
		return
	}
	switch args[0] {
	case "set":
		if l.adminSetCursedWeapon(gm, itemID) {
			return
		}
	case "remove":
		if l.adminRemoveCursedWeapon(gm, itemID) {
			return
		}
	case "teleportto":
		if !l.adminTeleportToCursedWeapon(gm, itemID) {
			sendText(gm, cursedUsage)
			return
		}
	}
	l.sendCursedWeaponsPage(gm)
}

// cursedWeaponNamed resolves a //cw weapon parameter: an id when it is all
// digits, else the first weapon whose name contains it. ok is false for
// digits that are no int; a name no weapon has resolves to 0, which no
// weapon is.
func (l *GameClientLink) cursedWeaponNamed(param string) (int32, bool) {
	if isDigits(param) {
		return parseJavaInt(param)
	}
	name := strings.ToLower(strings.ReplaceAll(param, "_", " "))
	for _, w := range l.cursed.Weapons() {
		if strings.Contains(strings.ToLower(w.Name), name) {
			return w.ItemID, true
		}
	}
	return 0, true
}

// adminSetCursedWeapon gives itemID's weapon to the selected player, gm
// without one, on that player's queue: it gets the weapon as it would
// picking it up, becoming its holder, and the weapon's full life starts
// then. A weapon already out is refused. It reports whether the panel is
// sent later, from the player's queue, instead of at once.
//
// The weapon is reserved while it is handed out, so no drop brings it out
// meanwhile; one that is not given after all goes back. A player already
// holding a weapon takes the new one in, which ends it: its life does not
// start.
func (l *GameClientLink) adminSetCursedWeapon(gm *livePlayer, itemID int32) bool {
	if !l.cursed.Reserve(itemID) {
		sendText(gm, "This cursed weapon is already active.")
		return false
	}
	target := adminTargetPlayer(gm, true)
	give := func() {
		if !target.AddCreatedItem(itemID, 1, l.nextObjectID) || !l.cursed.StartLife(itemID, target.ObjectID()) {
			l.cursed.Unreserve(itemID)
		}
	}
	if target == gm {
		give()
		return false
	}
	if !postLive(target, func() {
		give()
		postLive(gm, func() { l.sendCursedWeaponsPage(gm) })
	}) {
		l.cursed.Unreserve(itemID)
		return false
	}
	return true
}

// adminRemoveCursedWeapon ends itemID's weapon wherever it is, out or not:
// a holder gets its karma and PK kills back and loses the weapon, in the
// world or out of it, a weapon on the ground leaves the world, and every
// player is told it disappeared. It reports whether the panel is sent
// later, once a holder on another queue was told, instead of at once.
func (l *GameClientLink) adminRemoveCursedWeapon(gm *livePlayer, itemID int32) bool {
	end, ok := l.cursed.End(itemID)
	if !ok {
		return false
	}
	if holder, online := l.livePlayerByID(end.HolderID); end.Held && online && holder != gm {
		l.endCursedWeaponThen(end, gm, func() {
			postLive(gm, func() { l.sendCursedWeaponsPage(gm) })
		})
		return true
	}
	l.endCursedWeapon(end, gm)
	return false
}

// adminTeleportToCursedWeapon moves gm to itemID's holder, or to where the
// weapon lies; a weapon not out is answered as not in the world. It
// reports false for a held weapon whose holder is out of the world, which
// the reference cannot reach either: CursedWeapon.teleportTo fails on the
// holder it no longer has.
func (l *GameClientLink) adminTeleportToCursedWeapon(gm *livePlayer, itemID int32) bool {
	for _, w := range l.cursed.Weapons() {
		if w.ItemID != itemID {
			continue
		}
		switch {
		case w.Activated:
			holder, online := l.livePlayerByID(w.HolderID)
			if !online {
				return false
			}
			l.teleportLivePlayer(gm, holder.CurrentLocation(), 0)
		case w.Dropped:
			if !w.HasGroundAt {
				return false
			}
			l.teleportLivePlayer(gm, w.GroundAt, 0)
		default:
			sendText(gm, w.Name+" isn't in the world.")
		}
	}
	return true
}

// Buttons of the cursed weapons panel.
const (
	cursedButtonOpen  = "<button value=\""
	cursedButtonMid   = "\" action=\"bypass -h admin_cw "
	cursedButtonClose = "\" width=75 height=21 back=\"L2UI_ch3.Btn1_normalOn\" fore=\"L2UI_ch3.Btn1_normal\">"
)

// sendCursedWeaponsPage sends gm the cursed weapons panel, cwinfo.htm, with
// every weapon in the reference's order: a held one with its holder (named
// while in the world, "null" otherwise), the karma and PK kills the holder
// gets back, its stage, its time and hunger left and its kills toward the
// next stage; one on the ground with its time left; one not out with the
// button that hands it out.
func (l *GameClientLink) sendCursedWeaponsPage(gm *livePlayer) {
	var sb strings.Builder
	for _, w := range l.cursed.Weapons() {
		id := strconv.Itoa(int(w.ItemID))
		sb.WriteString("<table width=280><tr><td>Name:</td><td>" + w.Name + "</td></tr>")
		switch {
		case !w.Out:
			sb.WriteString("<tr><td>Position:</td><td>Doesn't exist.</td></tr><tr><td>" + cursedButton("Set CW", "set "+id) + "</td><td></td></tr>")
		case w.Activated:
			owner := "null"
			if holder, online := l.livePlayerByID(w.HolderID); online {
				owner = holder.Name
			}
			sb.WriteString("<tr><td>Owner:</td><td>" + owner + "</td></tr>" +
				"<tr><td>Stored values:</td><td>Karma=" + itoa32(w.Karma) + " PKs=" + itoa32(w.PKKills) + "</td></tr>" +
				"<tr><td>Current stage:</td><td>" + itoa32(w.Stage) + "</td></tr>" +
				"<tr><td>Overall time:</td><td>" + cursedOverallTime(w) + "</td></tr>" +
				"<tr><td>Hungry time:</td><td>" + itoa32(w.HungryMinutes) + "m.</td></tr>" +
				"<tr><td>Current kills:</td><td>" + itoa32(w.Kills) + " / " + itoa32(w.NextStageKills) + "</td></tr>" +
				"<tr><td>" + cursedButton("Remove CW", "remove "+id) + "</td><td>" + cursedButton("Teleport To", "teleportto "+id) + "</td></tr>")
		case w.Dropped:
			sb.WriteString("<tr><td>Position:</td><td>Lying on the ground</td></tr>" +
				"<tr><td>Overall time:</td><td>" + cursedOverallTime(w) + "</td></tr>" +
				"<tr><td>" + cursedButton("Remove", "remove "+id) + "</td><td>" + cursedButton("Go", "teleportto "+id) + "</td></tr>")
		}
		sb.WriteString("</table>")
	}
	page := strings.ReplaceAll(l.adminHTML("cwinfo.htm"), "%cwinfo%", sb.String())
	sendFilledHTML(gm, 0, page, 0)
}

// cursedButton is a panel button labelled value running //cw command.
func cursedButton(value, command string) string {
	return cursedButtonOpen + value + cursedButtonMid + command + cursedButtonClose
}

// cursedOverallTime is a weapon's time left as the panel shows it, in
// days, hours and minutes, worked out as the reference does: the seconds
// past the last whole minute are taken off, then the minutes, hours and
// days are each the floor of what is left, in floating point.
func cursedOverallTime(w cursedweapon.Info) string {
	ms := w.TimeLeft.Milliseconds()
	secs := float64((ms / 1000) % 60)
	countDown := (float64(ms)/1000 - secs) / 60
	mins := int(math.Floor(math.Mod(countDown, 60)))
	countDown = (countDown - float64(mins)) / 60
	hours := int(math.Floor(math.Mod(countDown, 24)))
	days := int(math.Floor((countDown - float64(hours)) / 24))
	return strconv.Itoa(days) + "d. " + strconv.Itoa(hours) + "h. " + strconv.Itoa(mins) + "m."
}

// reloadCursedWeapons answers //reload cw (CursedWeaponManager.reload):
// every weapon ends, out or not, each told to every player as it would be
// at the end of its life, and the weapons are read anew from
// cursedWeapons.xml, none of them out. A holder has the end applied on its
// own queue, the game master included. With cursed weapons disabled there
// is nothing to reload. A file that cannot be read leaves every weapon as
// it was.
func (l *GameClientLink) reloadCursedWeapons() error {
	if l.cursed == nil {
		return nil
	}
	if l.reloads.CursedWeapons == nil {
		return errReloadNotPorted
	}
	fresh, err := l.reloads.CursedWeapons()
	if err != nil {
		return err
	}
	for _, end := range l.cursed.Reload(fresh) {
		l.endCursedWeapon(end, nil)
	}
	if l.cursedWeapons != nil {
		l.cursedWeapons.Replace(fresh)
	}
	return nil
}
