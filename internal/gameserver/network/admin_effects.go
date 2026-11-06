package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// superHasteSkillID is the toggle //gmspeed puts on the game master, at
// levels 1 to 4.
const superHasteSkillID = 7029

// adminJukebox answers //jukebox with the first page of the sound list.
func (l *GameClientLink) adminJukebox(gm *livePlayer, _ string) {
	l.sendAdminFile(gm, "songs/songs.htm")
}

// adminUndying answers //undying: the game master, never its target,
// turns immortal when mortal and back, and is told which it now is.
func (l *GameClientLink) adminUndying(gm *livePlayer, _ string) {
	gm.SetMortal(!gm.Mortal())
	if gm.Mortal() {
		sendText(gm, "You are now mortal.")
		return
	}
	sendText(gm, "You are now immortal.")
}

// abnormalHolder is a creature showing a visual abnormal state.
type abnormalHolder interface {
	AbnormalEffect() int
	StartAbnormalEffect(mask int)
	StopAbnormalEffect(mask int)
	UpdateAbnormalEffect()
}

// adminAbnormal answers //abnormal <hex mask> on the selected creature, the
// game master without one: a creature already showing every bit of the
// mask stops showing them, any other starts, and is shown again. A missing
// or malformed mask answers the usage. A door shows no abnormal state, so
// it is left as it is.
func (l *GameClientLink) adminAbnormal(gm *livePlayer, line string) {
	target := adminTargetCreature(gm)
	args := handleradmin.Args(line)
	mask, ok := 0, len(args) > 0
	if ok {
		mask, ok = parseAbnormalMask(args[0])
	}
	if !ok {
		sendText(gm, "Usage: //abnormal mask")
		return
	}
	c, ok := target.(abnormalHolder)
	if !ok {
		return
	}
	onCreature(gm, target, func() {
		if c.AbnormalEffect()&mask == mask {
			c.StopAbnormalEffect(mask)
		} else {
			c.StartAbnormalEffect(mask)
		}
		c.UpdateAbnormalEffect()
	})
}

// parseAbnormalMask parses tok as hexadecimal digits, without sign or
// prefix, in the non-negative int range.
func parseAbnormalMask(tok string) (int, bool) {
	if tok == "" || tok[0] == '+' || tok[0] == '-' {
		return 0, false
	}
	v, err := strconv.ParseInt(tok, 16, 32)
	if err != nil {
		return 0, false
	}
	return int(v), true
}

// adminEarthquake answers //earthquake <intensity> <duration>: every player
// in the world, the game master included, feels an earthquake centred on
// the game master. A missing or malformed number answers the usage.
func (l *GameClientLink) adminEarthquake(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	var intensity, duration int32
	ok := len(args) >= 2
	if ok {
		intensity, ok = parseInt32(args[0])
	}
	if ok {
		duration, ok = parseInt32(args[1])
	}
	if !ok {
		sendText(gm, "Use: //earthquake <intensity> <duration>")
		return
	}
	x, y, z := gm.Position()
	l.broadcastAllPlayers(func() wire.Frame {
		return serverpackets.FrameEarthquake(x, y, z, intensity, duration, false)
	})
}

// broadcastAllPlayers sends every player in the world its own copy of the
// frame build makes.
func (l *GameClientLink) broadcastAllPlayers(build func() wire.Frame) {
	if l.world == nil {
		return
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, p := range l.world.Players() {
			if live, ok := p.(*livePlayer); ok {
				send(live)
			}
		}
	})
}

// adminPlaySound answers //play_sound <file>: the game master and every
// player that knows it hear the file, as a sound when its name holds a dot,
// else as music. A missing file answers the usage.
func (l *GameClientLink) adminPlaySound(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, "Usage: //play_sound soundFile")
		return
	}
	sound := serverpackets.Sound{File: args[0]}
	if !strings.Contains(sound.File, ".") {
		sound.Type = 1
	}
	l.broadcastLiveFrame(gm, func() wire.Frame { return serverpackets.FramePlaySoundAt(sound) })
}

// adminSocial answers //social <action> on the selected creature, the game
// master without one: the creature plays the social action for everyone
// that sees it, a player for itself too. A summon or chest is no valid
// target, nor an NPC outside actions 1 to 3, nor a player outside 2 to 16.
// A missing or malformed action answers the usage.
func (l *GameClientLink) adminSocial(gm *livePlayer, line string) {
	target := adminTargetCreature(gm)
	args := handleradmin.Args(line)
	var action int32
	ok := len(args) > 0
	if ok {
		action, ok = parseInt32(args[0])
	}
	if !ok {
		sendText(gm, "Usage: //social actionId")
		return
	}
	invalid := false
	switch t := target.(type) {
	case *summon.Actor:
		invalid = true
	case *livePlayer:
		invalid = action < 2 || action > 16
	case *npc.Hostile:
		invalid = t.Unlockable() || action < 1 || action > 3
	default:
		invalid = npcInstanceOf(target) != nil && (action < 1 || action > 3)
	}
	if invalid {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	id := target.ObjectID()
	l.broadcastCreatureFrame(target, func() wire.Frame { return serverpackets.FrameSocialAction(id, action) })
}

// broadcastCreatureFrame sends the frame build makes to every player that
// knows c, and to c itself when it is a player.
func (l *GameClientLink) broadcastCreatureFrame(c world.Tracked, build func() wire.Frame) {
	if live, ok := c.(*livePlayer); ok {
		l.broadcastLiveFrame(live, build)
		return
	}
	l.broadcastNPCFrame(c, build)
}

// onCreature runs fn on the queue that owns c: at once for gm or a door,
// which a command touches from gm's queue, else posted to the player's,
// NPC's or summon's queue.
func onCreature(gm *livePlayer, c world.Tracked, fn func()) {
	switch t := c.(type) {
	case *livePlayer:
		onPlayer(gm, t, fn)
	case *npc.Hostile:
		t.Queue().Post(fn)
	case *npc.Folk:
		t.Queue().Post(fn)
	case *summon.Actor:
		t.Post(fn)
	default:
		fn()
	}
}

// adminGMSpeed answers //gmspeed <level>: the game master's Super Haste
// ends, then levels 1 to 4 put that level on, any other number nothing. A
// missing or malformed level, or a level the skill table lacks, answers the
// usage. The game master's effect icons are refreshed in every case.
func (l *GameClientLink) adminGMSpeed(gm *livePlayer, line string) {
	const usage = "Use: //gmspeed value (0-4)."
	gm.StopSkillEffectsByID(superHasteSkillID)
	defer l.updateLiveAbnormalEffect(gm)

	args := handleradmin.Args(line)
	var level int32
	ok := len(args) > 0
	if ok {
		level, ok = parseInt32(args[0])
	}
	if !ok {
		sendText(gm, usage)
		return
	}
	if level < 1 || level > 4 {
		return
	}
	def, ok := l.skills.Definition(modelskill.Ref{ID: superHasteSkillID, Level: int(level)})
	if !ok {
		sendText(gm, usage)
		return
	}
	handlers := l.castEffects()
	handlers.Sink = l.playerMessageSink(gm, nil)
	actorcast.ApplyEffectsResult(handlers, gm, gm, *def)
}
