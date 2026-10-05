package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// babyDuckRodID is the datapack's Baby Duck Rod (weapon_type FISHINGROD).
	babyDuckRodID int32 = 6529
	// longBowID is the datapack's Long Bow (weapon_type BOW, mp_consume 4).
	longBowID int32 = 275
)

const longBowMPConsume = 4

// weaponRefusalCatalog is the shared catalog plus a fishing rod and a bow
// with the datapack's MP cost.
func weaponRefusalCatalog() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(),
		&item.Template{
			ID: babyDuckRodID, Name: "Baby Duck Rod", Kind: item.KindWeapon, Slot: item.SlotLRHand,
			Duration: -1, Destroyable: true, DefaultAction: item.ActionEquip,
			Weapon: &item.WeaponDetail{Type: item.WeaponFishingRod},
		},
		&item.Template{
			ID: longBowID, Name: "Long Bow", Kind: item.KindWeapon, Slot: item.SlotLRHand,
			Duration: -1, Destroyable: true, DefaultAction: item.ActionEquip,
			Weapon: &item.WeaponDetail{Type: item.WeaponBow, MPConsume: longBowMPConsume, ReuseDelay: 1500},
		},
	))
}

// TestAttackRefusedByWeaponSendsSystemMessage drives the real attack request
// against a monster in reach: PlayerAI.thinkAttack reaches
// PlayerAttack.canAttack, which answers a fishing rod with
// CANNOT_ATTACK_WITH_FISHING_POLE (1472), a bow with no arrows with
// NOT_ENOUGH_ARROWS (112), and a bow with arrows but less MP than its cost
// with NOT_ENOUGH_MP (24), each once and ahead of the think's ActionFailed.
// No swing starts.
func TestAttackRefusedByWeaponSendsSystemMessage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		weapon  int32
		arrows  bool
		drainMP bool
		wantMsg int
	}{
		{"fishing rod", babyDuckRodID, false, false, serverpackets.SystemMessageCannotAttackWithFishingPole},
		{"bow without arrows", longBowID, false, false, serverpackets.SystemMessageNotEnoughArrows},
		{"bow without MP", longBowID, true, true, serverpackets.SystemMessageNotEnoughMP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithItemTemplates(weaponRefusalCatalog()),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			weapon := srv.GiveItem(t, objID, tc.weapon, 1)
			var arrows int32
			if tc.arrows {
				arrows = srv.GiveItem(t, objID, woodenArrowTemplateID, bowArrowStack)
			}
			startInWorld(t, c)
			equipAndFlush(t, srv, c, weapon)
			if tc.arrows {
				equipAndFlush(t, srv, c, arrows)
			}

			hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)

			c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
			assertFrameOpcode(t, mustRead(t, c, "select ValidateLocation"), serverpackets.OpcodeValidateLocation, "select ValidateLocation")
			assertFrameOpcode(t, mustRead(t, c, "MyTargetSelected"), serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
			assertFrameOpcode(t, mustRead(t, c, "selection StatusUpdate"), serverpackets.OpcodeStatusUpdate, "selection StatusUpdate")
			drainUntilQuiet(t, c)

			if tc.drainMP {
				live := mustLiveBowPlayer(t, srv, objID)
				drainer, ok := live.(interface{ ReduceCurrentMP(int) })
				if !ok {
					t.Fatalf("live player %T cannot lose MP", live)
				}
				drainer.ReduceCurrentMP(live.CurrentMP())
				if got := live.CurrentMP(); got >= longBowMPConsume {
					t.Fatalf("CurrentMP() = %d after draining, want below %d", got, longBowMPConsume)
				}
			}

			c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
			var messages []int32
			for {
				frame := mustRead(t, c, "refused attack ActionFailed")
				switch frame[0] {
				case serverpackets.OpcodeAttack:
					t.Fatal("refused attack swung")
				case serverpackets.OpcodeSystemMessage:
					messages = append(messages, wire.NewReader(frame[1:]).ReadInt32())
					assertStaticBowSystemMessage(t, frame, int(messages[len(messages)-1]))
					continue
				case serverpackets.OpcodeActionFailed:
				default:
					continue
				}
				break
			}
			if len(messages) != 1 || messages[0] != int32(tc.wantMsg) {
				t.Fatalf("SystemMessages before ActionFailed = %v, want exactly [%d]", messages, tc.wantMsg)
			}

			for range 10 {
				frame := c.ReadWithTimeout(readQuietWindow)
				if frame == nil {
					break
				}
				if frame[0] == serverpackets.OpcodeAttack {
					t.Fatal("refused attack swung after ActionFailed")
				}
				if frame[0] == serverpackets.OpcodeSystemMessage {
					t.Fatalf("SystemMessage %d after the refused think, want one per think", wire.NewReader(frame[1:]).ReadInt32())
				}
			}
		})
	}
}
