package admin

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/commons"
)

type AccessLevel struct {
	Level            int
	Name             string
	NameColor        string
	TitleColor       string
	ChildLevel       int
	IsGM             bool
	AllowFixedRes    bool
	AllowTransaction bool
	AllowAltG        bool
	GiveDamage       bool
}

func NewAccessLevel(set *commons.StatSet) (AccessLevel, error) {
	idf := commons.NewFields(set, "admin: access level")
	level := idf.Int("level")
	if err := idf.Err(); err != nil {
		return AccessLevel{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("admin: access level %d", level))
	accessLevel := AccessLevel{
		Level:            level,
		Name:             f.String("name"),
		NameColor:        f.StringDefault("nameColor", "FFFFFF"),
		TitleColor:       f.StringDefault("titleColor", "FFFF77"),
		ChildLevel:       f.IntDefault("childLevel", 0),
		IsGM:             f.BoolDefault("isGM", false),
		AllowFixedRes:    f.BoolDefault("allowFixedRes", false),
		AllowTransaction: f.BoolDefault("allowTransaction", true),
		AllowAltG:        f.BoolDefault("allowAltg", false),
		GiveDamage:       f.BoolDefault("giveDamage", true),
	}
	if err := f.Err(); err != nil {
		return AccessLevel{}, err
	}
	// A color is the hex digits of an int32 literal with its "0x" left off.
	for _, c := range [...]struct{ key, value string }{{"nameColor", accessLevel.NameColor}, {"titleColor", accessLevel.TitleColor}} {
		if _, err := commons.DecodeInt32("0x" + c.value); err != nil {
			return AccessLevel{}, fmt.Errorf("admin: access level %d: %s: %w", level, c.key, err)
		}
	}
	return accessLevel, nil
}

type Command struct {
	Name        string
	AccessLevel int
	Params      string
	Description string
}

func NewCommand(set *commons.StatSet) (Command, error) {
	idf := commons.NewFields(set, "admin: command")
	name := idf.String("name")
	if err := idf.Err(); err != nil {
		return Command{}, err
	}
	f := commons.NewFields(set, fmt.Sprintf("admin: command %q", name))
	command := Command{
		Name:        name,
		AccessLevel: f.Int("accessLevel"),
		Params:      f.StringDefault("params", ""),
		Description: f.StringDefault("desc", ""),
	}
	if err := f.Err(); err != nil {
		return Command{}, err
	}
	return command, nil
}

type Announcement struct {
	Message      string
	Critical     bool
	Auto         bool
	InitialDelay int
	Delay        int
	Limit        int
}

func NewAnnouncement(set *commons.StatSet) (Announcement, error) {
	f := commons.NewFields(set, "admin: announcement")
	message := f.String("message")
	critical := f.BoolDefault("critical", false)
	auto := f.BoolDefault("auto", false)
	if err := f.Err(); err != nil {
		return Announcement{}, err
	}
	a := Announcement{
		Message:  message,
		Critical: critical,
		Auto:     auto,
	}
	if !a.Auto {
		return a, nil
	}

	af := commons.NewFields(set, fmt.Sprintf("admin: announcement %q", message))
	a.InitialDelay = af.Int("initial_delay")
	a.Delay = af.Int("delay")
	a.Limit = af.Int("limit")
	if err := af.Err(); err != nil {
		return Announcement{}, err
	}
	if a.Limit < 0 {
		a.Limit = 0
	}
	return a, nil
}

// Data is the access-level and admin-command tables. //reload admin
// replaces them in place with Replace: every method call reads one
// snapshot, the old tables or the new ones, never a mix.
type Data struct {
	cur atomic.Pointer[tables]
}

// tables is one loaded snapshot of the access-level and command tables.
type tables struct {
	accessLevels map[int]AccessLevel
	commands     map[string]Command
	// ordered holds the commands in the order the table lists them.
	ordered []Command
}

func NewData(levels []AccessLevel, commands []Command) (*Data, error) {
	accessLevels := make(map[int]AccessLevel, len(levels))
	for _, level := range levels {
		if _, exists := accessLevels[level.Level]; exists {
			return nil, fmt.Errorf("admin: duplicate access level %d", level.Level)
		}
		accessLevels[level.Level] = level
	}

	commandMap := make(map[string]Command, len(commands))
	for _, command := range commands {
		key := strings.ToLower(command.Name)
		if _, exists := commandMap[key]; exists {
			return nil, fmt.Errorf("admin: duplicate command %q", command.Name)
		}
		commandMap[key] = command
	}

	d := &Data{}
	d.cur.Store(&tables{accessLevels: accessLevels, commands: commandMap, ordered: slices.Clone(commands)})
	return d, nil
}

// Replace swaps d's tables for from's, at once for every holder of d:
// AdminData.reload. A character keeps the access level it plays under
// until that level is resolved again.
func (d *Data) Replace(from *Data) {
	d.cur.Store(from.cur.Load())
}

// load returns d's current snapshot; a nil table has an empty one.
func (d *Data) load() *tables {
	if d == nil {
		return &tables{}
	}
	return d.cur.Load()
}

func (d *Data) AccessLevel(level int) (AccessLevel, bool) {
	value, ok := d.load().accessLevels[level]
	return value, ok
}

// defaultAccessLevel is the user level the attribute defaults describe, for a
// server running without an access-level table.
var defaultAccessLevel = AccessLevel{AllowTransaction: true, GiveDamage: true}

// Resolve returns the access level a character with the persisted level plays
// under: every negative level reads the -1 entry, and a level the table does
// not define falls back to the user level 0. A nil table, or one without a
// level 0, resolves to the attribute defaults.
func (d *Data) Resolve(level int) AccessLevel {
	if d == nil {
		return defaultAccessLevel
	}
	if level < 0 {
		level = -1
	}
	t := d.load()
	if value, ok := t.accessLevels[level]; ok {
		return value
	}
	if value, ok := t.accessLevels[0]; ok {
		return value
	}
	return defaultAccessLevel
}

func (d *Data) Command(name string) (Command, bool) {
	return d.load().command(name)
}

func (t *tables) command(name string) (Command, bool) {
	value, ok := t.commands[strings.ToLower(name)]
	return value, ok
}

// Commands returns the command table in the order it lists the commands.
// The slice is shared; callers must not modify it.
func (d *Data) Commands() []Command {
	return d.load().ordered
}

func (d *Data) AccessLevelCount() int { return len(d.load().accessLevels) }
func (d *Data) CommandCount() int     { return len(d.load().commands) }
