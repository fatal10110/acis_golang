package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// systemMessageNpcServerNotOperating is NPC_SERVER_NOT_OPERATING, which
// //unspawnall tells every player.
const systemMessageNpcServerNotOperating = 1278

// respawnAllTimeout bounds the spawn data save and load of //respawnall.
const respawnAllTimeout = 30 * time.Second

// adminUnspawnAll answers //unspawnall: every player is told the NPC server
// is not operating, every NPC leaves the world for good, and every game
// master online, hidden ones too, is told the unspawn is complete.
func (l *GameClientLink) adminUnspawnAll(_ *livePlayer, _ string) {
	l.spawnAll.Lock()
	defer l.spawnAll.Unlock()
	l.toAllPlayers(func() wire.Frame { return serverpackets.FrameSystemMessage(systemMessageNpcServerNotOperating) })
	if npcs := l.npcSpawns.Load(); npcs != nil {
		npcs.DespawnAll()
	}
	l.messageGMs("NPCs' unspawn is now complete.")
}

// adminRespawnAll answers //respawnall: every NPC leaves the world as for
// //unspawnall, with nothing told to the players; the NPC templates and the
// spawn list are read again; every on-start maker spawns anew; and every
// game master online, hidden ones too, is told the respawn is complete. The
// standalone spawns are gone afterwards. When the templates or the spawn
// list cannot be read, the failure is logged, the NPCs stay despawned and
// no game master is told anything, as the reference's exception does. One
// //respawnall or //unspawnall runs at a time: a second one waits for the
// first to finish, so it despawns what the first placed.
func (l *GameClientLink) adminRespawnAll(_ *livePlayer, _ string) {
	l.spawnAll.Lock()
	defer l.spawnAll.Unlock()
	npcs := l.npcSpawns.Load()
	if npcs != nil {
		npcs.DespawnAll()
	}
	if l.reloads.NPCs != nil {
		if err := l.reloads.NPCs(); err != nil {
			l.log.Error().Err(err).Msg("admin: //respawnall could not reload the npc templates")
			return
		}
	}
	if npcs != nil {
		spawns := npcs.Spawns()
		if l.reloads.SpawnList != nil {
			ctx, cancel := context.WithTimeout(context.Background(), respawnAllTimeout)
			defer cancel()
			fresh, err := l.reloads.SpawnList(ctx, spawns)
			if err != nil {
				l.log.Error().Err(err).Msg("admin: //respawnall could not reload the spawn list")
				return
			}
			spawns = fresh
		}
		npcs.RespawnAll(spawns)
	}
	l.messageGMs("NPCs' respawn is now complete.")
}

// messageGMs tells every game master online, hidden ones too, text:
// AdminData.broadcastMessageToGMs.
func (l *GameClientLink) messageGMs(text string) {
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, text)
	}, func(send func(frameReceiver)) {
		for _, entry := range l.gms.Entries(true) {
			send(entry.Player)
		}
	})
}
