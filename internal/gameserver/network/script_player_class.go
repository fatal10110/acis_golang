package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// applyScriptClassEvent runs the class operations a script asked of live,
// on the script's goroutine before the script's call returns:
//
//   - ClassChangeRequested: the occupation change every class change makes
//     (changeOccupation), as Player.setClassId does;
//   - BaseClassChangeRequested: the base class, and the base body with it,
//     as Player.setBaseClass does;
//   - UserInfoBroadcast: UserInfo to live and CharInfo around it, as
//     Player.broadcastUserInfo does.
//
// A class with no template loaded changes nothing and is logged: the
// reference loads every class's template.
func (l *GameClientLink) applyScriptClassEvent(live *livePlayer, ev event.Event) {
	switch e := ev.(type) {
	case event.ClassChangeRequested:
		if tmpl, ok := l.scriptClassTemplate(live, e.ClassID); ok {
			l.changeOccupation(live, e.ClassID, tmpl)
		}
	case event.BaseClassChangeRequested:
		if tmpl, ok := l.scriptClassTemplate(live, e.ClassID); ok {
			live.SetBaseClass(e.ClassID, tmpl)
		}
	case event.UserInfoBroadcast:
		l.broadcastCharacterInfo(live)
	}
}

// scriptClassTemplate returns the template of classID, logging its
// absence.
func (l *GameClientLink) scriptClassTemplate(live *livePlayer, classID int) (*player.Template, bool) {
	if l.templates != nil {
		if tmpl, ok := l.templates.Get(classID); ok {
			return tmpl, true
		}
	}
	l.log.Error().Int32("object_id", live.ObjectID()).Int("class_id", classID).Msg("script class change: no template loaded")
	return nil, false
}
