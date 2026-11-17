package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	sinEaterNPCID      = 12564
	sinEaterAction     = int32(1001)
	sinEaterBusterID   = 4139
	sinEaterLastString = ", has not hit...!"
)

// bootSummonWithBuster boots an owner whose collar calls a pet of npcID
// that knows the Ultimate Bombastic Buster (4139, a self skill), and
// summons it.
func bootSummonWithBuster(t *testing.T, npcID int) (*petWorld, *summon.Actor) {
	t.Helper()
	tmpl := wolfTemplate()
	tmpl.ID, tmpl.TemplateID = npcID, npcID
	tmpl.Skills = map[int]int{sinEaterBusterID: 1}
	// A Sin Eater starts at its owner's level, which is 1 here.
	tmpl.Pet.Levels[1] = wolfLevelStats(wolfLevelExp)
	items, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: wolfCollarID, NPCID: int32(npcID), SummonType: 1}})
	if err != nil {
		t.Fatal(err)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{
			ID: sinEaterBusterID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
	}), gamesql.NewCharacterSkillStore(db))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl, treeTemplate()})),
		gameservertest.WithSummonItems(items),
		gameservertest.WithSkills(skills),
	})
	actor, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	return h, actor
}

// npcSays returns the text of every NpcSay the summon spoke among frames.
func npcSays(t *testing.T, frames [][]byte, speaker *summon.Actor) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNpcSay {
			continue
		}
		r := wire.NewReader(f[1:])
		id, typ, npcRef, text := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadString()
		if id != speaker.ObjectID() || typ != serverpackets.SayTypeAll || int(npcRef) != 1_000_000+speaker.NPCID() {
			t.Fatalf("NpcSay = (%d, %d, %d), want the summon %d, say-all, npc %d", id, typ, npcRef, speaker.ObjectID(), speaker.NPCID())
		}
		out = append(out, text)
	}
	return out
}

// TestSinEaterBusterSometimesSpeaks pins the Ultimate Bombastic Buster
// command (action 1001): a Sin Eater that starts the skill says one of its
// lines to everyone in sight on a roll under 10 of 100; a roll of 10 or more
// stays quiet, and so does any other summon with the same skill.
func TestSinEaterBusterSometimesSpeaks(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		npcID    int
		hundred  int
		wantLine bool
	}{
		{"sin eater on a low roll", sinEaterNPCID, 9, true},
		{"sin eater on the first quiet roll", sinEaterNPCID, 10, false},
		{"another summon on a low roll", wolfNPCID, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, actor := bootSummonWithBuster(t, tt.npcID)
			watcher := h.joinSecondPlayer(t, "Watcher")
			runOnPetQueue(t, actor, func() {
				// The percentage roll is the one of 100; the line picked is
				// the last of the four.
				actor.SetRollSource(func(n int) int {
					if n == 100 {
						return tt.hundred
					}
					return n - 1
				})
			})

			h.client.Send(encodeRequestActionUse(sinEaterAction, false))
			frames := advanceCollecting(t, h, time.Second)
			if _, cast := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse); !cast {
				t.Fatalf("the buster was not cast (frames %x)", frameOpcodes(frames))
			}
			seen := drainFrames(t, watcher.client)
			for who, got := range map[string][]string{"owner": npcSays(t, frames, actor), "watcher": npcSays(t, seen, actor)} {
				switch {
				case tt.wantLine && (len(got) != 1 || got[0] != sinEaterLastString):
					t.Fatalf("%s heard %q, want the one line %q", who, got, sinEaterLastString)
				case !tt.wantLine && len(got) != 0:
					t.Fatalf("%s heard %q, want nothing", who, got)
				}
			}
		})
	}
}
