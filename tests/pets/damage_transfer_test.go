package pets

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: PlayerStatus.reduceHp (PlayerStatus.java:153-191). A hit from
// another creature moves TRANSFER_DAMAGE_PERCENT of the damage onto the
// player's servitor — never a pet — within 900 units, capped at the
// servitor's HP minus 1, through summon.reduceCurrentHp(tDmg, attacker,
// null); the player takes the rest, reads S1_GAVE_YOU_S2_DMG with it, and
// an attacking player reads GIVEN_S1_DAMAGE_TO_YOUR_TARGET_AND_S2_DAMAGE_TO_
// SERVITOR (1130) with both amounts. Skill 1262 Transfer Pain level 5 adds
// transDam 50.

const transferPainPercent = 50

// giveTransferPain lands a transDam 50 effect, Transfer Pain level 5's
// stat, on the player id.
func giveTransferPain(t *testing.T, srv *gameservertest.Server, id int32) {
	t.Helper()
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d not in world", id)
	}
	owner := obj.(interface {
		effect.Actor
		EffectList() *effect.List
		CalcStat(stat.Stat, float64) float64
	})
	e, err := effect.New(effect.Skill{ID: 1262, Level: 5}, modelskill.EffectTemplate{
		Name: "Buff", Time: 120,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "transDam", Value: transferPainPercent}},
	})
	if err != nil {
		t.Fatalf("effect.New: %v", err)
	}
	e.Effector, e.Effected = owner, owner
	runOn(t, srv.PlayerQueue(t, id), func() { owner.EffectList().Add(e) })
	if got := owner.CalcStat(stat.TransferDamagePercent, 0); got != transferPainPercent {
		t.Fatalf("owner transDam = %v, want %d", got, transferPainPercent)
	}
}

// joinHitter brings a second player, the attacker, into the world.
func joinHitter(t *testing.T, srv *gameservertest.Server, owner *testsupport.ScriptedClient) secondPlayer {
	t.Helper()
	id := srv.SeedCharacterFor(t, "player2", "Hitter", 1, 0).ID
	c := srv.DialClient(t, "player2", 1)
	startInWorld(t, c)
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatal("hitter not in world")
	}
	drainUntilQuiet(t, owner)
	drainUntilQuiet(t, c)
	return secondPlayer{client: c, id: id, actor: obj.(attackable.Combatant), queue: srv.PlayerQueue(t, id)}
}

// transferMessage is one decoded system message: its id and its text and
// number parameters in order.
type transferMessage struct {
	ID     int32
	Params []any
}

// transferMessages decodes every system message among frames.
func transferMessages(t *testing.T, frames [][]byte) []transferMessage {
	t.Helper()
	var out []transferMessage
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(frame[1:])
		m := transferMessage{ID: r.ReadInt32()}
		for range r.ReadInt32() {
			switch typ := r.ReadInt32(); typ {
			case serverpackets.SystemMessageParamText:
				m.Params = append(m.Params, r.ReadString())
			case serverpackets.SystemMessageParamNumber:
				m.Params = append(m.Params, r.ReadInt32())
			default:
				t.Fatalf("system message %d parameter type %d not decoded here", m.ID, typ)
			}
		}
		out = append(out, m)
	}
	return out
}

// damageMessages keeps the damage reports among messages.
func damageMessages(messages []transferMessage) string {
	var out []string
	for _, m := range messages {
		switch m.ID {
		case serverpackets.SystemMessageS1GaveYouS2Dmg, serverpackets.SystemMessageSummonReceivedS2ByS1,
			serverpackets.SystemMessageGivenS1DamageToTargetS2ToServitor:
			out = append(out, fmt.Sprint(m.ID, m.Params))
		}
	}
	return fmt.Sprint(out)
}

// ownerVitals returns the owner's CP plus HP.
func ownerVitals(t *testing.T, srv *gameservertest.Server, id int32) int {
	t.Helper()
	return srv.PlayerCurrentCP(t, id) + srv.PlayerCurrentHP(t, id)
}

// hitOwner lands a 20-damage hit from hitter on the owner, on the hitter's
// queue, as the hitter's attack would.
func hitOwner(t *testing.T, srv *gameservertest.Server, ownerID int32, hitter secondPlayer) {
	t.Helper()
	obj, _ := srv.State.Player(ownerID)
	owner := obj.(attackable.Combatant)
	runOn(t, hitter.queue, func() { owner.TakeDamage(20, hitter.actor) })
}

// TestServitorTakesTransferPainShare hits a Transfer Pain owner whose
// servitor stands next to it: the servitor takes half of the 20 damage and
// its owner is told so, the owner loses the other half and reads it as the
// damage dealt, and the hitter reads the split.
func TestServitorTakesTransferPainShare(t *testing.T) {
	t.Parallel()
	o := bootServitorOwner(t)
	servitor := o.summonServitor(t)
	giveTransferPain(t, o.srv, o.id)
	hitter := joinHitter(t, o.srv, o.client)
	drainUntilQuiet(t, o.client)
	servitorBefore, ownerBefore := servitor.HP(), ownerVitals(t, o.srv, o.id)

	hitOwner(t, o.srv, o.id, hitter)

	if got := servitorBefore - servitor.HP(); got != 10 {
		t.Fatalf("servitor lost %v HP, want the 10 transferred", got)
	}
	if got := ownerBefore - ownerVitals(t, o.srv, o.id); got != 10 {
		t.Fatalf("owner lost %d CP+HP, want the 10 left after the transfer", got)
	}
	want := fmt.Sprint([]string{
		fmt.Sprint(serverpackets.SystemMessageSummonReceivedS2ByS1, []any{"Hitter", int32(10)}),
		fmt.Sprint(serverpackets.SystemMessageS1GaveYouS2Dmg, []any{"Hitter", int32(10)}),
	})
	if got := damageMessages(transferMessages(t, drainFrames(t, o.client))); got != want {
		t.Fatalf("owner damage messages = %s, want %s", got, want)
	}
	want = fmt.Sprint([]string{fmt.Sprint(serverpackets.SystemMessageGivenS1DamageToTargetS2ToServitor, []any{int32(10), int32(10)})})
	if got := damageMessages(transferMessages(t, drainFrames(t, hitter.client))); got != want {
		t.Fatalf("hitter damage messages = %s, want %s", got, want)
	}
}

// TestNoTransferPainShare hits a Transfer Pain owner whose summon cannot
// take a share: a servitor at 1 HP, a servitor 1000 units away, or a pet.
// The owner takes the whole hit, and the hitter reads no split.
func TestNoTransferPainShare(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32, *summon.Actor)
	}{
		{"servitor at 1 HP", func(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32, *summon.Actor) {
			o := bootServitorOwner(t)
			servitor := o.summonServitor(t)
			o.hurtServitor(t, servitor, servitor.HP()-1)
			return o.srv, o.client, o.id, servitor
		}},
		{"servitor out of range", func(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32, *summon.Actor) {
			o := bootServitorOwner(t)
			servitor := o.summonServitor(t)
			x, y, z := servitor.Position()
			runOn(t, servitor.Queue(), func() { servitor.SetXYZ(x+1000, y, z) })
			drainUntilQuiet(t, o.client)
			return o.srv, o.client, o.id, servitor
		}},
		{"pet", func(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32, *summon.Actor) {
			h := bootOwnerWithCollar(t)
			pet, _ := h.spawnWolf(t)
			return h.srv, h.client, h.ownerID, pet
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, client, ownerID, summoned := tc.setup(t)
			giveTransferPain(t, srv, ownerID)
			hitter := joinHitter(t, srv, client)
			drainUntilQuiet(t, client)
			summonBefore, ownerBefore := summoned.HP(), ownerVitals(t, srv, ownerID)

			hitOwner(t, srv, ownerID, hitter)

			if got := summoned.HP(); got != summonBefore {
				t.Fatalf("summon HP = %v after the owner's hit, want %v kept", got, summonBefore)
			}
			if got := ownerBefore - ownerVitals(t, srv, ownerID); got != 20 {
				t.Fatalf("owner lost %d CP+HP, want the whole 20", got)
			}
			want := fmt.Sprint([]string{fmt.Sprint(serverpackets.SystemMessageS1GaveYouS2Dmg, []any{"Hitter", int32(20)})})
			if got := damageMessages(transferMessages(t, drainFrames(t, client))); got != want {
				t.Fatalf("owner damage messages = %s, want %s", got, want)
			}
			if got := damageMessages(transferMessages(t, drainFrames(t, hitter.client))); got != "[]" {
				t.Fatalf("hitter damage messages = %s, want none", got)
			}
		})
	}
}
