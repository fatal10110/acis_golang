package network

import (
	"sort"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// raisedSkill accepts a skill level change that went up: the shortcut
// refresh of a grant that only ever raises skills.
func raisedSkill(old, level int) bool { return level > old }

// changedSkill accepts any change to a level still held: the shortcut
// refresh of a grant that sets a skill to a given level.
func changedSkill(old, level int) bool { return level > 0 && level != old }

// sendSkillChanges tells live's client about a change to its skills that
// started from before. Every SKILL shortcut of a skill whose level moved in
// a way refresh accepts is re-pointed at the new level (a ShortCutRegister
// each, by skill id), and only then does the new SkillList go out: a skill
// grant refreshes the shortcuts as it adds each skill and sends the list
// once at the end. A nil refresh leaves the shortcuts alone.
//
// It runs on live's queue.
func (l *GameClientLink) sendSkillChanges(live *livePlayer, before player.SkillLevels, refresh func(old, level int) bool) {
	if refresh != nil {
		after := live.SkillLevels()
		ids := make([]int, 0, len(after))
		for id, level := range after {
			if refresh(before[id], level) {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
		for _, id := range ids {
			l.refreshSkillShortcuts(live, int32(id), int32(after[id]))
		}
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
}
