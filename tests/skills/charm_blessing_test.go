package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// etcStatusFrame is one decoded EtcStatusUpdate.
type etcStatusFrame struct {
	charges, weightPenalty, blocked, danger, gradePenalty, charm, deathPenalty int32
}

func etcStatusFrames(t *testing.T, frames [][]byte) []etcStatusFrame {
	t.Helper()
	var out []etcStatusFrame
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeEtcStatusUpdate {
			continue
		}
		r := wire.NewReader(f[1:])
		s := etcStatusFrame{
			charges: r.ReadInt32(), weightPenalty: r.ReadInt32(), blocked: r.ReadInt32(),
			danger: r.ReadInt32(), gradePenalty: r.ReadInt32(), charm: r.ReadInt32(), deathPenalty: r.ReadInt32(),
		}
		if err := r.Err(); err != nil {
			t.Fatalf("read EtcStatusUpdate: %v", err)
		}
		out = append(out, s)
	}
	return out
}

func requireCharmStatus(t *testing.T, frames [][]byte, charges, charm int32, who, when string) {
	t.Helper()
	got := etcStatusFrames(t, frames)
	if len(got) != 1 || got[0].charm != charm || got[0].charges != charges {
		t.Fatalf("%s: %s EtcStatusUpdates = %+v, want one with charges %d and charm %d", when, who, got, charges, charm)
	}
}

// TestCharmOfCourageBroadcastsEtcStatus casts a Charm of Courage self-buff
// (skill 5041's CharmOfCourage effect) next to an observer.
// EffectCharmOfCourage.onStart and onExit each broadcast the player's
// EtcStatusUpdate to itself and its observers, whose charm field is
// isAffected(CHARM_OF_COURAGE) (EtcStatusUpdate.java:25): 1 while the charm
// is held, 0 once it wore off. A status update for another reason while the
// charm is held carries it too.
func TestCharmOfCourageBroadcastsEtcStatus(t *testing.T) {
	t.Parallel()
	const skillID = 5041
	charm := modelskill.Definition{
		ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "BUFF",
		Effects:   []modelskill.EffectTemplate{{Name: "CharmOfCourage", Time: 2, Count: 1, Icon: true}},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Caster", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{charm})),
	)
	caster, casterID := srv.Client, srv.SoleObjectID(t)
	srv.SeedCharacterFor(t, "player2", "Observer", 5, 0)
	observer := srv.DialClient(t, "player2", 1)
	seedKnownSkill(t, srv, casterID, skillID, 1)
	startInWorld(t, caster)
	startInWorldAmongPlayers(t, observer)
	drainUntilQuiet(t, caster)
	drainUntilQuiet(t, observer)

	caster.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, caster, casterID, skillID, 1, 500, 60_000, casterID)
	srv.Advance(t, 700*time.Millisecond)
	requireCharmStatus(t, collectUntilQuiet(t, caster), 0, 1, "caster", "charm landed")
	requireCharmStatus(t, collectUntilQuiet(t, observer), 0, 1, "observer", "charm landed")

	obj, ok := srv.State.Player(casterID)
	if !ok {
		t.Fatal("caster missing from world state")
	}
	charged, ok := obj.(interface{ IncreaseCharges(count, max int) bool })
	if !ok {
		t.Fatalf("world caster %T does not expose IncreaseCharges", obj)
	}
	if !charged.IncreaseCharges(1, 2) {
		t.Fatal("caster charges were not raised")
	}
	requireCharmStatus(t, collectUntilQuiet(t, caster), 1, 1, "caster", "charge raised under the charm")
	if got := etcStatusFrames(t, collectUntilQuiet(t, observer)); len(got) != 0 {
		t.Fatalf("observer EtcStatusUpdates after the caster's charge = %+v, want none", got)
	}

	srv.Advance(t, 2200*time.Millisecond)
	srv.TickEffects()
	requireCharmStatus(t, collectUntilQuiet(t, caster), 1, 0, "caster", "charm wore off")
	requireCharmStatus(t, collectUntilQuiet(t, observer), 1, 0, "observer", "charm wore off")
}

// charmOfLuckDef is a Charm of Luck self-buff; its effect keeps the
// datapack's default count of 1.
func charmOfLuckDef(id modelskill.ID, order float64, duration int) modelskill.Definition {
	d := slotBuffDef(id)
	d.Effects = []modelskill.EffectTemplate{{Name: "CharmOfLuck", Time: duration, Count: 1, Icon: true, StackType: "reduce_drop_penalty", StackOrder: order}}
	return d
}

type skillMessage struct{ id, skillID int32 }

// skillMessages lists the skill-name system messages among frames, in order.
func skillMessages(t *testing.T, frames [][]byte) []skillMessage {
	t.Helper()
	var out []skillMessage
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		id, params := r.ReadInt32(), r.ReadInt32()
		if params != 1 || r.ReadInt32() != serverpackets.SystemMessageParamSkillName {
			continue
		}
		out = append(out, skillMessage{id: id, skillID: r.ReadInt32()})
	}
	return out
}

func lastBuffIcons(t *testing.T, frames [][]byte) []int32 {
	t.Helper()
	var icons []int32
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeAbnormalStatusUpdate {
			icons = buffSlotIDs(readAbnormalStatusUpdateEntriesFromFrame(t, f))
		}
	}
	return icons
}

// TestDisplacedCharmOfLuckLeavesWithLesserKept lands a stronger Charm of Luck
// (stack reduce_drop_penalty, as skills 1325 and 2168 share) on a held one.
// With CancelLesserEffect off, the displaced charm's onExit
// (Playable.stopCharmOfLuck(this) -> removeEffect) removes it once the
// newcomer took over: after its displacement message and the newcomer's felt
// message it is announced disappeared again, and it is not promoted back when
// the newcomer wears off. With the default config it was already cancelled.
func TestDisplacedCharmOfLuckLeavesWithLesserKept(t *testing.T) {
	t.Parallel()
	const weak, strong = 201, 202
	for _, tc := range []struct {
		name      string
		cancel    bool
		afterCast []skillMessage
	}{
		{
			name:   "cancel lesser",
			cancel: true,
			afterCast: []skillMessage{
				{serverpackets.SystemMessageEffectS1Disappeared, weak},
				{serverpackets.SystemMessageYouFeelS1Effect, strong},
			},
		},
		{
			name: "keep lesser",
			afterCast: []skillMessage{
				{serverpackets.SystemMessageEffectS1Disappeared, weak},
				{serverpackets.SystemMessageYouFeelS1Effect, strong},
				{serverpackets.SystemMessageEffectS1Disappeared, weak},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithCancelLesserEffect(tc.cancel),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
					charmOfLuckDef(weak, 6, 60), charmOfLuckDef(strong, 8, 2),
				})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, weak, 1)
			seedKnownSkill(t, srv, objID, strong, 1)
			startInWorld(t, c)

			castSlotBuff(t, c, objID, weak)
			c.Send(encodeRequestMagicSkillUse(strong, false, false))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, strong, 1)
			frames := collectUntilQuiet(t, c)
			if got := skillMessages(t, frames); !slices.Equal(got, tc.afterCast) {
				t.Fatalf("skill messages after the stronger charm = %+v, want %+v", got, tc.afterCast)
			}
			if icons := lastBuffIcons(t, frames); !slices.Equal(icons, []int32{strong}) {
				t.Fatalf("icons after the stronger charm = %v, want [%d]", icons, strong)
			}
			if ids := liveHeldSkillIDs(t, srv, objID); !slices.Equal(ids, []int32{strong}) {
				t.Fatalf("held effects after the stronger charm = %v, want [%d]", ids, strong)
			}

			srv.Advance(t, 2200*time.Millisecond)
			srv.TickEffects()
			frames = collectUntilQuiet(t, c)
			if got, want := skillMessages(t, frames), []skillMessage{{serverpackets.SystemMessageS1HasWornOff, strong}}; !slices.Equal(got, want) {
				t.Fatalf("skill messages after the stronger charm wore off = %+v, want %+v", got, want)
			}
			if icons := lastBuffIcons(t, frames); len(icons) != 0 {
				t.Fatalf("icons after the stronger charm wore off = %v, want none", icons)
			}
			if ids := liveHeldSkillIDs(t, srv, objID); len(ids) != 0 {
				t.Fatalf("held effects after the stronger charm wore off = %v, want none", ids)
			}
		})
	}
}
