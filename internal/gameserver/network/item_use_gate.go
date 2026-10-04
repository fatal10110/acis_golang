package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// useConditionsHold reports whether live meets every use condition of tmpl,
// without telling the client anything.
func useConditionsHold(live *livePlayer, tmpl *item.Template) bool {
	for _, uc := range tmpl.UseConditions {
		if !itemUseConditionHolds(live, uc.Root) {
			return false
		}
	}
	return true
}

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
			if !ok || live.Level() < level {
				return false
			}
		case "sex":
			sex, ok := parseConditionInt(raw)
			if !ok || int(live.Sex) != sex {
				return false
			}
		case "ishero":
			want := parseConditionBool(raw)
			if live.IsHero() != want {
				return false
			}
		case "pkcount":
			limit, ok := parseConditionInt(raw)
			if !ok || live.ProgressionValues().PKKills > limit {
				return false
			}
		case "flying":
			want := parseConditionBool(raw)
			if live.Flying() != want {
				return false
			}
		case "transformed":
			want := parseConditionBool(raw)
			if live.Transformed() != want {
				return false
			}
		case "resting":
			want := parseConditionBool(raw)
			if !live.Standing() != want {
				return false
			}
		case "running":
			want := parseConditionBool(raw)
			if live.Running() != want {
				return false
			}
		case "moving", "riding", "olympiad":
			if parseConditionBool(raw) {
				return false
			}
		case "castle", "clanhall", "pledgeclass":
			if !clanConditionHolds(live, name, raw) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// parseConditionBool reads a boolean condition attribute: true only for a
// case-insensitive "true", false for anything else, never an error.
func parseConditionBool(raw string) bool {
	return strings.EqualFold(raw, "true")
}

// clanConditionHolds judges a clan rank or residence attribute with the
// skill condition of the same name, reading the clan's live ownership.
// A value that does not parse fails.
func clanConditionHolds(live *livePlayer, name, raw string) bool {
	cond, err := conditions.Compile(modelskill.Condition{Kind: "player", Attrs: map[string]string{name: raw}})
	return err == nil && cond.Test(live.ConditionActor(), nil, nil)
}

func parseConditionInt(raw string) (int, bool) {
	v, err := commons.DecodeInt32(raw)
	return int(v), err == nil
}
