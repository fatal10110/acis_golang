package skills

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Seeded social-scene characters and clans: far above anything the id
// factory hands out during a test.
const (
	socialMateID       int32 = 0x7f300001
	socialStrangerID   int32 = 0x7f300002
	socialKnightsID    int32 = 0x7f300101
	socialRivalsID     int32 = 0x7f300102
	socialSkillID      int32 = 1230
	socialPartySkillID int32 = 1255
)

// socialScene is the boot character, Caster, beside two seeded players,
// Mate and Stranger, all level 20 at the shared spawn.
type socialScene struct {
	srv                          *gameservertest.Server
	caster, mate, stranger       *testsupport.ScriptedClient
	casterID, mateID, strangerID int32
}

// bootSocialScene boots the scene with Caster knowing def, plus opts.
func bootSocialScene(t *testing.T, def modelskill.Definition, opts ...gameservertest.Option) *socialScene {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Caster", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
			tmpl, ok := gameservertest.Templates(t).Get(0)
			if !ok {
				t.Fatal("missing test class template")
			}
			for _, m := range []struct {
				id            int32
				account, name string
			}{{socialMateID, "player2", "Mate"}, {socialStrangerID, "player3", "Stranger"}} {
				ch, err := player.NewCharacter(m.id, tmpl, m.account, m.name, 1, 0, 0, player.SexMale)
				if err != nil {
					t.Fatalf("seed %s: %v", m.name, err)
				}
				ch.CharLevel = 20
				if err := chars.Create(context.Background(), ch); err != nil {
					t.Fatalf("seed %s: %v", m.name, err)
				}
			}
		}),
	}, opts...)...)
	s := &socialScene{srv: srv, caster: srv.Client, casterID: srv.SoleObjectID(t), mateID: socialMateID, strangerID: socialStrangerID}
	seedKnownSkill(t, srv, s.casterID, int(def.ID), int(def.Level))
	enterSocialScene(t, s.caster)
	s.mate = srv.DialClient(t, "player2", 1)
	enterSocialScene(t, s.mate)
	s.stranger = srv.DialClient(t, "player3", 1)
	enterSocialScene(t, s.stranger)
	s.quiet(t)
	return s
}

// enterSocialScene brings c's character into the world, whatever its clan
// adds to the entry burst.
func enterSocialScene(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	for frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelected; frame = c.Read() {
	}
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
}

func (s *socialScene) quiet(t *testing.T) {
	t.Helper()
	for _, c := range []*testsupport.ScriptedClient{s.caster, s.mate, s.stranger} {
		drainUntilQuiet(t, c)
	}
}

// formParty has Caster invite Mate into a party and Mate accept.
func (s *socialScene) formParty(t *testing.T) {
	t.Helper()
	s.caster.Send(encodePartyInvite("Mate"))
	drainUntilQuiet(t, s.caster)
	s.mate.Send(encodePartyAnswer(1))
	s.quiet(t)
}

// flag PvP-flags the player id.
func (s *socialScene) flag(t *testing.T, id int32) {
	t.Helper()
	obj, ok := s.srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d missing from the world", id)
	}
	obj.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
	s.quiet(t)
}

// selectPlayer has Caster select the player id.
func (s *socialScene) selectPlayer(t *testing.T, id int32) {
	t.Helper()
	x, y, z := s.srv.PlayerPosition(t, id)
	s.caster.Send(encodeAction(id, int32(x), int32(y), int32(z), false))
	s.quiet(t)
}

// cast has Caster cast the scene skill, with or without CTRL, at its
// selection id: started reports whether the cast starts on it, otherwise
// the cast must be refused as an invalid target.
func (s *socialScene) cast(t *testing.T, def modelskill.Definition, id int32, ctrl, started bool) {
	t.Helper()
	s.caster.Send(encodeRequestMagicSkillUse(int32(def.ID), ctrl, false))
	if started {
		readCastStartFrames(t, s.caster, s.casterID, int32(def.ID), 1, 500, 60_000, id)
		return
	}
	assertStaticSystemMessage(t, s.caster.Read(), serverpackets.SystemMessageInvalidTarget)
	assertNoActionFailedUntilQuiet(t, s.caster, "refused social cast")
}

// TestSingleTargetSocialPolicy casts single-target skills at a party
// member and at a stranger. Playable.canCastOffensiveSkillOnPlayable
// (Playable.java:383-445): a party member may only take a CTRL damage
// skill, flagged or not, while a flagged stranger takes a debuff without
// CTRL and no debuff reaches an unflagged player.
// Player.canCastBeneficialSkillOnPlayable (Player.java:4826-4864): a
// flagged party member is helped freely, a flagged stranger only with CTRL.
func TestSingleTargetSocialPolicy(t *testing.T) {
	t.Parallel()
	debuff := targetedSkill(socialSkillID, modelskill.TargetOne, true)
	debuff.SkillType, debuff.Debuff = "DEBUFF", true
	damage := targetedSkill(socialSkillID, modelskill.TargetOne, true)
	damage.SkillType = "MDAM"
	buff := targetedSkill(socialSkillID, modelskill.TargetOne, false)
	for _, tt := range []struct {
		name     string
		def      modelskill.Definition
		member   bool
		flagged  bool
		ctrl     bool
		accepted bool
	}{
		{"debuff on a flagged party member", debuff, true, true, false, false},
		{"debuff on a flagged stranger", debuff, false, true, false, true},
		{"CTRL debuff on a party member", debuff, true, false, true, false},
		{"CTRL debuff on an unflagged stranger", debuff, false, false, true, false},
		{"CTRL damage on a party member", damage, true, false, true, true},
		{"damage on a flagged party member", damage, true, true, false, false},
		{"buff on a flagged party member", buff, true, true, false, true},
		{"buff on a flagged stranger", buff, false, true, false, false},
		{"CTRL buff on a flagged stranger", buff, false, true, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSocialScene(t, tt.def)
			s.formParty(t)
			target := s.strangerID
			if tt.member {
				target = s.mateID
			}
			if tt.flagged {
				s.flag(t, target)
			}
			s.selectPlayer(t, target)
			s.cast(t, tt.def, target, tt.ctrl, tt.accepted)
		})
	}
}

// launchedTargets casts def and returns the target ids its
// MagicSkillLaunched lists.
func launchedTargets(t *testing.T, s *socialScene, def modelskill.Definition) []int32 {
	t.Helper()
	s.caster.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	for range 20 {
		frame := s.caster.Read()
		if frame[0] != serverpackets.OpcodeMagicSkillLaunched {
			continue
		}
		r := wireReader(frame[1:])
		r.ReadInt32() // caster
		r.ReadInt32() // skill
		r.ReadInt32() // level
		ids := make([]int32, r.ReadInt32())
		for i := range ids {
			ids[i] = r.ReadInt32()
		}
		return ids
	}
	t.Fatal("no MagicSkillLaunched")
	return nil
}

// TestPartySkillSweepsPartyMembers casts a PARTY buff: TargetParty lists
// the caster, then every living playable in radius sharing its party
// (TargetParty.java:24-45), so the party member is affected and the
// stranger standing beside it is not. Before the party forms, the caster
// is alone.
func TestPartySkillSweepsPartyMembers(t *testing.T) {
	t.Parallel()
	def := targetedSkill(socialPartySkillID, modelskill.TargetParty, false)
	for _, tt := range []struct {
		name  string
		party bool
	}{{"in a party", true}, {"alone", false}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSocialScene(t, def)
			want := []int32{s.casterID}
			if tt.party {
				s.formParty(t)
				want = append(want, s.mateID)
			}
			if got := launchedTargets(t, s, def); !slices.Equal(got, want) {
				t.Fatalf("PARTY targets = %v, want %v", got, want)
			}
		})
	}
}

// TestPartyOtherSkillReachesPartyMember casts a PARTY_OTHER skill at a party
// member: TargetPartyOther.meetCastConditions accepts a living player in the
// caster's party (TargetPartyOther.java:40-66), so the cast starts on it.
func TestPartyOtherSkillReachesPartyMember(t *testing.T) {
	t.Parallel()
	def := targetedSkill(partyOtherSkillID, modelskill.TargetPartyOther, false)
	s := bootSocialScene(t, def)
	s.formParty(t)
	s.selectPlayer(t, s.mateID)
	s.cast(t, def, s.mateID, false, true)
}

// TestCubicSparesFlaggedPartyMember pins Cubic.pickEnemyTarget's
// isAttackableWithoutForceBy gate (Playable.java:497-534) on a party:
// a PvP-flagged member of the owner's party needs force, so the owner's
// cubic never fires at it, while the same flag on a stranger draws fire.
func TestCubicSparesFlaggedPartyMember(t *testing.T) {
	t.Parallel()
	s := bootSocialScene(t, targetedSkill(socialSkillID, modelskill.TargetOne, false))
	s.formParty(t)
	s.flag(t, s.mateID)
	s.flag(t, s.strangerID)

	ownerObj, _ := s.srv.State.Player(s.casterID)
	ownerChar, ok := network.OnlineCharacter(ownerObj)
	if !ok {
		t.Fatalf("owner is %T", ownerObj)
	}
	mate, _ := s.srv.State.Player(s.mateID)
	if _, _, ok := actorcast.DecideCubicFire(gateCubicOwner{Character: ownerChar, target: mate}, []int{4049}, 100); ok {
		t.Fatal("a cubic fires at a flagged party member")
	}
	stranger, _ := s.srv.State.Player(s.strangerID)
	if _, got, ok := actorcast.DecideCubicFire(gateCubicOwner{Character: ownerChar, target: stranger}, []int{4049}, 100); !ok || got.ObjectID() != s.strangerID {
		t.Fatalf("cubic target = %v, %v, want the flagged stranger", got, ok)
	}
}

// socialClans seeds Caster and Mate in Knights, led by Caster, and
// Stranger leading Rivals; allied puts both clans in Knights' alliance.
func socialClans(t *testing.T, allied bool) gameservertest.Option {
	return socialClansLedBy(t, allied, "Caster")
}

// socialClansLedBy is socialClans with Knights led by the character named
// leader.
func socialClansLedBy(t *testing.T, allied bool, leader string) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		ally := "0"
		if allied {
			ally = strconv.Itoa(int(socialKnightsID))
		}
		for _, stmt := range []string{
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, ally_id, ally_name)
				SELECT ` + strconv.Itoa(int(socialKnightsID)) + `, 'Knights', 5, obj_Id, ` + ally + `, 'Pact' FROM characters WHERE char_name = '` + leader + `'`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, ally_id, ally_name)
				VALUES (` + strconv.Itoa(int(socialRivalsID)) + `, 'Rivals', 5, ` + strconv.Itoa(int(socialStrangerID)) + `, ` + ally + `, 'Pact')`,
			`UPDATE characters SET clanid = ` + strconv.Itoa(int(socialKnightsID)) + `, power_grade = 6 WHERE char_name IN ('Caster', 'Mate')`,
			`UPDATE characters SET clanid = ` + strconv.Itoa(int(socialRivalsID)) + `, power_grade = 6 WHERE char_name = 'Stranger'`,
		} {
			if _, err := db.ExecContext(context.Background(), stmt); err != nil {
				t.Fatalf("seed clans: %v", err)
			}
		}
	})
}

// TestAllySkillSweepsClanAndAlliance casts an ALLY buff: TargetAlly lists
// the caster, then every living playable in radius sharing its clan or
// alliance (TargetAlly.java:24-54). A clanmate is always affected; a
// player of another clan only once both clans share an alliance.
func TestAllySkillSweepsClanAndAlliance(t *testing.T) {
	t.Parallel()
	def := targetedSkill(socialPartySkillID, modelskill.TargetAlly, false)
	for _, tt := range []struct {
		name   string
		allied bool
	}{{"clans apart", false}, {"clans allied", true}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSocialScene(t, def, socialClans(t, tt.allied))
			want := []int32{s.casterID, s.mateID}
			if tt.allied {
				want = append(want, s.strangerID)
			}
			got := launchedTargets(t, s, def)
			if len(got) == 0 || got[0] != s.casterID {
				t.Fatalf("ALLY targets = %v, want the caster first", got)
			}
			slices.Sort(got[1:])
			slices.Sort(want[1:])
			if !slices.Equal(got, want) {
				t.Fatalf("ALLY targets = %v, want %v", got, want)
			}
		})
	}
}

// TestClanmateNeedsForce casts a debuff at a PvP-flagged clanmate and at a
// PvP-flagged player of an allied clan: a shared clan or alliance outranks
// the flag (Playable.java:410-412), so both are invalid targets without
// CTRL.
func TestClanmateNeedsForce(t *testing.T) {
	t.Parallel()
	def := targetedSkill(socialSkillID, modelskill.TargetOne, true)
	def.SkillType, def.Debuff = "DEBUFF", true
	for _, tt := range []struct {
		name     string
		stranger bool
	}{{"clanmate", false}, {"allied clan member", true}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSocialScene(t, def, socialClans(t, true))
			target := s.mateID
			if tt.stranger {
				target = s.strangerID
			}
			s.flag(t, target)
			s.selectPlayer(t, target)
			s.cast(t, def, target, false, false)
		})
	}
}

// TestPledgeClassConditionReadsClanLeadership casts a skill carrying
// <player pledgeClass="-1"/>: ConditionPlayerPledgeClass accepts only the
// clan leader (ConditionPlayerPledgeClass.java:18-28), so the leader's cast
// starts and a mere member's is refused by name.
func TestPledgeClassConditionReadsClanLeadership(t *testing.T) {
	t.Parallel()
	def := targetedSkill(socialSkillID, modelskill.TargetSelf, false)
	def.Conditions = []modelskill.ConditionClause{{
		Root:      modelskill.Condition{Kind: "player", Attrs: map[string]string{"pledgeClass": "-1"}},
		MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true,
	}}
	for _, tt := range []struct {
		name   string
		leader bool
	}{{"clan leader", true}, {"clan member", false}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			leader := "Mate"
			if tt.leader {
				leader = "Caster"
			}
			s := bootSocialScene(t, def, socialClansLedBy(t, false, leader))
			s.caster.Send(encodeRequestMagicSkillUse(socialSkillID, false, false))
			if tt.leader {
				readCastStartFrames(t, s.caster, s.casterID, socialSkillID, 1, 500, 60_000, s.casterID)
				return
			}
			assertSystemMessageSkillFrame(t, s.caster.Read(), serverpackets.SystemMessageS1CannotBeUsed, socialSkillID, 1)
			assertNoActionFailedUntilQuiet(t, s.caster, "member's leader-only skill")
		})
	}
}

// relationOf returns the relation and auto-attackable flag of the first
// RelationChanged describing objectID in frames.
func relationOf(t *testing.T, frames [][]byte, objectID int32) (relation, autoAttackable int32) {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeRelationChanged {
			continue
		}
		r := wireReader(f[1:])
		if r.ReadInt32() != objectID {
			continue
		}
		return r.ReadInt32(), r.ReadInt32()
	}
	t.Fatalf("no RelationChanged for %d", objectID)
	return 0, 0
}

// TestFlaggedPartyMemberRelationNeedsForce PvP-flags a party member: every
// nearby player is sent its relation (Player.broadcastRelationsChanges),
// flag bit set, and only the stranger may attack it without force
// (Playable.isAttackableWithoutForceBy, Playable.java:497-534).
func TestFlaggedPartyMemberRelationNeedsForce(t *testing.T) {
	t.Parallel()
	s := bootSocialScene(t, targetedSkill(socialSkillID, modelskill.TargetOne, false))
	s.formParty(t)
	obj, _ := s.srv.State.Player(s.mateID)
	obj.(interface{ UpdatePvPFlag(task.PvPFlagState) }).UpdatePvPFlag(task.PvPFlagOn)
	for _, tt := range []struct {
		name string
		c    *testsupport.ScriptedClient
		auto int32
	}{{"party leader", s.caster, 0}, {"stranger", s.stranger, 1}} {
		relation, auto := relationOf(t, readUntilQuiet(t, tt.c), s.mateID)
		if relation != serverpackets.RelationPvPFlag || auto != tt.auto {
			t.Fatalf("%s sees the flagged member as relation %#x auto-attackable %d, want %#x/%d", tt.name, relation, auto, serverpackets.RelationPvPFlag, tt.auto)
		}
	}
}
