package network

import (
	"context"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// characterEditStore stores the character fields admin commands change at
// once rather than with the character's next save.
type characterEditStore interface {
	SetNoble(ctx context.Context, objectID int32, noble bool) error
	SetTitle(ctx context.Context, objectID int32, title string) error
	SetClanPenalties(ctx context.Context, objectID int32, joinExpiry, createExpiry int64) error
}

// adminSet answers //set <field> [value] for gm's selection, gm itself
// without one. //set access is adminSetAccess; the other fields change a
// selected player and tell gm, and a selection that is no player answers
// nothing, as the reference does: a chat command leaves no client action
// pending. A missing or malformed value answers the field's usage.
//
// //set class, //set name and //set sex, and //set title on an NPC, are
// not ported yet (#3326): they log the gap and release the client.
func (l *GameClientLink) adminSet(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, "Usage: //set <access|class|color|exp|karma|level>")
		sendText(gm, "Usage: //set <name|noble|rec|sex|sp|tcolor|title>")
		return
	}
	field, values := args[0], args[1:]
	value, hasValue := "", len(values) > 0
	if hasValue {
		value = values[0]
	}
	if field == "access" {
		l.adminSetAccess(gm, values)
		return
	}
	target, isPlayer := adminSelectedPlayer(gm)
	edit := func(fn func()) {
		if isPlayer {
			onPlayer(gm, target, fn)
		}
	}
	switch field {
	case "class", "name", "sex":
		l.log.Warn().Str("field", field).Msg("admin: //set field not implemented yet (#3326)")
		gm.SendFrame(serverpackets.FrameActionFailed())
	case "color":
		edit(func() {
			color, err := commons.DecodeInt32("0x" + value)
			if !hasValue || err != nil {
				sendText(gm, "Usage: //set color <number>")
				return
			}
			target.SetColors(color, target.TitleColor())
			l.broadcastCharacterInfo(target)
			sendText(gm, "You successfully set color name of "+target.Name+".")
		})
	case "tcolor":
		edit(func() {
			// The color is read from the 17th character of the command on,
			// the space before the value included, which no hexadecimal
			// number parses: the reference answers the usage every time.
			color, ok := int32(0), false
			if len(line) >= len("admin_set tcolor") {
				var err error
				color, err = commons.DecodeInt32("0x" + line[len("admin_set tcolor"):])
				ok = err == nil
			}
			if !ok {
				sendText(gm, "Usage: //set tcolor <number>")
				return
			}
			target.SetColors(target.NameColor(), color)
			l.broadcastCharacterInfo(target)
			sendText(gm, "You successfully set title color name of "+target.Name+".")
		})
	case "exp":
		edit(func() {
			exp, err := commons.ParseInt(value, 64)
			if !hasValue || err != nil {
				sendText(gm, "Usage: //set exp <number>")
				return
			}
			l.setExp(target, exp)
			l.broadcastCharacterInfo(target)
			sendText(gm, "You successfully set "+target.Name+"'s XP to "+strconv.FormatInt(exp, 10)+".")
		})
	case "karma":
		edit(func() {
			karma, ok := parseJavaInt(value)
			if !ok {
				sendText(gm, "Usage: //set karma <number>")
				return
			}
			if karma < 0 {
				sendText(gm, "The karma value must be greater or equal to 0.")
				return
			}
			target.SetKarma(int(karma))
			sendText(gm, "You successfully set "+target.Name+"'s karma to "+strconv.Itoa(int(karma))+".")
		})
	case "level":
		edit(func() {
			level, ok := parseJavaInt(value)
			if !ok {
				sendText(gm, "Usage: //set level <number>")
				return
			}
			if l.levels == nil {
				sendText(gm, "Invalid used level for //set level.")
				return
			}
			row, ok := l.levels.Level(int(level))
			if !ok {
				sendText(gm, "Invalid used level for //set level.")
				return
			}
			l.setExp(target, row.RequiredExpToLevelUp)
			sendText(gm, "You successfully set "+target.Name+"'s level to "+strconv.Itoa(int(level))+".")
		})
	case "noble":
		edit(func() {
			l.toggleNoble(target)
			sendText(gm, "You have modified "+target.Name+"'s noble status.")
		})
	case "rec":
		edit(func() {
			rec, ok := parseJavaInt(value)
			if !ok {
				sendText(gm, "Usage: //set rec <number>")
				return
			}
			target.SetRecommendationsHave(int(rec))
			l.broadcastCharacterInfo(target)
			sendText(gm, "You successfully set "+target.Name+" to "+strconv.Itoa(int(rec))+".")
		})
	case "sp":
		edit(func() {
			sp, ok := parseJavaInt(value)
			if !ok {
				sendText(gm, "Usage: //set sp <number>")
				return
			}
			l.setSP(target, sp)
			l.broadcastCharacterInfo(target)
			sendText(gm, "You successfully set "+target.Name+"'s SP to "+strconv.Itoa(int(sp))+".")
		})
	case "title":
		l.adminSetTitle(gm, value, hasValue)
	default:
		sendText(gm, "Usage: //set access|class|color|exp|karma")
		sendText(gm, "Usage: //set level|name|rec|sex|sp|tcolor|title")
	}
}

// setExp brings live's experience to exp, on live's queue: a gain is
// earned and a loss taken away, each telling live as a reward or a loss
// does. The difference wraps as 64-bit arithmetic does, and a wrapped loss
// takes nothing away.
func (l *GameClientLink) setExp(live *livePlayer, exp int64) {
	switch cur := live.ProgressionValues().Exp; {
	case cur < exp:
		live.AddExpAndSp(exp-cur, 0)
	case cur > exp:
		live.RemoveExpAndSp(l.levels, live.Template(), cur-exp, 0)
	}
}

// setSP brings live's SP to sp as setExp brings its experience, the
// difference wrapping as 32-bit arithmetic does.
func (l *GameClientLink) setSP(live *livePlayer, sp int32) {
	switch cur := int32(live.ProgressionValues().SP); {
	case cur < sp:
		live.AddExpAndSp(0, int(sp-cur))
	case cur > sp:
		live.RemoveExpAndSp(l.levels, live.Template(), 0, int(cur-sp))
	}
}

// adminSetTitle answers //set title <title>: a selected player, gm itself
// without one, takes the title's first word, and it and the players around
// it see it.
func (l *GameClientLink) adminSetTitle(gm *livePlayer, title string, hasTitle bool) {
	if !hasTitle {
		sendText(gm, "Usage: //set title <title>")
		return
	}
	target, ok := adminSelectedPlayer(gm)
	if !ok {
		if gm.Target().Kind() == actor.KindNPC {
			l.log.Warn().Msg("admin: //set title on an NPC not implemented yet (#3326)")
			gm.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	onPlayer(gm, target, func() {
		title := trimTitle(title)
		target.SetTitle(title)
		l.broadcastTitleInfo(target)
		l.storeCharacterEdit(target.ObjectID(), "store title", func(ctx context.Context, s characterEditStore) error {
			return s.SetTitle(ctx, target.ObjectID(), title)
		})
		sendText(gm, "You successfully set your target's title to "+title+".")
	})
}

// broadcastTitleInfo shows live its new title in its UserInfo, then shows
// it and every player around it the title over its head.
func (l *GameClientLink) broadcastTitleInfo(live *livePlayer) {
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	id, title := live.ObjectID(), live.Title()
	l.broadcastLiveFrame(live, func() wire.Frame { return serverpackets.FrameTitleUpdate(id, title) })
}

// toggleNoble makes live a noble, or no longer one, on live's queue: the
// noble skills come or go with it, live gets its new skill list and
// UserInfo, and the status is stored at once.
func (l *GameClientLink) toggleNoble(live *livePlayer) {
	noble := !live.IsNoble()
	if noble {
		if l.skills != nil {
			if err := l.skills.GrantTransientSkills(live.Character, modelskill.NobleSkills()); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("admin: give noble skills")
			}
		}
	} else if l.skills != nil {
		for _, ref := range modelskill.NobleSkills() {
			l.removeLiveSkill(live, int(ref.ID), false)
		}
	}
	live.SetNoble(noble)
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	l.storeCharacterEdit(live.ObjectID(), "store noble", func(ctx context.Context, s characterEditStore) error {
		return s.SetNoble(ctx, live.ObjectID(), noble)
	})
}

// storeClanPenalties stores the clan penalties live serves now.
func (l *GameClientLink) storeClanPenalties(live *livePlayer) {
	join, create := live.ClanJoinExpiryTime(), live.ClanCreateExpiryTime()
	l.storeCharacterEdit(live.ObjectID(), "store clan penalties", func(ctx context.Context, s characterEditStore) error {
		return s.SetClanPenalties(ctx, live.ObjectID(), join, create)
	})
}

// storeCharacterEdit runs write on objectID's persistence lane, ahead of
// any save its logout queues after; what fails is logged under what.
func (l *GameClientLink) storeCharacterEdit(objectID int32, what string, write func(context.Context, characterEditStore) error) {
	if l.characterEdits == nil {
		return
	}
	store, log := l.characterEdits, l.log
	if !l.persist.Enqueue(objectID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), accessLevelWriteTimeout)
		defer cancel()
		if err := write(ctx, store); err != nil {
			log.Error().Err(err).Int32("object_id", objectID).Msg(what)
		}
	}) {
		log.Error().Int32("object_id", objectID).Msg(what + ": persistence closed")
	}
}
