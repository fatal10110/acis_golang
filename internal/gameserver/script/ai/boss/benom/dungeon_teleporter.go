package benom

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// dir is the script directory of the Benom behaviors.
const dir = "ai/boss/benom"

// dungeonTeleporterID is the Dungeon Gatekeeper of Rune castle.
const dungeonTeleporterID = 35506

// DungeonTeleporter is the Dungeon Gatekeeper of Rune castle, the way down
// to Benom's dungeon. It greets with its page; asked to teleport, it
// refuses with its other page while the castle's siege is fought, and
// otherwise sends the player into the dungeon. Script event 10101, sent
// when the castle is losing its life towers during a siege, makes it shout.
func DungeonTeleporter() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{
		Dir: dir, Behavior: true, NPCs: []int32{dungeonTeleporterID},
		Bind: script.Bindings{script.EventFirstTalk: {dungeonTeleporterID}, script.EventTalked: {dungeonTeleporterID}},
		Hooks: base.With(script.Hooks{
			OnCreated: func(_ *script.Script, e script.Created) {
				e.NPC.SetInt(script.SlotIntAI0, 1)
			},
			OnFirstTalk: func(s *script.Script, _ script.FirstTalk) string {
				html, _ := s.Page("data/html/doormen/35506.htm")
				return html
			},
			OnTalk: func(s *script.Script, e script.Talk) string {
				castle := s.Castle(e.NPC)
				if castle == nil {
					// A gatekeeper of no castle sends no one down.
					return ""
				}

				if castle.SiegeInProgress() {
					html, _ := s.Page("data/html/doormen/35506-2.htm")
					return html
				}

				e.Player.TeleportTo(script.Loc{X: 12589, Y: -49044, Z: -3008}, 0)

				return base.Talk(s, e)
			},
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				if e.EventID == 10101 {
					e.NPC.Shout(script.NpcString(1010632))
				}
			},
		}),
	}
}
