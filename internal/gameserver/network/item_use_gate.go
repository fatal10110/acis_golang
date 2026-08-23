package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func rejectUseItemConditions(live *livePlayer, tmpl *item.Template) bool {
	if live == nil || tmpl == nil || len(tmpl.UseConditions) == 0 {
		return false
	}
	for _, uc := range tmpl.UseConditions {
		if itemUseConditionHolds(live, uc.Root) {
			continue
		}
		sendUseConditionFailure(live, tmpl, uc)
		return true
	}
	return false
}

func sendUseConditionFailure(live *livePlayer, tmpl *item.Template, uc item.UseCondition) {
	switch {
	case uc.Message != "":
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, uc.Message))
	case uc.MessageID > 0 && uc.AddName:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(int(uc.MessageID), tmpl.ID))
	case uc.MessageID > 0:
		live.SendFrame(serverpackets.FrameSystemMessage(int(uc.MessageID)))
	default:
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}

func itemUseConditionHolds(live *livePlayer, cond item.Condition) bool {
	return item.EvaluateCondition(cond, func(leaf item.Condition) bool {
		if strings.ToLower(leaf.Kind) != "player" {
			return false
		}
		return playerUseConditionHolds(live, leaf.Attrs)
	})
}

func playerUseConditionHolds(live *livePlayer, attrs map[string]string) bool {
	for name, raw := range attrs {
		switch strings.ToLower(name) {
		case "level":
			level, ok := parseConditionInt(raw)
			if !ok || live.CharLevel < level {
				return false
			}
		case "sex":
			sex, ok := parseConditionInt(raw)
			if !ok || int(live.Sex) != sex {
				return false
			}
		case "ishero":
			want, ok := parseConditionBool(raw)
			if !ok || live.IsHero() != want {
				return false
			}
		case "pkcount":
			limit, ok := parseConditionInt(raw)
			if !ok || live.PKKills > limit {
				return false
			}
		case "flying":
			want, ok := parseConditionBool(raw)
			if !ok || live.Flying() != want {
				return false
			}
		case "transformed":
			want, ok := parseConditionBool(raw)
			if !ok || live.Transformed() != want {
				return false
			}
		case "resting":
			want, ok := parseConditionBool(raw)
			if !ok || !live.Standing() != want {
				return false
			}
		case "running":
			want, ok := parseConditionBool(raw)
			if !ok || live.Running() != want {
				return false
			}
		case "moving", "riding", "olympiad":
			want, ok := parseConditionBool(raw)
			if !ok || want {
				return false
			}
		case "castle", "clanhall":
			id, ok := parseConditionInt(raw)
			if !ok || id != 0 {
				return false
			}
		case "pledgeclass":
			return false
		default:
			return false
		}
	}
	return true
}

func parseConditionBool(raw string) (bool, bool) {
	v, err := strconv.ParseBool(raw)
	return v, err == nil
}

func parseConditionInt(raw string) (int, bool) {
	v, err := strconv.Atoi(raw)
	return v, err == nil
}
