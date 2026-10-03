package olympiad

import (
	"database/sql"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// shippedSkills are the datapack's skill definitions and trees, loaded once
// per test binary.
var shippedSkills = struct {
	once  sync.Once
	defs  *modelskill.Table
	trees *modelskill.Trees
	err   error
}{}

// skillOptions boots the shipped skill definitions and trees, so the skills
// a character is given at login reach its SkillList.
func skillOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	skillsDir := datapack.Path(t, "data", "xml", "skills")
	treesDir := datapack.Path(t, "data", "xml", "skillstrees")
	shippedSkills.once.Do(func() {
		shippedSkills.defs, shippedSkills.err = gamexml.LoadSkillDefinitions(skillsDir, zerolog.Nop())
		if shippedSkills.err == nil {
			shippedSkills.trees, shippedSkills.err = gamexml.LoadSkillTrees(treesDir)
		}
	})
	if shippedSkills.err != nil {
		t.Fatalf("load shipped skills: %v", shippedSkills.err)
	}
	db := sqltest.SharedDB(t)
	return []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), shippedSkills.defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithSkillTrees(shippedSkills.trees),
	}
}

// bootNamed boots the character name, its Olympiad state seeded by stmts,
// and returns the client with the character in the world and the frames of
// its login burst.
func bootNamed(t *testing.T, name string, stmts []string, opts ...gameservertest.Option) (*testsupport.ScriptedClient, [][]byte) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter(name, 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			for _, stmt := range stmts {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed %q: %v", stmt, err)
				}
			}
		}),
	}, opts...)...)
	burst := startInWorld(t, srv.Client)
	return srv.Client, burst
}

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

// startInWorld selects the first character, enters the world and returns
// the login burst.
func startInWorld(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	return drainFrames(t, c)
}

// drainFrames collects every frame c receives until the server goes quiet.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, 8)
	for range 400 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 400 drains")
	return nil
}

// firstFrame returns the first of frames with opcode.
func firstFrame(t *testing.T, frames [][]byte, opcode byte) []byte {
	t.Helper()
	for _, f := range frames {
		if len(f) > 0 && f[0] == opcode {
			return f
		}
	}
	t.Fatalf("no frame with opcode %#x among %d", opcode, len(frames))
	return nil
}

// sysMsgs returns each frame as a system message: its id then its
// parameters.
func sysMsgs(t *testing.T, frames [][]byte) [][]string {
	t.Helper()
	var out [][]string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			t.Fatalf("opcode = %#x, want SystemMessage", f[0])
		}
		r := wire.NewReader(f[1:])
		msg := []string{strconv.Itoa(int(r.ReadInt32()))}
		for range int(r.ReadInt32()) {
			if r.ReadInt32() == serverpackets.SystemMessageParamText {
				msg = append(msg, r.ReadString())
			} else {
				msg = append(msg, strconv.Itoa(int(r.ReadInt32())))
			}
		}
		out = append(out, msg)
	}
	return out
}

// skillListLevels returns each skill of a SkillList frame with its level.
func skillListLevels(t *testing.T, frame []byte) map[int32]int32 {
	t.Helper()
	r := wire.NewReader(frame[1:])
	out := map[int32]int32{}
	for range int(r.ReadInt32()) {
		r.ReadInt32() // passive
		level := r.ReadInt32()
		out[r.ReadInt32()] = level
		r.ReadUint8() // disabled
	}
	return out
}
