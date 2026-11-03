package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// oraclePolymorphCases are the polymorphed views (testdata/oracle/
// ProbePolymorph.java): the NpcInfo other players get for a polymorphed
// character, the UserInfo it gets itself, and the NpcInfo of a polymorphed
// npc, over the player and npc of the same row as oracleCharacterCases.
func oraclePolymorphCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		morphed := func() (*oraclePlayer, player.NPCForm) {
			o, form := newOraclePlayer(r, v, false), oracleForm(r, v)
			o.c.SetPolymorph(&form)
			return o, form
		}
		m["PcMorphInfo"+n] = func() wire.Frame {
			o, form := morphed()
			return FramePcMorphInfo(o.c, form)
		}
		m["UserInfo/polymorph"+n] = func() wire.Frame {
			o, _ := morphed()
			return FrameUserInfo(o.user())
		}
		// A polymorphed npc shows the form's template id, held items and
		// body; its name and title stay its own.
		m["NpcInfo/polymorph"+n] = func() wire.Frame {
			s, form := oracleNpc(r, v), oracleForm(r, v)
			s.TemplateID = int(rowField(v, 8))
			s.RightHand, s.LeftHand = int(form.RightHand), int(form.LeftHand)
			s.CollisionRadius, s.CollisionHeight = form.CollisionRadius, form.CollisionHeight
			return FrameNPCInfo(s)
		}
	}
	m["PcMorphInfo/live"] = func() wire.Frame {
		a := oracleRows[0].v
		o, form := newOraclePlayer(0, a, true), oracleForm(0, a)
		o.c.SetRunning(true)
		o.c.SetInCombat(true)
		o.c.MarkDead()
		o.c.StartAbnormalEffect(0x2040)
		return FramePcMorphInfo(o.c, form)
	}
	return m
}

// oracleForm is ProbePolymorph.form: the npc form of row r, every value
// apart from the row's own.
func oracleForm(r int, v [10]int32) player.NPCForm {
	return player.NPCForm{
		NPCID:           rowField(v, 7),
		Name:            oracleStrings[(r+2)%len(oracleStrings)].s,
		RightHand:       rowField(v, 9) ^ rowField(v, 0),
		LeftHand:        rowField(v, 1) ^ rowField(v, 2),
		CollisionRadius: rowDouble(r, 4), CollisionHeight: rowDouble(r, 5),
	}
}
