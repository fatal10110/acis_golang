package npc

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// villageMasterRaces are the races each race-bound village master teaches.
// A master absent here teaches every race.
var villageMasterRaces = map[InstanceKind][]player.Race{
	"VillageMasterDElf":    {player.RaceDarkElf},
	"VillageMasterDwarf":   {player.RaceDwarf},
	"VillageMasterOrc":     {player.RaceOrc},
	"VillageMasterFighter": {player.RaceHuman, player.RaceElf},
	"VillageMasterMystic":  {player.RaceHuman, player.RaceElf},
	"VillageMasterPriest":  {player.RaceHuman, player.RaceElf},
}

// villageMasterTypes are the teaching lines the fighter, mystic and priest
// masters each keep to.
var villageMasterTypes = map[InstanceKind]player.ClassType{
	"VillageMasterFighter": player.ClassTypeFighter,
	"VillageMasterMystic":  player.ClassTypeMystic,
	"VillageMasterPriest":  player.ClassTypePriest,
}

// VillageMaster reports whether f is a village master of any kind.
func (f *Folk) VillageMaster() bool {
	return strings.HasPrefix(string(hostileKind(f.Instance)), "VillageMaster")
}

// Teaches reports whether this village master handles classID: the generic
// master every class, a dark elf, orc or dwarf master its own race's, and a
// fighter, mystic or priest master the human and elven classes of its line.
// An id naming no class is handled by none but the generic master.
func (f *Folk) Teaches(classID int) bool {
	kind := hostileKind(f.Instance)
	races, raceBound := villageMasterRaces[kind]
	if !raceBound {
		return true
	}
	race, ok := player.ClassRace(classID)
	if !ok || !slices.Contains(races, race) {
		return false
	}
	typ, typeBound := villageMasterTypes[kind]
	if !typeBound {
		return true
	}
	classType, _ := player.ClassTypeOf(classID)
	return classType == typ
}

// SubclassCommand is a parsed "Subclass <choice> [<one> [<two>]]" command.
type SubclassCommand struct {
	Choice   int
	One, Two int
}

// ParseSubclassCommand reads command's single-digit choice at its tenth
// character and its numbers from the twelfth: the first up to the next
// space, the second the rest. Characters are counted as the client encodes
// them, one per UTF-16 unit, and a number reads any Basic Multilingual Plane
// decimal digit, as the reference's Integer.parseInt over String.substring
// does. Reading stops at the first part that does not parse, leaving it and
// every later part 0.
func ParseSubclassCommand(command string) SubclassCommand {
	var cmd SubclassCommand
	units := utf16.Encode([]rune(command))
	if len(units) < 10 {
		return cmd
	}
	n, ok := parseTrimmedInt(units[9:10])
	if !ok {
		return cmd
	}
	cmd.Choice = n
	end := len(units)
	for i := 11; i < len(units); i++ {
		if units[i] == ' ' {
			end = i
			break
		}
	}
	if end < 11 {
		return cmd
	}
	if cmd.One, ok = parseTrimmedInt(units[11:end]); !ok {
		return cmd
	}
	if len(units) > end {
		cmd.Two, _ = parseTrimmedInt(units[end:])
	}
	return cmd
}

// parseTrimmedInt parses the characters in units, trimmed of every
// character up to the space, as a 32-bit decimal. A surrogate left alone by
// the range reads as U+FFFD, which no number reads.
func parseTrimmedInt(units []uint16) (int, bool) {
	n, err := commons.ParseInt(strings.TrimFunc(string(utf16.Decode(units)), javaSpace), 32)
	return int(n), err == nil
}

// AvailableSubclasses lists, by ascending class id, the classes this master
// offers a character of base class baseClassID holding subs: the subclasses
// its base class may take, that this master teaches, and that none of subs
// already is or upgrades from.
func (f *Folk) AvailableSubclasses(baseClassID int, subs []player.SubClass) []int {
	var out []int
	for _, id := range player.AvailableSubclasses(baseClassID) {
		if !f.Teaches(id) || takenSubclass(subs, id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// ValidNewSubclass reports whether a character of base class baseClassID
// holding subs may take classID as a subclass from this master.
func (f *Folk) ValidNewSubclass(baseClassID int, subs []player.SubClass, classID int) bool {
	if !f.Teaches(classID) || takenSubclass(subs, classID) {
		return false
	}
	return slices.Contains(player.AvailableSubclasses(baseClassID), classID)
}

// takenSubclass reports whether one of subs is classID or one of its
// upgrades.
func takenSubclass(subs []player.SubClass, classID int) bool {
	for _, sub := range subs {
		if player.ClassEqualsOrChildOf(sub.ClassID, classID) {
			return true
		}
	}
	return false
}

// SubclassPage reads data/html/villagemaster/<name>.htm, puts list in place
// of %list% and names this NPC in it.
func (f *Folk) SubclassPage(pages Pages, name, list string) string {
	return f.subclassPage(pages, name, func(page string) string {
		return strings.ReplaceAll(page, "%list%", list)
	})
}

// SubclassModifyPage is the replace-a-subclass menu for a character holding
// subs: each held slot shows its class name, and the link of a slot not
// held is taken out.
func (f *Folk) SubclassModifyPage(pages Pages, subs []player.SubClass) string {
	return f.subclassPage(pages, "SubClass_Modify", func(page string) string {
		for slot := 1; slot <= player.MaxSubclasses; slot++ {
			n := strconv.Itoa(slot)
			if i := slices.IndexFunc(subs, func(s player.SubClass) bool { return s.Index == slot }); i >= 0 {
				page = strings.ReplaceAll(page, "%sub"+n+"%", player.ClassName(subs[i].ClassID))
				continue
			}
			page = strings.ReplaceAll(page, `<a action="bypass -h npc_%objectId%_Subclass 6 `+n+`">%sub`+n+`%</a><br>`, "")
		}
		return page
	})
}

func (f *Folk) subclassPage(pages Pages, name string, fill func(string) string) string {
	path := "data/html/villagemaster/" + name + ".htm"
	page, ok := pages.Get(path)
	if !ok {
		page = "<html><body>My html is missing:<br>" + path + "</body></html>"
	}
	return strings.ReplaceAll(fill(page), "%objectId%", strconv.Itoa(int(f.ObjectID())))
}

// SubclassAddList is the add-a-subclass menu's list of classes.
func SubclassAddList(classes []int) string {
	var b strings.Builder
	for _, id := range classes {
		name := player.ClassName(id)
		b.WriteString(`<a action="bypass -h npc_%objectId%_Subclass 4 ` + strconv.Itoa(id) + `" msg="1268;` + name + `">` + name + `</a><br>`)
	}
	return b.String()
}

// SubclassChangeList is the change-class menu's list: the base class when
// this master teaches it, then each subclass it teaches, by slot.
func (f *Folk) SubclassChangeList(baseClassID int, subs []player.SubClass) string {
	var b strings.Builder
	if f.Teaches(baseClassID) {
		b.WriteString(`<a action="bypass -h npc_%objectId%_Subclass 5 0">` + player.ClassName(baseClassID) + `</a><br>`)
	}
	for _, sub := range subs {
		if f.Teaches(sub.ClassID) {
			b.WriteString(`<a action="bypass -h npc_%objectId%_Subclass 5 ` + strconv.Itoa(sub.Index) + `">` + player.ClassName(sub.ClassID) + `</a><br>`)
		}
	}
	return b.String()
}

// SubclassReplaceList is the replace-a-subclass menu's list of classes slot
// may take instead.
func SubclassReplaceList(slot int, classes []int) string {
	var b strings.Builder
	for _, id := range classes {
		b.WriteString(`<a action="bypass -h npc_%objectId%_Subclass 7 ` + strconv.Itoa(slot) + ` ` + strconv.Itoa(id) + `" msg="1445;">` + player.ClassName(id) + `</a><br>`)
	}
	return b.String()
}

// villageMasterClanCommands are the clan and alliance commands a village
// master runs itself, matched on the command's first word ignoring case.
var villageMasterClanCommands = []string{
	"create_clan", "increase_clan_level", "change_clan_leader", "cancel_clan_leader_change",
	"learn_clan_skills", "create_ally", "dissolve_ally", "dissolve_clan", "recover_clan",
	"create_academy", "create_royal", "create_knight", "rename_pledge", "assign_subpl_leader",
}

// VillageMasterClanCommand reports whether command is a clan command a
// village master runs.
func VillageMasterClanCommand(command string) bool {
	first := firstToken(command)
	for _, c := range villageMasterClanCommands {
		if strings.EqualFold(first, c) {
			return true
		}
	}
	return false
}
