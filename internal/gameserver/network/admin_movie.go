package network

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// movieShot is one camera shot of the server's movie, centred on the object
// it was recorded around.
type movieShot struct {
	id       int32
	objectID int32
	shot     event.CameraShot
}

// frame builds s's SpecialCamera packet.
func (s movieShot) frame() wire.Frame { return serverpackets.FrameSpecialCamera(s.objectID, s.shot) }

// movieBoard holds the shots every game master's //sequence edits, by id;
// the zero value is empty. Its own mutex guards it: game masters edit it
// from their own queues.
type movieBoard struct {
	mu    sync.Mutex
	shots map[int32]movieShot
}

// all returns every shot in id order.
func (b *movieBoard) all() []movieShot {
	b.mu.Lock()
	out := make([]movieShot, 0, len(b.shots))
	for _, s := range b.shots {
		out = append(out, s)
	}
	b.mu.Unlock()
	slices.SortFunc(out, func(a, b movieShot) int { return cmp.Compare(a.id, b.id) })
	return out
}

func (b *movieBoard) get(id int32) (movieShot, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.shots[id]
	return s, ok
}

// addIfAbsent records s unless a shot holds its id already; that one stays.
func (b *movieBoard) addIfAbsent(s movieShot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.shots[s.id]; ok {
		return
	}
	if b.shots == nil {
		b.shots = map[int32]movieShot{}
	}
	b.shots[s.id] = s
}

// remove deletes shot id, reporting whether there was one.
func (b *movieBoard) remove(id int32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.shots[id]
	delete(b.shots, id)
	return ok
}

// Movie playback timing.
const (
	// movieFirstShotDelay is when the first shot of a played movie starts.
	movieFirstShotDelay = 500 * time.Millisecond
	// movieShotOverlap is how long before a shot ends the next one starts.
	movieShotOverlap = 100 * time.Millisecond
)

// adminMovie answers //movie [play|broadcast]: play shows every shot in id
// order to gm, broadcast to gm and the players around, each shot starting
// as the one before ends; no parameter opens the movie panel, any other
// is ignored.
func (l *GameClientLink) adminMovie(gm *livePlayer, line string) {
	args := spaceTokens(line)[1:]
	if len(args) == 0 {
		l.sendMoviePanel(gm)
		return
	}
	switch args[0] {
	case "play":
		l.playMovie(gm, false)
	case "broadcast":
		l.playMovie(gm, true)
	default:
		// Like an unknown //sequence action, an unknown //movie action
		// answers nothing; a typed or clicked command leaves the client
		// waiting on nothing.
	}
}

// playMovie arms every shot on gm's queue, around gm too when broadcast.
// The shots stop when gm leaves the world.
func (l *GameClientLink) playMovie(gm *livePlayer, broadcast bool) {
	shots := l.movies.all()
	if len(shots) == 0 {
		sendText(gm, "There is nothing to play.")
		l.sendMoviePanel(gm)
		return
	}
	at := movieFirstShotDelay
	for _, s := range shots {
		gm.after(max(at, 0), func() { l.showShot(gm, s, broadcast) })
		at += time.Duration(s.shot.Duration)*time.Millisecond - movieShotOverlap
	}
}

// showShot sends s to gm, and to the players that know gm when broadcast.
func (l *GameClientLink) showShot(gm *livePlayer, s movieShot, broadcast bool) {
	if broadcast {
		l.broadcastLiveFrame(gm, s.frame)
		return
	}
	gm.SendFrame(s.frame())
}

// sendMoviePanel opens the movie panel: the shots in id order, or the
// empty panel.
func (l *GameClientLink) sendMoviePanel(gm *livePlayer) {
	shots := l.movies.all()
	if len(shots) == 0 {
		l.sendFilePage(gm, 0, adminFile("movie/main_empty.htm"), l.adminHTML("movie/main_empty.htm"), 0)
		return
	}
	var b strings.Builder
	for _, s := range shots {
		b.WriteString("<tr>")
		for _, v := range [...]int32{s.id, s.shot.Distance, s.shot.Yaw, s.shot.Pitch, s.shot.Time, s.shot.Duration, s.shot.Turn, s.shot.Rise, s.shot.Widescreen} {
			b.WriteString("<td>" + strconv.Itoa(int(v)) + "</td>")
		}
		b.WriteString("</tr>")
	}
	html := strings.ReplaceAll(l.adminHTML("movie/main_notempty.htm"), "%sequences%", b.String())
	l.sendFilePage(gm, 0, adminFile("movie/main_notempty.htm"), html, 0)
}

// adminSequence answers //sequence add|delete|play|broadcast, editing and
// trying the movie's shots; no parameter opens the movie panel, any other
// is ignored.
func (l *GameClientLink) adminSequence(gm *livePlayer, line string) {
	args := spaceTokens(line)[1:]
	if len(args) == 0 {
		l.sendMoviePanel(gm)
		return
	}
	switch args[0] {
	case "add":
		l.addSequence(gm, args[1:])
	case "delete":
		if id, ok := sequenceID(args[1:]); !ok {
			sendText(gm, "You entered an invalid sequence id.")
		} else if !l.movies.remove(id) {
			sendText(gm, "The sequence id doesn't exist.")
		}
		l.sendMoviePanel(gm)
	case "play", "broadcast":
		id, ok := sequenceID(args[1:])
		if !ok {
			sendText(gm, "You entered an invalid sequence id.")
			l.sendMoviePanel(gm)
			return
		}
		s, ok := l.movies.get(id)
		if !ok {
			sendText(gm, "The sequence id doesn't exist.")
			l.sendMoviePanel(gm)
			return
		}
		l.showShot(gm, s, args[0] == "broadcast")
	default:
		// An unknown action answers nothing, as an unknown //movie one.
	}
}

// sequenceID reads the shot id args lead with.
func sequenceID(args []string) (int32, bool) {
	if len(args) == 0 {
		return 0, false
	}
	return parseInt32(args[0])
}

// addSequence answers //sequence add [id [dist yaw pitch time duration turn
// rise widescreen]]: no id opens the new-shot form, an id alone the edit
// form of that shot, and an id with all eight values records a new shot
// around gm's target, or gm without one. An id already recorded keeps its
// shot.
func (l *GameClientLink) addSequence(gm *livePlayer, args []string) {
	if len(args) == 0 {
		l.sendFilePage(gm, 0, adminFile("movie/add_sequence.htm"), l.adminHTML("movie/add_sequence.htm"), 0)
		return
	}
	id, ok := parseInt32(args[0])
	if !ok {
		l.sendMoviePanel(gm)
		return
	}
	if id < 0 {
		sendText(gm, "The sequence id is invalid.")
		l.sendMoviePanel(gm)
		return
	}
	values := args[1:]
	if len(values) == 0 {
		s, ok := l.movies.get(id)
		if !ok {
			sendText(gm, "The sequence couldn't be updated.")
			l.sendMoviePanel(gm)
			return
		}
		l.sendSequenceForm(gm, s)
		return
	}
	if len(values) != 8 {
		sendText(gm, "Some arguments are missing.")
		l.sendMoviePanel(gm)
		return
	}
	var v [8]int32
	for i, text := range values {
		if v[i], ok = parseInt32(text); !ok {
			l.sendMoviePanel(gm)
			return
		}
	}
	var target world.Tracked = gm
	if t := gm.Target(); t != nil {
		target = t
	}
	l.movies.addIfAbsent(movieShot{id: id, objectID: target.ObjectID(), shot: event.CameraShot{
		Distance: v[0], Yaw: v[1], Pitch: v[2], Time: v[3], Duration: v[4], Turn: v[5], Rise: v[6], Widescreen: v[7],
	}})
	l.sendMoviePanel(gm)
}

// sendSequenceForm opens the edit form of shot s.
func (l *GameClientLink) sendSequenceForm(gm *livePlayer, s movieShot) {
	itoa := func(v int32) string { return strconv.Itoa(int(v)) }
	html := fillPage(l.adminHTML("movie/edit_sequence.htm"),
		"%sId%", itoa(s.id),
		"%sDist%", itoa(s.shot.Distance),
		"%sYaw%", itoa(s.shot.Yaw),
		"%sPitch%", itoa(s.shot.Pitch),
		"%sTime%", itoa(s.shot.Time),
		"%sDuration%", itoa(s.shot.Duration),
		"%sTurn%", itoa(s.shot.Turn),
		"%sRise%", itoa(s.shot.Rise),
		"%sWidescreen%", itoa(s.shot.Widescreen),
	)
	l.sendFilePage(gm, 0, adminFile("movie/edit_sequence.htm"), html, 0)
}
