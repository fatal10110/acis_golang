package network

import (
	"context"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/memo"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// tutorialRequest answers the four tutorial requests: the event they carry
// goes to live's tutorial quest, which answers through the tutorial window.
// A player with no tutorial quest state gets nothing at all;
// the tutorial window and its question marks leave the client waiting on
// no answer, so no ActionFailed is owed either.
func (l *GameClientLink) tutorialRequest(live *livePlayer, name string) {
	live.NotifyTutorial(name)
}

// tutorialPickedUp hands the tutorial quest of picker, the player that
// picked ground up itself or through its pet, the pickup of adena or of a
// blue gemstone. It runs right after the pickup's GetItem, before the item
// leaves the world.
func tutorialPickedUp(picker *livePlayer, itemID int32) {
	if itemID == item.AdenaID || itemID == blueGemstoneID {
		picker.NotifyTutorial(fmt.Sprintf("CE%d", itemID))
	}
}

// blueGemstoneID is the gemstone whose pickup the tutorial hears of.
const blueGemstoneID = 6353

// soundVoice is the PlaySound type of a voice.
const soundVoice = 2

// memoWriteTimeout bounds one memo write.
const memoWriteTimeout = 5 * time.Second

// sendTutorial maps live's tutorial window and radar events to their
// packets, all for live alone.
func (l *GameClientLink) sendTutorial(live *livePlayer, ev event.Event) {
	switch e := ev.(type) {
	case event.TutorialPageShown:
		page, ok := "", false
		if l.html != nil {
			page, ok = l.html.Get(e.File)
		}
		if !ok {
			page = "<html><body>My html is missing:<br>" + e.File + "</body></html>"
		}
		live.SendFrame(serverpackets.FrameTutorialShowHTML(page))
	case event.TutorialPageClosed:
		live.SendFrame(serverpackets.FrameTutorialCloseHTML())
	case event.TutorialQuestionMarkShown:
		live.SendFrame(serverpackets.FrameTutorialShowQuestionMark(e.ID))
	case event.TutorialClientEventEnabled:
		live.SendFrame(serverpackets.FrameTutorialEnableClientEvent(e.ID))
	case event.TutorialVoicePlayed:
		live.SendFrame(serverpackets.FramePlaySoundAt(serverpackets.Sound{
			Type: soundVoice, File: e.Voice, BindToObject: true,
			ObjectID: live.ObjectID(), Location: live.CurrentLocation(),
		}))
	case event.RadarMarkerAdded:
		live.SendFrame(serverpackets.FrameRadarControl(serverpackets.Radar{Show: 2, Type: 2, X: e.X, Y: e.Y, Z: e.Z}))
		live.SendFrame(serverpackets.FrameRadarControl(serverpackets.Radar{Show: 0, Type: 1, X: e.X, Y: e.Y, Z: e.Z}))
	case event.RadarMarkerRemoved:
		live.SendFrame(serverpackets.FrameRadarControl(serverpackets.Radar{Show: 1, Type: 1, X: e.X, Y: e.Y, Z: e.Z}))
	}
}

// memoStore reads and writes the character_memo rows.
type memoStore interface {
	ListMemos(ctx context.Context, ownerID int32) (map[string]string, error)
	SetMemo(ctx context.Context, ownerID int32, key, value string) error
	UnsetMemo(ctx context.Context, ownerID int32, key string) error
}

// restoreMemos loads c's memos at its selection. A failed read is
// returned: the selection must not go on with no memos, which would offer
// the one-time rewards they record again.
func (l *GameClientLink) restoreMemos(ctx context.Context, c *player.Character) error {
	if l.memos == nil {
		return nil
	}
	vals, err := l.memos.ListMemos(ctx, c.ID)
	if err != nil {
		return err
	}
	c.Memos().Restore(vals, memoWriter{link: l, ownerID: c.ID})
	return nil
}

// memoWriter saves one character's memo changes on its persistence lane, in
// the order they are made.
type memoWriter struct {
	link    *GameClientLink
	ownerID int32
}

var _ memo.Writer = memoWriter{}

func (w memoWriter) Set(key, value string) {
	w.write("set", key, func(ctx context.Context) error { return w.link.memos.SetMemo(ctx, w.ownerID, key, value) })
}

func (w memoWriter) Unset(key string) {
	w.write("unset", key, func(ctx context.Context) error { return w.link.memos.UnsetMemo(ctx, w.ownerID, key) })
}

func (w memoWriter) write(op, key string, run func(context.Context) error) {
	w.link.persist.Enqueue(w.ownerID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), memoWriteTimeout)
		defer cancel()
		if err := run(ctx); err != nil {
			w.link.log.Error().Err(err).Int32("object_id", w.ownerID).Str("op", op).Str("memo", key).Msg("write memo")
		}
	})
}

// tutorialDecoder returns the decoder of the tutorial request opcode.
func tutorialDecoder(opcode byte) func([]byte) (clientpackets.TutorialEvent, error) {
	switch opcode {
	case clientpackets.OpcodeRequestTutorialLinkHTML:
		return clientpackets.DecodeRequestTutorialLinkHTML
	case clientpackets.OpcodeRequestTutorialPassCmdToServer:
		return clientpackets.DecodeRequestTutorialPassCmdToServer
	case clientpackets.OpcodeRequestTutorialQuestionMark:
		return clientpackets.DecodeRequestTutorialQuestionMark
	default:
		return clientpackets.DecodeRequestTutorialClientEvent
	}
}
