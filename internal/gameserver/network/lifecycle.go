package network

import (
	"context"
	"time"
)

const livePlayerDetachSaveTimeout = 2 * time.Second

func (l *GameClientLink) detachLivePlayer(ctx context.Context, live *livePlayer) {
	if live == nil {
		return
	}
	l.cancelActiveTrade(live)
	if l.roster != nil || l.skills != nil {
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), livePlayerDetachSaveTimeout)
		defer cancel()
		if l.roster != nil {
			if err := l.roster.SavePosition(saveCtx, live.Character); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player position")
			}
		}
		if l.skills != nil {
			if err := l.skills.Save(saveCtx, live.Character, true); err != nil {
				l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("save player skill state")
			}
		}
	}
	if l.world != nil {
		l.world.Despawn(live)
		l.world.RemovePlayer(live.ObjectID())
	}
	live.Character.SetFrameSender(nil)
	live.Character.SetAttackBroadcaster(nil)
}

func (l *GameClientLink) notifyPlayerLogout(account string) {
	loginLink := l.loginLink()
	if account == "" || loginLink == nil {
		return
	}
	if err := loginLink.SendPlayerLogout(account); err != nil {
		l.log.Debug().Err(err).Str("account", account).Msg("notify player logout")
	}
}
