package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	specialTreeNPCID = 13007
	// treeAuraSkill is Special Tree Recovery Bonus, the special tree's aura.
	treeAuraSkill = 2139
)

// TestSpecialChristmasTreeAura places a Special Christmas Tree with its kit
// (ChristmasTree: npc 13007 only). Every 3 s, from 3 s after it is placed,
// each player within 200 of it not affected by Special Tree Recovery Bonus
// lands that buff on itself: its owner, standing on it, gets the buff's
// icon on the first pulse and not before, and is not given it again while
// it lasts; a player 400 away gets nothing. Once the tree is gone the aura
// stops: the expired buff is not landed again.
func TestSpecialChristmasTreeAura(t *testing.T) {
	t.Parallel()
	h, far, farClient := bootSpecialTree(t)

	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	placed := drainFrames(t, h.client)
	if len(placed) == 0 || placed[0][0] != serverpackets.OpcodeNPCInfo {
		t.Fatal("placing the special tree did not show its NpcInfo first")
	}
	treeID := wire.NewReader(placed[0][1:]).ReadInt32()
	drainUntilQuiet(t, farClient)
	owner := h.character(t)

	if h.srv.DrivesClock() {
		h.srv.Advance(t, 3*time.Second-time.Millisecond)
		if _, ok := owner.EffectList().ActiveBySkillID(treeAuraSkill); ok {
			t.Fatal("owner affected by the aura before its first pulse")
		}
		h.srv.Advance(t, time.Millisecond)
	} else {
		h.srv.AdvanceUntil(t, "aura landed on the owner", func() bool {
			_, ok := owner.EffectList().ActiveBySkillID(treeAuraSkill)
			return ok
		})
	}
	if level, ok := owner.EffectList().ActiveBySkillID(treeAuraSkill); !ok || level != 1 {
		t.Fatalf("owner aura effect = level %d (%v), want level 1", level, ok)
	}
	if !auraIconShown(t, drainFrames(t, h.client)) {
		t.Fatal("owner shown no aura icon on the first pulse")
	}
	if _, ok := far.EffectList().ActiveBySkillID(treeAuraSkill); ok {
		t.Fatal("player 400 away affected by the aura")
	}
	if auraIconShown(t, drainFrames(t, farClient)) {
		t.Fatal("player 400 away shown the aura icon")
	}
	if !h.srv.DrivesClock() {
		return
	}

	h.srv.Advance(t, 3*time.Second)
	for _, f := range drainFrames(t, h.client) {
		if f[0] == serverpackets.OpcodeAbnormalStatusUpdate {
			t.Fatal("aura landed again on an owner it already affects")
		}
	}

	obj, ok := h.srv.State.Object(treeID)
	if !ok {
		t.Fatal("special tree not in the world")
	}
	obj.(*npc.Decoration).Despawn()
	h.srv.AdvanceWithEffects(t, 12*time.Second)
	if _, ok := owner.EffectList().ActiveBySkillID(treeAuraSkill); ok {
		t.Fatal("aura landed again after the tree was removed")
	}
}

// bootSpecialTree brings the owner in with a kit placing a Special
// Christmas Tree, and a second player teleported 400 away along x, returning the
// owner's world, the second player's character and its client.
func bootSpecialTree(t *testing.T) (*petWorld, *player.Character, *testsupport.ScriptedClient) {
	t.Helper()
	special := treeTemplate()
	special.ID, special.TemplateID, special.Name = specialTreeNPCID, specialTreeNPCID, "Special Tree"
	items, err := item.NewSummonItemTable([]item.SummonItem{
		{ItemID: wolfCollarID, NPCID: wolfNPCID, SummonType: 1},
		{ItemID: treeKitID, NPCID: specialTreeNPCID, SummonType: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: treeAuraSkill, Level: 1, Activation: modelskill.ActivationActive, SkillType: "BUFF", Magic: true,
		Effects: []modelskill.EffectTemplate{{
			Name: "Buff", Time: 10, Icon: true, StackType: "hp_regen_up", StackOrder: 99,
			Funcs: []modelskill.FuncTemplate{
				{Op: modelskill.FuncMul, Stat: "regHp", Value: 1.4},
				{Op: modelskill.FuncMul, Stat: "regMp", Value: 1.2},
			},
		}},
	}}), gamesql.NewCharacterSkillStore(db))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), special})),
		gameservertest.WithSummonItems(items),
		gameservertest.WithSkills(skills),
	}, seedItem{TemplateID: treeKitID, Count: 1})

	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	id := h.srv.SeedCharacterFor(t, "player2", "Far", 40, 0).ID
	c := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, c)
	obj, ok := h.srv.State.Player(id)
	if !ok {
		t.Fatal("Far not in world")
	}
	far, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world player %T is not an online character", obj)
	}
	far.TeleportTo(x+400, y, z, 0)
	readUntilOpcode(t, c, serverpackets.OpcodeTeleportToLocation, "Far teleport")
	drainUntilQuiet(t, c)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, c)
	return h, far, c
}

// auraIconShown reports whether an AbnormalStatusUpdate among frames shows
// the aura's level 1 icon.
func auraIconShown(t *testing.T, frames [][]byte) bool {
	t.Helper()
	shown := false
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeAbnormalStatusUpdate {
			continue
		}
		r := wire.NewReader(f[1:])
		for range r.ReadUint16() {
			if id, level, _ := r.ReadInt32(), r.ReadUint16(), r.ReadInt32(); id == treeAuraSkill && level == 1 {
				shown = true
			}
		}
		if err := r.Err(); err != nil {
			t.Fatalf("read AbnormalStatusUpdate: %v", err)
		}
	}
	return shown
}
