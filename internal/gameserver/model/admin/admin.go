package admin

import (
	"fmt"
	"strings"

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
	level, err := set.GetInt("level")
	if err != nil {
		return AccessLevel{}, fmt.Errorf("admin: access level: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("admin: access level %d: %w", level, err) }

	name, err := set.GetString("name")
	if err != nil {
		return AccessLevel{}, wrap(err)
	}
	nameColor, err := set.GetString("nameColor")
	if err != nil {
		return AccessLevel{}, wrap(err)
	}
	titleColor, err := set.GetString("titleColor")
	if err != nil {
		return AccessLevel{}, wrap(err)
	}
	childLevel, err := set.GetIntDefault("childLevel", 0)
	if err != nil {
		return AccessLevel{}, wrap(err)
	}

	return AccessLevel{
		Level:            level,
		Name:             name,
		NameColor:        nameColor,
		TitleColor:       titleColor,
		ChildLevel:       childLevel,
		IsGM:             set.GetBoolDefault("isGM", false),
		AllowFixedRes:    set.GetBoolDefault("allowFixedRes", false),
		AllowTransaction: set.GetBoolDefault("allowTransaction", false),
		AllowAltG:        set.GetBoolDefault("allowAltg", false),
		GiveDamage:       set.GetBoolDefault("giveDamage", false),
	}, nil
}

type Command struct {
	Name        string
	AccessLevel int
	Params      string
	Description string
}

func NewCommand(set *commons.StatSet) (Command, error) {
	name, err := set.GetString("name")
	if err != nil {
		return Command{}, fmt.Errorf("admin: command: %w", err)
	}
	level, err := set.GetInt("accessLevel")
	if err != nil {
		return Command{}, fmt.Errorf("admin: command %q: %w", name, err)
	}
	params := set.GetStringDefault("params", "")
	desc := set.GetStringDefault("desc", "")
	return Command{Name: name, AccessLevel: level, Params: params, Description: desc}, nil
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
	message, err := set.GetString("message")
	if err != nil {
		return Announcement{}, fmt.Errorf("admin: announcement: %w", err)
	}
	if strings.TrimSpace(message) == "" {
		return Announcement{}, fmt.Errorf("admin: announcement: empty message")
	}

	a := Announcement{
		Message:  message,
		Critical: set.GetBoolDefault("critical", false),
		Auto:     set.GetBoolDefault("auto", false),
	}
	if !a.Auto {
		return a, nil
	}

	if a.InitialDelay, err = set.GetInt("initial_delay"); err != nil {
		return Announcement{}, fmt.Errorf("admin: announcement %q: %w", message, err)
	}
	if a.Delay, err = set.GetInt("delay"); err != nil {
		return Announcement{}, fmt.Errorf("admin: announcement %q: %w", message, err)
	}
	if a.Limit, err = set.GetIntDefault("limit", 0); err != nil {
		return Announcement{}, fmt.Errorf("admin: announcement %q: %w", message, err)
	}
	if a.Limit < 0 {
		a.Limit = 0
	}
	return a, nil
}

type Data struct {
	accessLevels map[int]AccessLevel
	commands     map[string]Command
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

	return &Data{accessLevels: accessLevels, commands: commandMap}, nil
}

func (d *Data) AccessLevel(level int) (AccessLevel, bool) {
	value, ok := d.accessLevels[level]
	return value, ok
}

func (d *Data) Command(name string) (Command, bool) {
	value, ok := d.commands[strings.ToLower(name)]
	return value, ok
}

func (d *Data) AccessLevelCount() int { return len(d.accessLevels) }
func (d *Data) CommandCount() int     { return len(d.commands) }
