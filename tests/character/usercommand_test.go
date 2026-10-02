package character

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// User command ids the client sends for /loc, /unstuck, /mount, /dismount,
// /time and /partyinfo.
const (
	cmdLoc       = 0
	cmdEscape    = 52
	cmdMount     = 61
	cmdDismount  = 62
	cmdTime      = 77
	cmdPartyInfo = 81
)

func encodeUserCommand(id int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUserCommand)
	w.WriteInt32(id)
	return w.Bytes()
}

// userCommandFrames has c run user command id and returns its answer.
func userCommandFrames(t *testing.T, c *testsupport.ScriptedClient, id int32) [][]byte {
	t.Helper()
	c.Send(encodeUserCommand(id))
	return quietFrames(t, c)
}

// bootUserCommands boots one level 5 character in the world.
func bootUserCommands(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	objID := srv.SoleObjectID(t)
	enterWorld(t, srv.Client)
	drainQuiet(t, srv.Client)
	return srv, objID
}

// TestLocCommand pins /loc (Loc.useUserCommand): the message the player's
// restart point names (its locName, here 910, "Current location: $s1, $s2,
// $s3 (near Talking Island Village)") with the player's x, y and z as
// numbers. Where no restart point covers the player nothing is said.
func TestLocCommand(t *testing.T) {
	t.Parallel()
	t.Run("covered", func(t *testing.T) {
		t.Parallel()
		// The fixture spawn point lies in the map region of (0, 0).
		region := location.Point{
			X: (0-world.MinX)/world.TileSize + world.TileXMin,
			Y: (0-world.MinY)/world.TileSize + world.TileYMin,
		}
		table := &restart.Table{Points: []restart.Point{{Name: "TestTown", LocName: 910, MapRegions: []location.Point{region}}}}
		srv, objID := bootUserCommands(t, gameservertest.WithRestartPoints(table))
		frames := userCommandFrames(t, srv.Client, cmdLoc)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSystemMessage {
			t.Fatalf("/loc = %x, want one SystemMessage", opcodes(frames))
		}
		x, y, z := srv.PlayerPosition(t, objID)
		got := parseSystemMessage(t, frames[0])
		if got.id != 910 || !slices.Equal(got.number, []int32{int32(x), int32(y), int32(z)}) || len(got.texts) != 0 {
			t.Fatalf("/loc = %+v, want 910 with %d, %d, %d", got, x, y, z)
		}
	})
	t.Run("uncovered", func(t *testing.T) {
		t.Parallel()
		srv, _ := bootUserCommands(t, gameservertest.WithRestartPoints(&restart.Table{}))
		if frames := userCommandFrames(t, srv.Client, cmdLoc); len(frames) != 0 {
			t.Fatalf("/loc with no restart point = %x, want silence", opcodes(frames))
		}
	})
}

// TestTimeCommand pins /time (Time.useUserCommand): the in-game hour as a
// number and the minute as two-digit text, in the night message (928)
// before 6:00 and the day message (927) after.
func TestTimeCommand(t *testing.T) {
	t.Parallel()
	before := task.NewGameClock(time.Now)
	srv, _ := bootUserCommands(t)
	frames := userCommandFrames(t, srv.Client, cmdTime)
	after := task.NewGameClock(time.Now)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("/time = %x, want one SystemMessage", opcodes(frames))
	}
	got := parseSystemMessage(t, frames[0])
	matches := func(clock *task.GameClock) bool {
		id := int32(serverpackets.SystemMessageTimeS1S2InTheDay)
		if clock.Hour() < 6 {
			id = serverpackets.SystemMessageTimeS1S2InTheNight
		}
		return got.id == id && slices.Equal(got.number, []int32{int32(clock.Hour())}) &&
			slices.Equal(got.texts, []string{fmt.Sprintf("%02d", clock.Minute())})
	}
	if !matches(before) && !matches(after) {
		t.Fatalf("/time = %+v, want the game time %s or %s", got, before, after)
	}
}

var shippedEscape struct {
	once sync.Once
	defs []modelskill.Definition
	err  error
}

// escapeSkills is a skill table holding the shipped Escape skills, 2099
// (five minutes) and 2100 (one second), as aCis_datapack defines them.
func escapeSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	dir := datapack.Path(t, "data", "xml", "skills")
	shippedEscape.once.Do(func() {
		table, err := xmldata.LoadSkillDefinitions(dir, zerolog.Nop())
		if err != nil {
			shippedEscape.err = err
			return
		}
		for _, id := range []modelskill.ID{2099, 2100} {
			def, ok := table.Definition(modelskill.Ref{ID: id, Level: 1})
			if !ok {
				shippedEscape.err = fmt.Errorf("skill %d missing", id)
				return
			}
			shippedEscape.defs = append(shippedEscape.defs, def)
		}
	})
	if shippedEscape.err != nil {
		t.Fatalf("load shipped escape skills: %v", shippedEscape.err)
	}
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(shippedEscape.defs), gamesql.NewCharacterSkillStore(db))
}

// TestEscapeCommand pins /unstuck (Escape.useUserCommand) for a player:
// the PlaySound "systemmsg_e.809", the notice that it takes five minutes
// (809), then the cast of Escape: 5 minutes (2099) on itself, its 300000 ms
// hit time on the cast and the blue gauge. In a boss zone the player is
// told to petition instead (1043) and nothing is cast.
func TestEscapeCommand(t *testing.T) {
	t.Parallel()
	t.Run("player", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootUserCommands(t, gameservertest.WithSkills(escapeSkills(t)))
		frames := userCommandFrames(t, srv.Client, cmdEscape)
		if len(frames) < 4 {
			t.Fatalf("/unstuck = %x, want the sound, the notice and the cast", opcodes(frames))
		}
		if frames[0][0] != serverpackets.OpcodePlaySound {
			t.Fatalf("/unstuck first frame = %#x, want PlaySound", frames[0][0])
		}
		r := wire.NewReader(frames[0][1:])
		if typ, file := r.ReadInt32(), r.ReadString(); typ != 0 || file != "systemmsg_e.809" {
			t.Fatalf("PlaySound = %d %q, want 0 systemmsg_e.809", typ, file)
		}
		if got := parseSystemMessage(t, frames[1]); got.id != serverpackets.SystemMessageStuckTransportInFiveMinutes {
			t.Fatalf("/unstuck notice = %+v, want 809", got)
		}
		i := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeMagicSkillUse })
		if i < 0 {
			t.Fatalf("/unstuck = %x, want a MagicSkillUse", opcodes(frames))
		}
		r = wire.NewReader(frames[i][1:])
		caster, target, skill, level, hit := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if caster != objID || target != objID || skill != 2099 || level != 1 || hit != 300000 {
			t.Fatalf("MagicSkillUse = caster %d target %d skill %d/%d hit %d, want %d on itself, 2099/1, 300000", caster, target, skill, level, hit, objID)
		}
		if slices.IndexFunc(frames[i:], func(f []byte) bool { return f[0] == serverpackets.OpcodeSetupGauge }) < 0 {
			t.Fatalf("/unstuck = %x, want the cast gauge", opcodes(frames))
		}
	})
	t.Run("boss zone", func(t *testing.T) {
		t.Parallel()
		form, err := zone.NewCuboid(-1000000, 1000000, -1000000, 1000000, -100000, 100000)
		if err != nil {
			t.Fatalf("build zone form: %v", err)
		}
		set := commons.NewStatSet()
		set.Set("InvadeTime", "600000")
		boss, err := zone.NewBoss(1, form, set)
		if err != nil {
			t.Fatalf("build boss zone: %v", err)
		}
		zones := zone.NewIndex()
		zones.Add(boss)
		srv := gameservertest.Boot(t,
			gameservertest.WithCharacter("Newbie", 5, 0),
			gameservertest.WithWantChars(1),
			gameservertest.WithZones(zones),
			gameservertest.WithSkills(escapeSkills(t)),
		)
		boss.AllowEntry(srv.SoleObjectID(t), time.Minute)
		enterWorld(t, srv.Client)
		drainQuiet(t, srv.Client)
		frames := userCommandFrames(t, srv.Client, cmdEscape)
		if len(frames) != 1 || parseSystemMessage(t, frames[0]).id != serverpackets.SystemMessageNoUnstuckPleaseSendPetition {
			t.Fatalf("/unstuck in a boss zone = %x, want only 1043", opcodes(frames))
		}
	})
}

// TestMountCommandsWithoutMount pins /mount and /dismount (Mount, Dismount)
// with no mount and no pet to ride: both say nothing.
func TestMountCommandsWithoutMount(t *testing.T) {
	t.Parallel()
	srv, _ := bootUserCommands(t)
	for _, id := range []int32{cmdMount, cmdDismount, cmdPartyInfo} {
		if frames := userCommandFrames(t, srv.Client, id); len(frames) != 0 {
			t.Fatalf("user command %d = %x, want silence", id, opcodes(frames))
		}
	}
}
