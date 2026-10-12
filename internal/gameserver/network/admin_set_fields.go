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
			sendText(gm, "You tried to set an invalid class for "+target.CharacterName()+".")
			return
		}
		className := player.ClassName(classID)
		if target.ClassID() == classID {
			sendText(gm, target.CharacterName()+" is already a(n) "+className+".")
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
			sendText(gm, "You successfully set "+target.CharacterName()+" class to "+className+".")
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
			sendText(gm, target.CharacterName()+"'s sex is already defined as "+name+".")
			return
		}
		l.adminRespawn(target, func() { target.SetSex(sex) }, func() {
			l.storeCharacterEdit(target.ObjectID(), "store sex", func(ctx context.Context, s characterEditStore) error {
				return s.SetSex(ctx, target.ObjectID(), byte(sex))
			})
		}, func() {
			sendText(gm, "You successfully set "+target.CharacterName()+" gender to "+name+".")
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

// adminSetName answers //set name <name>: a selected player is renamed,
// see adminRenamePlayer; a selected NPC takes the name and the players
// around it see it; anything else is the wrong target.
func (l *GameClientLink) adminSetName(gm *livePlayer, name string, hasName bool) {
	if !hasName {
		sendText(gm, "Usage: //set name <name>")
		return
	}
	if target, isPlayer := adminSelectedPlayer(gm); isPlayer {
		l.adminRenamePlayer(gm, target, name)
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

// adminRenamePlayer renames target to name for gm. A name that is not 1
// to 16 ASCII letters or digits, an NPC's name or a character's, online
// or not, is refused with INCORRECT_NAME_TRY_AGAIN. Otherwise, on target's
// queue, target takes the name, is found by it from then on, shows it to
// itself and the players around it, and is stored before gm is told.
func (l *GameClientLink) adminRenamePlayer(gm, target *livePlayer, name string) {
	if !shortNamePattern.MatchString(name) || l.npcNamed(name) || l.characterNamed(name) {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageIncorrectNameTryAgain))
		return
	}
	onPlayer(gm, target, func() {
		if !l.renamePlayer(target, name) {
			// A player that took the name online since the check.
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageIncorrectNameTryAgain))
			return
		}
		l.broadcastCharacterInfo(target)
		l.storeCharacterEdit(target.ObjectID(), "store name", func(ctx context.Context, s characterEditStore) error {
			return s.SetName(ctx, target.ObjectID(), name)
		})
		l.storeLive(target)
		sendText(gm, "You successfully set your target's name to "+target.CharacterName()+".")
	})
}

// npcNamed reports whether an NPC template is named name, matched
// case-insensitively.
func (l *GameClientLink) npcNamed(name string) bool {
	if l.npcs == nil {
		return false
	}
	_, ok := l.npcs.GetByName(name)
	return ok
}

// characterNamed reports whether a character is named name. A lookup that
// cannot answer is logged and counts as named, so the rename is refused.
func (l *GameClientLink) characterNamed(name string) bool {
	if l.characterEdits == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), characterLookupTimeout)
	defer cancel()
	taken, err := l.characterEdits.NameTaken(ctx, name)
	if err != nil {
		l.log.Error().Err(err).Str("name", name).Msg("admin: //set name: check name")
		return true
	}
	return taken
}

// renamePlayer names live name, on its queue, together with its name
// index entry and its clan roster row. It refuses a name another online
// player holds.
func (l *GameClientLink) renamePlayer(live *livePlayer, name string) bool {
	rename := func() { live.SetName(name) }
	if l.world == nil {
		rename()
	} else if !l.world.RenamePlayer(live.ObjectID(), name, rename) {
		return false
	}
	if cl, ok := l.clanService().ClanOf(live.Character); ok {
		cl.RefreshName(live.ObjectID(), name)
	}
	return true
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
	broadcastFrame(func() wire.Frame {
		s := info()
		l.addNPCCrest(obj, &s)
		return serverpackets.FrameNPCInfo(s)
	}, func(send func(frameReceiver)) {
		l.world.ForEachKnown(obj, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}
