package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// crowdQuest is the script whose teleports crowd a landing in
// TestJailAfterHeldScriptMovesAppliesOnLanding.
const crowdQuest = "Q953_CrowdedLanding"

// TestJailAfterHeldScriptMovesAppliesOnLanding pins that a jail refused
// while the player's own teleport lands is made once it lands however
// many other moves are held for that landing (owner-approved, #5121):
// with the GM's recall in flight, a script asks more teleports than are
// ever held for one landing, then the GM jails the player. The landing
// takes the player to the jail, and none of the script's held moves takes
// it back out.
func TestJailAfterHeldScriptMovesAppliesOnLanding(t *testing.T) {
	t.Parallel()
	catalog := script.Catalog{"quest." + crowdQuest: func() script.Script {
		return script.Script{Title: "Crowded Landing", QuestID: 953}
	}}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(punishPages(t)),
		gameservertest.WithZones(shippedJailZones(t)),
		gameservertest.WithNPCScripts(nil, []script.Listing{{Path: "quest." + crowdQuest}}, catalog))
	enterWorld(t, srv.Client)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, srv.Client)
	settle(t, user)
	gm := srv.Client

	exchange(t, gm, encodeBuildCmd("recall Player"))
	recalled := teleportOf(t, settle(t, user), userID)
	away := script.Loc{X: int(recalled[0]) + 300, Y: int(recalled[1]), Z: int(recalled[2])}
	for range 12 {
		srv.RunScript(t, userID, crowdQuest, func(_ *script.Script, p *script.Player) { p.TeleportTo(away, 0) })
	}
	noTeleport(t, settle(t, user))
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("jail Player 30"))), "Player is jailed for 30 minutes.")
	noTeleport(t, settle(t, user))

	if at := teleportOf(t, landedTeleport(t, user), userID); at != jailAt {
		t.Fatalf("teleport after the landing = %v, want the jail %v", at, jailAt)
	}
	appear(t, user)
	noTeleport(t, settle(t, user))
	if x, y, _ := srv.PlayerPosition(t, userID); int32(x) != jailAt[0] || int32(y) != jailAt[1] {
		t.Fatalf("Player stands at %d %d, want in the jail at %v", x, y, jailAt)
	}
}
