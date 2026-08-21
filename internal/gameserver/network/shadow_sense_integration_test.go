//go:build integration

package network

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkEnterWorldSendsShadowSenseState(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		tmpl, ok := testTemplates(t).Get(0)
		if !ok {
			t.Fatal("missing test class template")
		}
		character, err := player.NewCharacter(100, tmpl, "player1", "Newbie", 1, 0, 0, player.SexMale)
		if err != nil {
			t.Fatalf("new character: %v", err)
		}
		character.Race = player.RaceDarkElf
		if err := chars.Create(context.Background(), character); err != nil {
			t.Fatalf("seed character: %v", err)
		}
	}, 1)

	objectID := sqlSoleObjectID(t, chars)
	if _, err := sqltest.SharedDB(t).ExecContext(context.Background(), "INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES (?, ?, ?, ?)", objectID, 294, 1, 0); err != nil {
		t.Fatalf("seed Shadow Sense: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())

	for range 14 {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			break
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(frame[1:])
		messageID := r.ReadInt32()
		if messageID != serverpackets.SystemMessageNightSkillEffectApplies && messageID != serverpackets.SystemMessageDaySkillEffectDisappears {
			continue
		}
		if params := r.ReadInt32(); params != 1 {
			t.Fatalf("Shadow Sense message params = %d, want 1", params)
		}
		if kind := r.ReadInt32(); kind != serverpackets.SystemMessageParamSkillName {
			t.Fatalf("Shadow Sense message parameter type = %d, want skill name", kind)
		}
		if skillID, level := r.ReadInt32(), r.ReadInt32(); skillID != 294 || level != 1 {
			t.Fatalf("Shadow Sense message skill = %d level %d, want 294 level 1", skillID, level)
		}
		return
	}
	actor, _ := state.Player(objectID)
	if actor == nil {
		t.Fatal("entered character missing from world")
	}
	character := actor.(*livePlayer).Character
	t.Fatalf("Shadow Sense state message missing (race=%v skill level=%d)", character.Race, character.SkillLevel(294))
}
