package quest

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// lowHPSkill is the one skill a low-HP order case casts.
const lowHPSkill = 9300

// lowHPAnswer is the client event the recording tutorial enables when it
// hears CE45: its answer to the low-HP event.
const lowHPAnswer = 45

func encodeLowHPSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// has reports whether name was heard since the last take.
func (l *tutorialLog) has(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Contains(l.events, name)
}

// isLowHPAnswer reports whether frame is the recording tutorial's CE45
// answer.
func isLowHPAnswer(frame []byte) bool {
	return frame[0] == serverpackets.OpcodeTutorialEnableClientEvent && wire.NewReader(frame[1:]).ReadInt32() == lowHPAnswer
}

// isOwnHPStatus reports whether frame is a StatusUpdate of objID carrying
// its current HP.
func isOwnHPStatus(frame []byte, objID int32) bool {
	if frame[0] != serverpackets.OpcodeStatusUpdate {
		return false
	}
	r := wire.NewReader(frame[1:])
	if r.ReadInt32() != objID {
		return false
	}
	for n := r.ReadInt32(); n > 0; n-- {
		attr, _ := r.ReadInt32(), r.ReadInt32()
		if attr == int32(serverpackets.StatusCurrentHP) {
			return true
		}
	}
	return false
}

// bootLowHP boots Newbie with the recording tutorial, started, knowing
// defs, in the world at a tenth of its maximum HP, that write reported and
// its CE45 heard and answered.
func bootLowHP(t *testing.T, defs ...modelskill.Definition) (*gameservertest.Server, int32, *tutorialLog) {
	t.Helper()
	db := sqltest.SharedDB(t)
	srv, objID, heard := bootTutorial(t, true,
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))))
	for _, def := range defs {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(def.ID), def.Level); err != nil {
			t.Fatal(err)
		}
	}
	enterWorld(t, srv)
	onCharacter(t, srv, objID, func(c *player.Character) {
		c.SetHP(c.MaxHPValue() * 0.1)
		c.BroadcastStatus()
	})
	srv.ReadQueued(t, srv.Client)
	if got := heard.take(); !slices.Equal(got, []string{"UC", "CE45"}) {
		t.Fatalf("entering and dropping to 10%% HP: heard %q, want [UC CE45]", got)
	}
	return srv, objID, heard
}

// selfSkill is a self-targeted active skill of type skillType, with a fixed
// half-second cast.
func selfSkill(skillType string) modelskill.Definition {
	return modelskill.Definition{
		ID: lowHPSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, StaticReuse: true, SkillType: skillType,
	}
}

// TestTutorialLowHPFollowsTheWritersStatus: an HP write whose status the
// writer reports itself raises CE45 after that StatusUpdate, so the
// tutorial's answer reaches the client right behind the player's own HP
// status: a heal, a heal percent, the heal and heal-over-time effects, a
// skill's HP cost, REAL_DAMAGE's survivable branch, BALANCE_LIFE, a drain's
// absorb and a melee hit's vampiric absorb. Each leaves the player below
// 30% of its maximum HP.
func TestTutorialLowHPFollowsTheWritersStatus(t *testing.T) {
	t.Parallel()
	heal := selfSkill("HEAL")
	heal.Power = 1
	healPercent := selfSkill("HEAL_PERCENT")
	healPercent.Power = 1
	healEffect := selfSkill("BUFF")
	healEffect.Effects = []modelskill.EffectTemplate{{Name: "Heal", Value: 1, Count: 1, Time: 1}}
	healOverTime := selfSkill("BUFF")
	healOverTime.Effects = []modelskill.EffectTemplate{{Name: "HealOverTime", Value: 1, Count: 2, Time: 1}}
	hpCost := selfSkill("DUMMY")
	hpCost.HPConsume = 1
	realDamage := selfSkill("REAL_DAMAGE")
	realDamage.Power = 1
	balanceLife := selfSkill("BALANCE_LIFE")
	drain := modelskill.Definition{
		ID: lowHPSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, HitTime: 500, StaticHitTime: true, StaticReuse: true, CastRange: 600,
		SkillType: "DRAIN", Power: 10, AbsorbAbs: 1,
	}
	cases := []struct {
		name string
		def  modelskill.Definition
		// hostile casts at, or attacks, a monster beside the player.
		hostile bool
		melee   bool
	}{
		{name: "heal", def: heal},
		{name: "heal percent", def: healPercent},
		{name: "heal effect", def: healEffect},
		{name: "heal over time", def: healOverTime},
		{name: "hp cost", def: hpCost},
		{name: "real damage", def: realDamage},
		{name: "balance life", def: balanceLife},
		{name: "drain", def: drain, hostile: true},
		{name: "vampiric absorb", hostile: true, melee: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var defs []modelskill.Definition
			if !tc.melee {
				defs = append(defs, tc.def)
			}
			srv, objID, heard := bootLowHP(t, defs...)
			c := srv.Client
			if tc.hostile {
				x, y, z := srv.PlayerPosition(t, objID)
				at := location.Location{X: x + 20, Y: y, Z: z}
				hostile := srv.SpawnHostileNPCAt(t, at)
				onCharacter(t, srv, objID, func(pc *player.Character) {
					if tc.melee {
						// The hit lands and is no critical.
						pc.SetRollSource(func(n int) int {
							if n == 1000 {
								return 299
							}
							return 0
						})
						pc.AddStatFuncs([]effect.Mod{{Stat: stat.AbsorbDamagePercent, Op: effect.OpAdd, Value: 1}})
						return
					}
					// The magic roll succeeds.
					pc.SetRollSource(func(n int) int {
						switch {
						case n == 10000:
							return 9999
						case n <= 0:
							return 0
						}
						return n - 1
					})
				})
				c.Send(encodeTutorialAction(hostile.ObjectID(), at))
				srv.ReadQueued(t, c)
				if tc.melee {
					c.Send(encodeTutorialAction(hostile.ObjectID(), at))
				}
			}
			if !tc.melee {
				c.Send(encodeLowHPSkillUse(lowHPSkill))
			}
			srv.AdvanceUntil(t, "CE45", func() bool {
				// The heal-over-time effect ticks on the effect sweep.
				srv.TickEffects()
				return heard.has("CE45")
			})
			frames := srv.ReadQueued(t, c)

			answers := 0
			for i, frame := range frames {
				if !isLowHPAnswer(frame) {
					continue
				}
				answers++
				if i == 0 || !isOwnHPStatus(frames[i-1], objID) {
					t.Fatalf("tutorial answer at frame %d of %x, want it right after the player's own HP StatusUpdate", i, frames)
				}
			}
			if answers == 0 {
				t.Fatalf("no tutorial answer among %x", frames)
			}
			var ratio float64
			onCharacter(t, srv, objID, func(pc *player.Character) { ratio = pc.HP() / pc.MaxHPValue() })
			if ratio >= 0.3 {
				t.Fatalf("HP ratio after the write = %v, want below 0.3", ratio)
			}
		})
	}
}

// TestTutorialLowHPAheadOfUnreportedStatus: an HP write that reports no
// status of its own raises CE45 at the write. A regeneration tick's
// answer comes ahead of the tick's StatusUpdate, and a full lethal strike
// raises CE45 at its HP write with nothing reported.
func TestTutorialLowHPAheadOfUnreportedStatus(t *testing.T) {
	t.Parallel()
	srv, objID, heard := bootLowHP(t)
	c := srv.Client

	// The regeneration sweep posts the tick to the player's queue.
	onCharacter(t, srv, objID, func(pc *player.Character) { pc.TickRegen() })
	frames := srv.ReadQueued(t, c)
	answer := slices.IndexFunc(frames, isLowHPAnswer)
	status := slices.IndexFunc(frames, func(f []byte) bool { return isOwnHPStatus(f, objID) })
	if answer < 0 || status < 0 || answer > status {
		t.Fatalf("regeneration frames %x: tutorial answer at %d, StatusUpdate at %d; want the answer first", frames, answer, status)
	}
	if got := heard.take(); !slices.Equal(got, []string{"CE45"}) {
		t.Fatalf("regeneration tick: heard %q, want [CE45]", got)
	}

	onCharacter(t, srv, objID, func(pc *player.Character) {
		pc.ApplyLethalOutcome(formulas.LethalFull, nil, modelskill.Definition{})
	})
	if got := heard.take(); !slices.Equal(got, []string{"CE45"}) {
		t.Fatalf("lethal strike: heard %q, want [CE45]", got)
	}
}

// TestTutorialLowHPAtRestore: a newbie saved below 30% of its maximum HP
// runs the tutorial's low-HP step when the character is restored at
// selection. The step's packets reach no client, but its journal writes
// stay, so no question mark 10 ever shows.
func TestTutorialLowHPAtRestore(t *testing.T) {
	t.Parallel()
	srv := bootNewbie(t, true, false)
	objID := createNewbie(t, srv)
	srv.FlushPersistence(t)
	if _, err := srv.DB.Exec("UPDATE characters SET curHp=1 WHERE obj_Id=?", objID); err != nil {
		t.Fatal(err)
	}
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	selection := srv.ReadQueued(t, c)
	if got := tutorialFrames(selection); len(got) != 0 {
		t.Fatalf("selection sent tutorial frames %x, want none", got)
	}
	if got := srv.QuestVars(t, objID, tutorial); got["HP"] != "1" || got["sit"] != "8388608" {
		t.Fatalf("tutorial after the selection = %v, want HP 1 and sit 8388608", got)
	}
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	burst := srv.ReadQueued(t, c)
	for _, f := range tutorialFrames(burst) {
		if f[0] == serverpackets.OpcodeTutorialShowQuestionMark && wire.NewReader(f[1:]).ReadInt32() == 10 {
			t.Fatalf("burst shows question mark 10: %x", burst)
		}
	}
	srv.FlushPersistence(t)
	if got := readJournal(t, srv.DB); !slices.Contains(got, journalRow{objID, tutorial, "HP", val("1")}) {
		t.Fatalf("journal rows %v, want the HP step's row", got)
	}
}
