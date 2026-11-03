package scenario

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// MakerSpawn is the spawn entry of an npcmaker line: one NPC of template NPC
// (a fixture template of that instance Type) placed at the world position X,
// Y, Z, back Respawn after it leaves, kept in the spawn_data row DB when that
// is set.
type MakerSpawn struct {
	NPC     int32
	Type    string
	X, Y, Z int
	Respawn time.Duration
	DB      string
}

// SpawnRow is one spawnrow line: the spawn_data row Name as the server
// left it, restored as the server boots. A row saved alive stands at X, Y,
// Z; a Dead row is waiting for its respawn, Respawn after the server boots
// (the spawns read the wall clock, so keep it well ahead).
type SpawnRow struct {
	Name    string
	Dead    bool
	X, Y, Z int
	Respawn time.Duration
}

// MakerEvent is one started line: a script event the npcmakers sent each
// other as the server started them.
type MakerEvent struct {
	Maker, Event string
	Int1, Int2   int32
}

// parseNpcMaker reads an npcmaker line:
//
//	npcmaker <name> <ai type> [max <n>] [spawn <npc id> <type> <x> <y> <z> [respawn <seconds>] [db <name>]]
func parseNpcMaker(args []string) (NpcMaker, error) {
	usage := fmt.Errorf("npcmaker <name> <ai type> [max <n>] [spawn <npc id> <type> <x> <y> <z> [respawn <seconds>] [db <name>]]")
	if len(args) < 2 {
		return NpcMaker{}, usage
	}
	m := NpcMaker{Name: args[0], AIType: args[1]}
	rest := args[2:]
	if len(rest) >= 2 && rest[0] == "max" {
		n, err := strconv.Atoi(rest[1])
		if err != nil || n < 1 {
			return NpcMaker{}, fmt.Errorf("npcmaker max %q is not a number of 1 or more", rest[1])
		}
		m.Max, rest = n, rest[2:]
	}
	if len(rest) == 0 {
		return m, nil
	}
	if rest[0] != "spawn" || len(rest) < 6 {
		return NpcMaker{}, usage
	}
	id, err := strconv.ParseInt(rest[1], 10, 32)
	if err != nil {
		return NpcMaker{}, fmt.Errorf("npcmaker spawn number %q: %w", rest[1], err)
	}
	pos, err := ints(rest[3:6])
	if err != nil {
		return NpcMaker{}, fmt.Errorf("npcmaker spawn: %w", err)
	}
	sp := &MakerSpawn{NPC: int32(id), Type: rest[2], X: pos[0], Y: pos[1], Z: pos[2]}
	for opts := rest[6:]; len(opts) > 0; opts = opts[2:] {
		if len(opts) < 2 {
			return NpcMaker{}, usage
		}
		switch opts[0] {
		case "respawn":
			d, err := seconds(opts[1])
			if err != nil {
				return NpcMaker{}, fmt.Errorf("npcmaker spawn respawn: %w", err)
			}
			sp.Respawn = d
		case "db":
			sp.DB = opts[1]
		default:
			return NpcMaker{}, fmt.Errorf("npcmaker spawn option %q is not respawn or db", opts[0])
		}
	}
	m.Spawn = sp
	return m, nil
}

// seconds reads a whole number of seconds of 0 or more.
func seconds(s string) (time.Duration, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%q is not a number of seconds of 0 or more", s)
	}
	return time.Duration(n) * time.Second, nil
}

// worldMakers reads the spawnrow and started lines.
func (sc *Scenario) worldMakers(key string, args []string) error {
	switch key {
	case "spawnrow":
		usage := fmt.Errorf("spawnrow <name> <alive|dead> <x> <y> <z> [respawn <seconds>]")
		if len(args) != 5 && len(args) != 7 {
			return usage
		}
		if args[1] != "alive" && args[1] != "dead" {
			return fmt.Errorf("spawnrow state %q is not alive or dead", args[1])
		}
		if slices.ContainsFunc(sc.SpawnRows, func(r SpawnRow) bool { return r.Name == args[0] }) {
			return fmt.Errorf("spawnrow %s listed twice", args[0])
		}
		pos, err := ints(args[2:5])
		if err != nil {
			return fmt.Errorf("spawnrow: %w", err)
		}
		row := SpawnRow{Name: args[0], Dead: args[1] == "dead", X: pos[0], Y: pos[1], Z: pos[2]}
		if len(args) == 7 {
			if args[5] != "respawn" {
				return usage
			}
			if row.Respawn, err = seconds(args[6]); err != nil {
				return fmt.Errorf("spawnrow respawn: %w", err)
			}
		}
		if row.Dead && row.Respawn == 0 {
			return fmt.Errorf("spawnrow %s is dead with no respawn ahead; add respawn <seconds>", row.Name)
		}
		if !row.Dead && row.Respawn != 0 {
			return fmt.Errorf("spawnrow %s is alive and takes no respawn", row.Name)
		}
		sc.SpawnRows = append(sc.SpawnRows, row)
	case "started":
		if len(args) != 4 {
			return fmt.Errorf("started <maker> <event> <int1> <int2>")
		}
		n, err := ints(args[2:])
		if err != nil {
			return fmt.Errorf("started: %w", err)
		}
		sc.Started = append(sc.Started, MakerEvent{Maker: args[0], Event: args[1], Int1: int32(n[0]), Int2: int32(n[1])})
	}
	return nil
}

// StartsMakers reports whether the scenario's makers are started as the
// server boots: an npcmaker line carries a spawn entry.
func (sc *Scenario) StartsMakers() bool {
	return slices.ContainsFunc(sc.NpcMakers, func(m NpcMaker) bool { return m.Spawn != nil })
}

// checkMakerSeeds requires the spawnrow lines to name a spawn entry's row,
// the started lines a maker line and a started maker.
func (sc *Scenario) checkMakerSeeds() error {
	byDB := map[string]string{}
	for _, m := range sc.NpcMakers {
		if m.Spawn == nil || m.Spawn.DB == "" {
			continue
		}
		if prev, dup := byDB[m.Spawn.DB]; dup {
			return fmt.Errorf("npcmakers %s and %s keep the row %s", prev, m.Name, m.Spawn.DB)
		}
		byDB[m.Spawn.DB] = m.Name
	}
	for _, r := range sc.SpawnRows {
		if _, ok := byDB[r.Name]; !ok {
			return fmt.Errorf("spawnrow %s: no npcmaker line's spawn keeps that row (its db option)", r.Name)
		}
	}
	for _, e := range sc.Started {
		if !sc.StartsMakers() {
			return fmt.Errorf("started %s: no npcmaker line has a spawn, so no maker is started", e.Maker)
		}
		if !slices.ContainsFunc(sc.NpcMakers, func(m NpcMaker) bool { return strings.EqualFold(m.Name, e.Maker) }) {
			return fmt.Errorf("started %s: no npcmaker line names it", e.Maker)
		}
	}
	for _, m := range sc.NpcMakers {
		if m.Max != 0 && m.Spawn == nil {
			return fmt.Errorf("npcmaker %s: max needs a spawn", m.Name)
		}
	}
	return nil
}
