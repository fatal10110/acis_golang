package network

import (
	"context"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// adminRespawnDelay is how long //set class and //set sex keep their player
// off the grid before it comes back changed.
const adminRespawnDelay = 4 * time.Second

// adminSetClass answers //set class <id> for target: a value that is no
// number, or the id one past the last class, opens the class list; an id
// outside the classes answers nothing; a reserved id or target's own class
// is refused; any other class is taken through adminRespawn.
func (l *GameClientLink) adminSetClass(gm, target *livePlayer, value string, hasValue bool) {
	id, err := commons.ParseInt(value, 32)
	if !hasValue || err != nil || id == player.MaxClassID+1 {
		l.sendAdminFile(gm, "charclasses.htm")
		return
	}
	if id < 0 || id > player.MaxClassID {
		return
	}
	classID := int(id)
	onPlayer(gm, target, func() {
		if _, ok := player.ClassLevel(classID); !ok {
			sendText(gm, "You tried to set an invalid class for "+target.Name+".")
			return
		}
		className := player.ClassName(classID)
		if target.ClassID() == classID {
			sendText(gm, target.Name+" is already a(n) "+className+".")
			return
		}
		var tmpl *player.Template
		if l.templates != nil {
			tmpl, _ = l.templates.Get(classID)
		}
		if tmpl == nil {
			l.log.Error().Int32("object_id", target.ObjectID()).Int("class_id", classID).Msg("admin: //set class: no template loaded")
			return
		}
		l.adminRespawn(target, func() {
			if l.changeOccupation(target, classID, tmpl) && !target.SubclassActive() {
				target.SetBaseClass(classID, tmpl)
			}
		}, func() {
			l.storeLive(target)
			target.RefreshWeightPenalty()
			target.RefreshHennaStats()
			target.SendFrame(serverpackets.FrameHennaInfo(target.HennaSnapshot()))
		}, func() {
			sendText(gm, "You successfully set "+target.Name+" class to "+className+".")
		})
	})
}

// adminSetSex answers //set sex <male|female|etc> for target, the value
// read case-insensitively: target's own sex is refused, any other is taken
// through adminRespawn and stored.
func (l *GameClientLink) adminSetSex(gm, target *livePlayer, value string, hasValue bool) {
	sex, ok := parseAdminSex(value)
	if !hasValue || !ok {
		sendText(gm, "Usage: //set sex <sex>")
		return
	}
	name := strings.ToUpper(sex.String())
	onPlayer(gm, target, func() {
		if target.Sex() == sex {
			sendText(gm, target.Name+"'s sex is already defined as "+name+".")
			return
		}
		l.adminRespawn(target, func() { target.SetSex(sex) }, func() {
			l.storeCharacterEdit(target.ObjectID(), "store sex", func(ctx context.Context, s characterEditStore) error {
				return s.SetSex(ctx, target.ObjectID(), byte(sex))
			})
		}, func() {
			sendText(gm, "You successfully set "+target.Name+" gender to "+name+".")
		})
	})
}

// parseAdminSex reads a sex by its upper-cased name.
func parseAdminSex(value string) (player.Sex, bool) {
	switch strings.ToUpper(value) {
	case "MALE":
		return player.SexMale, true
	case "FEMALE":
		return player.SexFemale, true
	case "ETC":
		return player.SexEtc, true
	}
	return 0, false
}

// adminRespawn takes live, on its queue, off the grid in place drawn held,
// as the reference's decay: it stops what it does, forgets itself as its
// selection and is told it is gone. After adminRespawnDelay, on its queue
// again, change runs, live comes back on the grid, settle runs, live and
// the players around it see it no longer held, its client is released, and
// done runs. A live that left the world meanwhile drops it all with its
// queue.
func (l *GameClientLink) adminRespawn(live *livePlayer, change, settle, done func()) {
	live.abortAll(false)
	live.StartAbnormalEffect(modelskill.AbnormalHold2)
	live.BroadcastAbnormalEffect()
	if live.Target() == world.Tracked(live) {
		live.forgetTarget(live)
	}
	live.SendFrame(serverpackets.FrameDeleteObject(live.ObjectID(), live.seated()))
	l.leaveGrid(live)
	live.after(adminRespawnDelay, func() {
		if live.detached() {
			return
		}
		change()
		l.rejoinGrid(live)
		settle()
		l.broadcastCharacterInfo(live)
		live.StopAbnormalEffect(modelskill.AbnormalHold2)
		live.BroadcastAbnormalEffect()
		live.SendFrame(serverpackets.FrameActionFailed())
		done()
	})
}

// storeLive saves live's character row, position, subclasses and skill state at once,
// on its persistence lane, as an autosave does.
func (l *GameClientLink) storeLive(live *livePlayer) {
	if l.roster == nil {
		return
	}
	charState := live.saveState()
	var skillState skillstate.SaveState
	if l.skills != nil {
		skillState = l.skills.SaveState(live.Character)
	}
	roster, skills, log := l.roster, l.skills, l.log
	if !l.persist.Enqueue(live.ObjectID(), func() {
		ctx, cancel := context.WithTimeout(context.Background(), autosaveSaveTimeout)
		defer cancel()
		if err := roster.Save(ctx, charState); err != nil {
			log.Error().Err(err).Int32("object_id", charState.ID).Msg("admin: store player")
		}
		if err := roster.SavePosition(ctx, charState); err != nil {
			log.Error().Err(err).Int32("object_id", charState.ID).Msg("admin: store player position")
		}
		if skills == nil {
			return
		}
		if err := skills.Save(ctx, skillState); err != nil {
			log.Error().Err(err).Int32("object_id", charState.ID).Msg("admin: store skill state")
		}
	}) {
		log.Error().Int32("object_id", live.ObjectID()).Msg("admin: store player: persistence closed")
	}
}

// adminSetName answers //set name <name>: a selected NPC takes the name and
// the players around it see it; anything else is the wrong target. Renaming
// a player is not ported yet (#3399): it logs the gap and releases the
// client.
func (l *GameClientLink) adminSetName(gm *livePlayer, name string, hasName bool) {
	if !hasName {
		sendText(gm, "Usage: //set name <name>")
		return
	}
	if _, isPlayer := adminSelectedPlayer(gm); isPlayer {
		l.log.Warn().Msg("admin: //set name on a player not implemented yet (#3399)")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	selected := gm.Target()
	inst, info, ok := adminNPC(selected)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	inst.SetName(name)
	l.broadcastNPCInfo(selected, info)
	sendText(gm, "You successfully set your target's name to "+inst.Name()+".")
}

// adminNPC resolves obj as an NPC: its instance, whose name and title an
// admin may change, and its NpcInfo view.
func adminNPC(obj world.Tracked) (*npc.Instance, func() serverpackets.NPCInfoSnapshot, bool) {
	switch o := obj.(type) {
	case *npc.Folk:
		return o.Instance, o.NPCInfoSnapshot, true
	case *npc.Hostile:
		return o.Instance, o.NPCInfoSnapshot, true
	case *npc.Decoration:
		return o.Instance, o.NPCInfoSnapshot, true
	}
	return nil, nil, false
}

// broadcastNPCInfo shows every player that knows obj its NpcInfo again.
func (l *GameClientLink) broadcastNPCInfo(obj world.Tracked, info func() serverpackets.NPCInfoSnapshot) {
	if l.world == nil {
		return
	}
	broadcastFrame(func() wire.Frame { return serverpackets.FrameNPCInfo(info()) }, func(send func(frameReceiver)) {
		l.world.ForEachKnown(obj, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}
