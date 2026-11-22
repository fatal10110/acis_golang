package skills

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: SummonFriend.useSkill, checkSummoner, checkSummoned and
// teleportTo (handler/skillhandlers/SummonFriend.java:25-199),
// Player.teleportAnswer (Player.java:6902-6923) and the SUMMON_FRIEND case of
// PlayerCast.canCast (PlayerCast.java:372-376). checkSummoner tells its
// player YOU_MAY_NOT_SUMMON_FROM_YOUR_CURRENT_LOCATION (a mounted one
// nothing); checkSummoned(player, target) tells player, in order,
// CANNOT_USE_ON_YOURSELF, S1_IS_DEAD_AT_THE_MOMENT_AND_CANNOT_BE_SUMMONED,
// S1_CURRENTLY_TRADING_OR_OPERATING_PRIVATE_STORE_AND_CANNOT_BE_SUMMONED,
// S1_IS_ENGAGED_IN_COMBAT_AND_CANNOT_BE_SUMMONED (rooted or in combat),
// YOU_CANNOT_SUMMON_PLAYERS_WHO_ARE_IN_OLYMPIAD,
// YOUR_TARGET_IS_IN_AN_AREA_WHICH_BLOCKS_SUMMONING (festival or mounted) and
// S1_IN_SUMMON_BLOCKING_AREA, each S1 the target's name. teleportTo(summoned,
// caster) re-runs both with the roles swapped, so the summoned player hears
// them about the caster, then S1_REQUIRED_FOR_SUMMONING for a short item.

const (
	summonDirectSkill         int32 = 1404
	summonConfirmSkill        int32 = 1403
	summonPartySkill          int32 = 1429
	summonDirectItemSkill     int32 = 9404
	summonPartyItemSkill      int32 = 9429
	summonCasterItemID        int32 = 20
	summonTargetItemID        int32 = 20
	summonStriderNPCID        int32 = 12526
	summonCasterName                = "Caster"
	summonTargetName                = "Friend"
	summonFriendMPConsume           = 10
	summonFriendCastSettle          = time.Second
	summonFriendRootSkillID         = 100
	summonFriendLevel               = 40
	summonFriendHeldItemCount       = 10
)

func summonFriendSkills() []modelskill.Definition {
	def := func(id int32, typ string, target modelskill.Target, targetItem int) modelskill.Definition {
		d := modelskill.Definition{
			ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: target,
			HitTime: 500, StaticHitTime: true, StaticReuse: true, CastRange: 600,
			MPConsume: summonFriendMPConsume, ItemConsumeID: int(summonCasterItemID), ItemConsumeCount: 1,
			SkillType: typ,
		}
		if targetItem > 0 {
			extra := *d.Extra()
			extra.TargetConsumeID, extra.TargetConsumeCount = int(summonTargetItemID), targetItem
			d = d.WithExtra(extra)
		}
		return d
	}
	return []modelskill.Definition{
		def(summonDirectSkill, "SUMMON_FRIEND", modelskill.TargetOne, 0),
		def(summonConfirmSkill, "SUMMON_FRIEND", modelskill.TargetOne, 1),
		def(summonPartySkill, "SUMMON_PARTY", modelskill.TargetSelf, 0),
		def(summonDirectItemSkill, "SUMMON_FRIEND", modelskill.TargetOne, summonFriendHeldItemCount+1),
		def(summonPartyItemSkill, "SUMMON_PARTY", modelskill.TargetSelf, summonFriendHeldItemCount+1),
	}
}

type summonWorld struct {
	srv                *gameservertest.Server
	caster, target     *testsupport.ScriptedClient
	casterID, targetID int32
}

// bootSummonWorld logs a caster knowing every summon skill and its friend in
// next to each other, both holding summonFriendHeldItemCount of the consume
// item.
func bootSummonWorld(t *testing.T) summonWorld {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter(summonCasterName, summonFriendLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, summonFriendSkills())),
	)
	w := summonWorld{srv: srv, caster: srv.Client, casterID: srv.SoleObjectID(t)}
	for _, def := range summonFriendSkills() {
		seedKnownSkill(t, srv, w.casterID, int(def.ID), 1)
	}
	srv.GiveItem(t, w.casterID, summonCasterItemID, summonFriendHeldItemCount)
	startInWorld(t, w.caster)
	w.targetID = srv.SeedCharacterFor(t, "player2", summonTargetName, summonFriendLevel, 0).ID
	srv.GiveItem(t, w.targetID, summonTargetItemID, summonFriendHeldItemCount)
	w.target = srv.DialClient(t, "player2", 1)
	startInWorldAmongPlayers(t, w.target)
	w.drain(t)
	return w
}

func (w summonWorld) drain(t *testing.T) {
	t.Helper()
	drainUntilQuiet(t, w.caster)
	drainUntilQuiet(t, w.target)
}

// selectTarget has the caster click objID.
func (w summonWorld) selectTarget(t *testing.T, objID int32) {
	t.Helper()
	x, y, z := w.srv.PlayerPosition(t, objID)
	w.caster.Send(encodeAction(objID, int32(x), int32(y), int32(z), false))
	w.drain(t)
}

// formParty invites the friend into the caster's party.
func (w summonWorld) formParty(t *testing.T) {
	t.Helper()
	w.caster.Send(encodePartyInvite(summonTargetName))
	w.drain(t)
	w.target.Send(encodePartyAnswer(1))
	w.drain(t)
}

// cast has the caster use skillID and lets the cast end.
func (w summonWorld) cast(t *testing.T, skillID int32) {
	t.Helper()
	w.caster.Send(encodeRequestMagicSkillUse(skillID, false, false))
	w.srv.Advance(t, summonFriendCastSettle)
}

func (w summonWorld) on(t *testing.T, objID int32, fn func(*player.Character)) {
	t.Helper()
	onPlayerQueue(t, w.srv, objID, fn)
}

func encodeSummonDlgAnswer(answer, requesterID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeDlgAnswer)
	w.WriteInt32(serverpackets.ConfirmDlgSummonFriendRequest)
	w.WriteInt32(answer)
	w.WriteInt32(requesterID)
	return w.Bytes()
}

// summonMessageIDs are the system messages a summon-friend gate tells.
var summonMessageIDs = map[int32]bool{
	serverpackets.SystemMessageCannotUseOnYourself:               true,
	serverpackets.SystemMessageCannotSummonFromLocation:          true,
	serverpackets.SystemMessageS1EngagedInCombatCannotBeSummoned: true,
	serverpackets.SystemMessageS1DeadCannotBeSummoned:            true,
	serverpackets.SystemMessageS1InSummonBlockingArea:            true,
	serverpackets.SystemMessageS1AlreadySummoned:                 true,
	serverpackets.SystemMessageS1RequiredForSummoning:            true,
	serverpackets.SystemMessageS1OperatingCannotBeSummoned:       true,
	serverpackets.SystemMessageTargetInSummonBlockingArea:        true,
	serverpackets.SystemMessageCannotSummonOlympiadPlayers:       true,
}

// summonMessages returns the summon-gate system messages among frames.
func summonMessages(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if len(f) < 5 || f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if summonMessageIDs[wire.NewReader(f[1:]).ReadInt32()] {
			out = append(out, f)
		}
	}
	return out
}

// assertSummonMessage asserts frames carry exactly one summon-gate message,
// want, or none when want is nil.
func assertSummonMessage(t *testing.T, who string, frames [][]byte, want []byte) {
	t.Helper()
	got := summonMessages(frames)
	switch {
	case want == nil && len(got) != 0:
		t.Fatalf("%s summon messages = %x, want none", who, got)
	case want != nil && (len(got) != 1 || !bytes.Equal(got[0], want)):
		t.Fatalf("%s summon messages = %x, want only %x", who, got, want)
	}
}

func teleported(frames [][]byte) bool {
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeTeleportToLocation {
			return true
		}
	}
	return false
}

// payload is frame as the client reads it: without the length prefix.
func payload(frame wire.Frame) []byte { return frame.Bytes()[2:] }

func nameMessage(id int, name string) []byte {
	return payload(serverpackets.FrameSystemMessageString(id, name))
}

func plainMessage(id int) []byte { return payload(serverpackets.FrameSystemMessage(id)) }

func itemMessage(id int, itemID int32) []byte {
	return payload(serverpackets.FrameSystemMessageItemName(id, itemID))
}

func addRoot(t *testing.T, pc *player.Character) {
	root, err := effect.New(effect.Skill{ID: summonFriendRootSkillID}, modelskill.EffectTemplate{Name: "Root", Time: 60})
	if err != nil {
		t.Errorf("new root effect: %v", err)
		return
	}
	root.Effected = pc
	pc.EffectList().Add(root)
}

// summonedGateCases are checkSummoned's refusals of the friend, told to the
// caster, in the reference's order of checks.
func summonedGateCases() []struct {
	name  string
	setup func(*testing.T, *player.Character)
	want  []byte
} {
	return []struct {
		name  string
		setup func(*testing.T, *player.Character)
		want  []byte
	}{
		{"dead", func(t *testing.T, pc *player.Character) { pc.MarkDead() }, nameMessage(serverpackets.SystemMessageS1DeadCannotBeSummoned, summonTargetName)},
		{"operating", func(t *testing.T, pc *player.Character) { pc.SetOperateType(privatestore.OperateSell) }, nameMessage(serverpackets.SystemMessageS1OperatingCannotBeSummoned, summonTargetName)},
		{"in combat", func(t *testing.T, pc *player.Character) { pc.SetInCombat(true) }, nameMessage(serverpackets.SystemMessageS1EngagedInCombatCannotBeSummoned, summonTargetName)},
		{"rooted", addRoot, nameMessage(serverpackets.SystemMessageS1EngagedInCombatCannotBeSummoned, summonTargetName)},
		{"olympiad", func(t *testing.T, pc *player.Character) { pc.SetOlympiadMode(true) }, plainMessage(serverpackets.SystemMessageCannotSummonOlympiadPlayers)},
		{"mounted", func(t *testing.T, pc *player.Character) { pc.Mount(summonStriderNPCID, 0) }, plainMessage(serverpackets.SystemMessageTargetInSummonBlockingArea)},
		{"no-summon zone", func(t *testing.T, pc *player.Character) { holdZone(pc, zone.FlagNoSummonFriend) }, nameMessage(serverpackets.SystemMessageS1InSummonBlockingArea, summonTargetName)},
		{
			// Rooted and in a no-summon zone: the earlier check answers.
			"order", func(t *testing.T, pc *player.Character) { pc.SetInCombat(true); holdZone(pc, zone.FlagNoSummonFriend) },
			nameMessage(serverpackets.SystemMessageS1EngagedInCombatCannotBeSummoned, summonTargetName),
		},
	}
}

// TestSummonFriendCastStartRefusal casts SUMMON_FRIEND at a friend the gates
// refuse, or from where the caster may not summon: the cast is refused at
// its start with the reason alone, told to the caster, and costs neither MP
// nor the consume item; the friend stays put.
func TestSummonFriendCastStartRefusal(t *testing.T) {
	t.Parallel()
	type tc struct {
		name         string
		self         bool
		casterSetup  func(*testing.T, *player.Character)
		summonedFunc func(*testing.T, *player.Character)
		want         []byte
	}
	cases := []tc{
		{name: "self target", self: true, want: plainMessage(serverpackets.SystemMessageCannotUseOnYourself)},
		{name: "caster in no-summon zone", casterSetup: func(t *testing.T, pc *player.Character) { holdZone(pc, zone.FlagNoSummonFriend) }, want: plainMessage(serverpackets.SystemMessageCannotSummonFromLocation)},
		{name: "caster in olympiad", casterSetup: func(t *testing.T, pc *player.Character) { pc.SetOlympiadMode(true) }, want: plainMessage(serverpackets.SystemMessageCannotSummonFromLocation)},
		{name: "caster mounted", casterSetup: func(t *testing.T, pc *player.Character) { pc.Mount(summonStriderNPCID, 0) }},
	}
	for _, g := range summonedGateCases() {
		cases = append(cases, tc{name: "friend " + g.name, summonedFunc: g.setup, want: g.want})
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootSummonWorld(t)
			if tt.casterSetup != nil {
				w.on(t, w.casterID, func(pc *player.Character) { tt.casterSetup(t, pc) })
			}
			if tt.summonedFunc != nil {
				w.on(t, w.targetID, func(pc *player.Character) { tt.summonedFunc(t, pc) })
			}
			w.drain(t)
			if tt.self {
				w.selectTarget(t, w.casterID)
			} else {
				w.selectTarget(t, w.targetID)
			}
			mpBefore := w.srv.PlayerCurrentMP(t, w.casterID)

			w.cast(t, summonDirectSkill)
			frames := w.caster.ReadQueued()
			if tt.want == nil {
				if len(frames) != 0 {
					t.Fatalf("silent refusal sent caster opcodes %x, want none", opcodes(frames))
				}
			} else if len(frames) != 1 || !bytes.Equal(frames[0], tt.want) {
				t.Fatalf("caster frames = %x, want only %x", frames, tt.want)
			}
			if got := w.srv.PlayerCurrentMP(t, w.casterID); got != mpBefore {
				t.Fatalf("caster MP = %d, want unchanged %d", got, mpBefore)
			}
			if got := w.srv.PlayerItemCount(t, w.casterID, summonCasterItemID); got != summonFriendHeldItemCount {
				t.Fatalf("caster consume item = %d, want unchanged %d", got, summonFriendHeldItemCount)
			}
			if w.srv.PlayerCastingNow(t, w.casterID) {
				t.Fatal("refused cast is in flight")
			}
			if friend := w.target.ReadQueued(); teleported(friend) {
				t.Fatal("refused cast teleported the friend")
			}
		})
	}
}

// TestSummonPartyRefusalsTellCaster casts SUMMON_PARTY with a member each
// checkSummoned gate refuses: the cast goes through (SUMMON_PARTY has no
// cast-start gate), the caster hears the member's reason and the member
// stays put. A clear member is pulled.
func TestSummonPartyRefusalsTellCaster(t *testing.T) {
	t.Parallel()
	cases := summonedGateCases()
	cases = append(cases, struct {
		name  string
		setup func(*testing.T, *player.Character)
		want  []byte
	}{name: "clear"})
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootSummonWorld(t)
			w.formParty(t)
			if tt.setup != nil {
				w.on(t, w.targetID, func(pc *player.Character) { tt.setup(t, pc) })
			}
			w.drain(t)
			mpBefore := w.srv.PlayerCurrentMP(t, w.casterID)

			w.cast(t, summonPartySkill)
			assertSummonMessage(t, "caster", w.caster.ReadQueued(), tt.want)
			if got := w.srv.PlayerCurrentMP(t, w.casterID); got != mpBefore-summonFriendMPConsume {
				t.Fatalf("caster MP = %d, want %d after the cast", got, mpBefore-summonFriendMPConsume)
			}
			friend := w.target.ReadQueued()
			assertSummonMessage(t, "member", friend, nil)
			if got := teleported(friend); got != (tt.want == nil) {
				t.Fatalf("member teleported = %v, want %v", got, tt.want == nil)
			}
		})
	}
}

// TestSummonFriendSwappedRecheck casts the direct SUMMON_FRIEND and
// SUMMON_PARTY from a caster in combat or rooted: the summoner gate passes
// at cast start, but the teleport re-runs checkSummoned with the roles
// swapped, so the summoned friend hears the caster's state by name and is
// not pulled. A clear caster pulls it.
func TestSummonFriendSwappedRecheck(t *testing.T) {
	t.Parallel()
	combat := nameMessage(serverpackets.SystemMessageS1EngagedInCombatCannotBeSummoned, summonCasterName)
	for _, tt := range []struct {
		name  string
		skill int32
		setup func(*testing.T, *player.Character)
		want  []byte
	}{
		{"direct, caster in combat", summonDirectSkill, func(t *testing.T, pc *player.Character) { pc.SetInCombat(true) }, combat},
		{"direct, caster rooted", summonDirectSkill, addRoot, combat},
		{"direct, clear caster", summonDirectSkill, nil, nil},
		{"party, caster in combat", summonPartySkill, func(t *testing.T, pc *player.Character) { pc.SetInCombat(true) }, combat},
		{"party, clear caster", summonPartySkill, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootSummonWorld(t)
			if tt.skill == summonPartySkill {
				w.formParty(t)
			} else {
				w.selectTarget(t, w.targetID)
			}
			if tt.setup != nil {
				w.on(t, w.casterID, func(pc *player.Character) { tt.setup(t, pc) })
			}
			w.drain(t)

			w.cast(t, tt.skill)
			assertSummonMessage(t, "caster", w.caster.ReadQueued(), nil)
			friend := w.target.ReadQueued()
			assertSummonMessage(t, "friend", friend, tt.want)
			if got := teleported(friend); got != (tt.want == nil) {
				t.Fatalf("friend teleported = %v, want %v", got, tt.want == nil)
			}
		})
	}
}

// TestSummonFriendTargetItemShortage casts a summon skill that takes more
// of the consume item than the friend holds: the friend hears
// S1_REQUIRED_FOR_SUMMONING naming the item, keeps its items and stays put.
func TestSummonFriendTargetItemShortage(t *testing.T) {
	t.Parallel()
	for _, skill := range []int32{summonDirectItemSkill, summonPartyItemSkill} {
		t.Run(fmt.Sprint(skill), func(t *testing.T) {
			t.Parallel()
			w := bootSummonWorld(t)
			if skill == summonPartyItemSkill {
				w.formParty(t)
			} else {
				w.selectTarget(t, w.targetID)
			}
			w.cast(t, skill)
			assertSummonMessage(t, "caster", w.caster.ReadQueued(), nil)
			friend := w.target.ReadQueued()
			assertSummonMessage(t, "friend", friend, itemMessage(serverpackets.SystemMessageS1RequiredForSummoning, summonTargetItemID))
			if teleported(friend) {
				t.Fatal("teleported a friend short of the item")
			}
			if got := w.srv.PlayerItemCount(t, w.targetID, summonTargetItemID); got != summonFriendHeldItemCount {
				t.Fatalf("friend item = %d, want unchanged %d", got, summonFriendHeldItemCount)
			}
		})
	}
}

// TestSummonFriendAlreadyPending casts the confirm summon twice at the same
// friend: the second finds the first request pending and tells the caster
// S1_ALREADY_SUMMONED with the friend's name.
func TestSummonFriendAlreadyPending(t *testing.T) {
	t.Parallel()
	w := bootSummonWorld(t)
	w.selectTarget(t, w.targetID)
	w.cast(t, summonConfirmSkill)
	assertSummonMessage(t, "caster", w.caster.ReadQueued(), nil)
	w.drain(t)

	w.cast(t, summonConfirmSkill)
	assertSummonMessage(t, "caster", w.caster.ReadQueued(), nameMessage(serverpackets.SystemMessageS1AlreadySummoned, summonTargetName))
	assertSummonMessage(t, "friend", w.target.ReadQueued(), nil)
}

// TestSummonFriendAcceptGates casts the confirm summon, changes either side
// while the dialog is open, then accepts: the accept re-runs checkSummoner
// on the answering friend and checkSummoned with the caster as the summoned
// side, so the friend hears each refusal, then S1_REQUIRED_FOR_SUMMONING for
// a short item. A clear accept pulls the friend and takes its item.
func TestSummonFriendAcceptGates(t *testing.T) {
	t.Parallel()
	about := func(id int) []byte { return nameMessage(id, summonCasterName) }
	for _, tt := range []struct {
		name          string
		caster, other func(*testing.T, *player.Character)
		want          []byte
	}{
		{name: "caster dead", caster: func(t *testing.T, pc *player.Character) { pc.MarkDead() }, want: about(serverpackets.SystemMessageS1DeadCannotBeSummoned)},
		{name: "caster operating", caster: func(t *testing.T, pc *player.Character) { pc.SetOperateType(privatestore.OperateSell) }, want: about(serverpackets.SystemMessageS1OperatingCannotBeSummoned)},
		{name: "caster in combat", caster: func(t *testing.T, pc *player.Character) { pc.SetInCombat(true) }, want: about(serverpackets.SystemMessageS1EngagedInCombatCannotBeSummoned)},
		{name: "caster rooted", caster: addRoot, want: about(serverpackets.SystemMessageS1EngagedInCombatCannotBeSummoned)},
		{name: "caster in olympiad", caster: func(t *testing.T, pc *player.Character) { pc.SetOlympiadMode(true) }, want: plainMessage(serverpackets.SystemMessageCannotSummonOlympiadPlayers)},
		{name: "caster mounted", caster: func(t *testing.T, pc *player.Character) { pc.Mount(summonStriderNPCID, 0) }, want: plainMessage(serverpackets.SystemMessageTargetInSummonBlockingArea)},
		{name: "caster in no-summon zone", caster: func(t *testing.T, pc *player.Character) { holdZone(pc, zone.FlagNoSummonFriend) }, want: about(serverpackets.SystemMessageS1InSummonBlockingArea)},
		{name: "answerer in no-summon zone", other: func(t *testing.T, pc *player.Character) { holdZone(pc, zone.FlagNoSummonFriend) }, want: plainMessage(serverpackets.SystemMessageCannotSummonFromLocation)},
		{name: "answerer in olympiad", other: func(t *testing.T, pc *player.Character) { pc.SetOlympiadMode(true) }, want: plainMessage(serverpackets.SystemMessageCannotSummonFromLocation)},
		{
			name: "answerer short of the item",
			other: func(t *testing.T, pc *player.Character) {
				if !pc.ConsumeItem(int(summonTargetItemID), summonFriendHeldItemCount) {
					t.Error("drop the friend's consume item")
				}
			},
			want: itemMessage(serverpackets.SystemMessageS1RequiredForSummoning, summonTargetItemID),
		},
		{name: "clear"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootSummonWorld(t)
			w.selectTarget(t, w.targetID)
			w.cast(t, summonConfirmSkill)
			w.drain(t)
			if tt.caster != nil {
				w.on(t, w.casterID, func(pc *player.Character) { tt.caster(t, pc) })
			}
			if tt.other != nil {
				w.on(t, w.targetID, func(pc *player.Character) { tt.other(t, pc) })
			}
			w.drain(t)
			itemsBefore := w.srv.PlayerItemCount(t, w.targetID, summonTargetItemID)

			w.target.Send(encodeSummonDlgAnswer(1, w.casterID))
			w.srv.Settle(t)
			friend := w.target.ReadQueued()
			assertSummonMessage(t, "friend", friend, tt.want)
			assertSummonMessage(t, "caster", w.caster.ReadQueued(), nil)
			if got := teleported(friend); got != (tt.want == nil) {
				t.Fatalf("friend teleported = %v, want %v", got, tt.want == nil)
			}
			if tt.want == nil {
				assertItemNamedBeforeTeleport(t, friend)
			}
			wantItems := itemsBefore
			if tt.want == nil {
				wantItems--
			}
			if got := w.srv.PlayerItemCount(t, w.targetID, summonTargetItemID); got != wantItems {
				t.Fatalf("friend item = %d, want %d", got, wantItems)
			}
		})
	}
}

// assertItemNamedBeforeTeleport asserts the friend read S1_DISAPPEARED
// naming the one consume item it paid ahead of its TeleportToLocation, the
// destroy's own notice.
func assertItemNamedBeforeTeleport(t *testing.T, frames [][]byte) {
	t.Helper()
	want := itemMessage(serverpackets.SystemMessageS1Disappeared, summonTargetItemID)
	for _, f := range frames {
		if bytes.Equal(f, want) {
			return
		}
		if f[0] == serverpackets.OpcodeTeleportToLocation {
			break
		}
	}
	t.Fatalf("friend frames = %x, want S1_DISAPPEARED %x ahead of the teleport", frames, want)
}
