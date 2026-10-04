package network

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/fence"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminSpawnFence answers //spawnfence <type> <width> <length> [height]: a
// fence of that type is placed where gm stands, its width and length cut
// down to whole hundreds and its height capped at 3 layers, then the fence
// list opens. A fence that cannot be placed (past the world edge, or
// longer than the size table) is only logged; the list opens all the same.
// A missing or unreadable argument answers the usage.
func (l *GameClientLink) adminSpawnFence(gm *livePlayer, line string) {
	const usage = "Usage: //spawnfence <type> <width> <length> [height]"
	args := handleradmin.Args(line)
	if len(args) < 3 {
		sendText(gm, usage)
		return
	}
	values := [4]int32{3: 1}
	for i := range min(len(args), len(values)) {
		v, ok := parseJavaInt(args[i])
		if !ok {
			sendText(gm, usage)
			return
		}
		values[i] = v
	}
	typ, sizeX, sizeY, height := values[0], values[1]/100*100, values[2]/100*100, min(values[3], 3)
	if l.fences != nil {
		x, y, z := gm.Position()
		if _, err := l.fences.Add(x, y, z, int(typ), int(sizeX), int(sizeY), int(height)); err != nil {
			l.log.Warn().Err(err).Msg("admin: //spawnfence placed no fence")
		}
	}
	l.sendFenceList(gm)
}

// adminDeleteFence answers //deletefence <objectId> [1]: the fence of that
// object id is removed, and the fence list opens again when a second
// argument follows. Any other object, or none, is an invalid target; a
// missing or unreadable id answers the usage.
func (l *GameClientLink) adminDeleteFence(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	var id int32
	ok := len(args) > 0
	if ok {
		id, ok = parseJavaInt(args[0])
	}
	if !ok {
		sendText(gm, "Usage: //deletefence <objectId>")
		return
	}
	obj, found := l.world.Object(id)
	f, isFence := obj.(*fence.Fence)
	if !found || !isFence || l.fences == nil {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	l.fences.Remove(f)
	if len(args) > 1 {
		l.sendFenceList(gm)
	}
}

// adminListFence answers //listfence with the fence list.
func (l *GameClientLink) adminListFence(gm *livePlayer, _ string) {
	l.sendFenceList(gm)
}

// sendFenceList opens the fence list: how many fences are placed, then one
// link per fence, in placement order, that removes it and reopens the list.
func (l *GameClientLink) sendFenceList(gm *livePlayer) {
	var fences []*fence.Fence
	if l.fences != nil {
		fences = l.fences.Fences()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<html><body>Total Fences: %d<br><br>", len(fences))
	for _, f := range fences {
		x, y, z := f.Position()
		fmt.Fprintf(&b, `<a action="bypass -h admin_deletefence %d 1">Fence: %d [%d %d %d]</a><br>`, f.ObjectID(), f.ObjectID(), x, y, z)
	}
	b.WriteString("</body></html>")
	sendValidatedHTML(gm, 0, b.String(), 0)
}
