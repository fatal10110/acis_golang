package network

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ShutdownNotices delivers the shutdown countdown's notices to every player
// in a world.
type ShutdownNotices struct {
	state *world.State
}

// NewShutdownNotices returns the countdown's delivery to every player in
// state.
func NewShutdownNotices(state *world.State) ShutdownNotices {
	return ShutdownNotices{state: state}
}

// ServerDownIn tells every player online the server stops in seconds.
func (n ShutdownNotices) ServerDownIn(seconds int32) {
	if n.state == nil {
		return
	}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageServerDownInS1Seconds, seconds)
	}, func(send func(frameReceiver)) {
		for _, p := range n.state.Players() {
			if listener, ok := p.(*livePlayer); ok {
				send(listener)
			}
		}
	})
}

// Announce says text, by no one, to every player online on the
// announcement channel, the critical one when critical.
func (n ShutdownNotices) Announce(text string, critical bool) {
	announceToOnline(n.state, text, critical)
}

// adminServer answers //server <shutdown|restart|abort|gmonly|all|max>,
// then shows the maintenance page; with no argument it only shows the page.
//
//   - shutdown <seconds> and restart <seconds> start the countdown that
//     stops the server, asking for a restart with restart.
//   - abort stops the countdown.
//   - gmonly reserves the server for game masters; all opens it again.
//   - max <n> advertises n as the player cap.
//
// A missing or malformed number is answered with the usage line; an
// unknown argument with the page alone.
func (l *GameClientLink) adminServer(gm *livePlayer, line string) {
	tokens := javaTokens(line)
	if len(tokens) < 2 {
		l.sendMaintenancePage(gm)
		return
	}
	number := func() (int32, bool) {
		if len(tokens) < 3 {
			return 0, false
		}
		return parseJavaInt(tokens[2])
	}
	usage := false
	switch tokens[1] {
	case "shutdown", "restart":
		if seconds, ok := number(); ok {
			l.shutdown.Start(gm.CharacterName(), seconds, tokens[1] == "restart")
		} else {
			usage = true
		}
	case "abort":
		l.shutdown.Abort(gm.CharacterName())
	case "gmonly":
		l.loginStatus.SetGMOnly(true)
		sendText(gm, "Server is now set as GMonly.")
	case "all":
		l.loginStatus.SetGMOnly(false)
		sendText(gm, "Server isn't set as GMonly anymore.")
	case "max":
		if n, ok := number(); ok {
			l.loginStatus.SetMaxPlayers(n)
			sendText(gm, "Server maximum player amount is set to "+strconv.Itoa(int(n))+".")
		} else {
			usage = true
		}
	}
	if usage {
		sendText(gm, "Usage: //server <shutdown|restart|abort|gmonly|all|max>.")
	}
	l.sendMaintenancePage(gm)
}

// sendMaintenancePage shows gm the server maintenance page: the server's
// name, players online, player cap, memory in use, in-game time and login
// status.
func (l *GameClientLink) sendMaintenancePage(gm *livePlayer) {
	online := 0
	if l.world != nil {
		online = len(l.world.Players())
	}
	gameTime := ""
	if l.gameClock != nil {
		gameTime = l.gameClock.String()
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	page := l.adminHTML("maintenance.htm")
	for _, r := range [...][2]string{
		{"%count%", strconv.Itoa(online)},
		{"%used%", strconv.FormatUint(mem.HeapAlloc/(1<<20), 10)},
		{"%server_name%", l.loginStatus.ServerName()},
		{"%status%", l.loginStatus.Type().String()},
		{"%max_players%", strconv.Itoa(int(l.loginStatus.MaxPlayers()))},
		{"%time%", gameTime},
	} {
		page = strings.ReplaceAll(page, r[0], r[1])
	}
	sendValidatedHTML(gm, 0, page, 0)
}
