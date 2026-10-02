package network

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
)

// The memo board is a forum of topics, each with its text as post 0. The
// board reaches any forum by its id: a topic or post command takes the
// forum it names, whoever owns it, as the reference does (#3262).

// boardTopics runs a memo or topic command: _bbsmemo shows the player's
// memo forum, created on first use; _bbstopics;read;<forum>[;<page>]
// shows a memo forum's topic list page; _bbstopics;crea;<forum> opens a
// memo forum's new topic form; _bbstopics;del;<forum>;<topic> deletes a
// topic, then shows the player's memo forum. Any other command shows the
// unknown-command page.
func (l *GameClientLink) boardTopics(live *livePlayer, command string) {
	switch {
	case command == "_bbsmemo":
		l.showMemo(live)
	case strings.HasPrefix(command, "_bbstopics;read"):
		tokens := bbs.Tokens(command, ";")
		if len(tokens) < 3 {
			return
		}
		forumID, ok := parseJavaInt(tokens[2])
		if !ok {
			return
		}
		index := int32(1)
		if len(tokens) > 3 {
			if index, ok = parseJavaInt(tokens[3]); !ok {
				return
			}
		}
		l.showTopics(live, forumID, index)
	case strings.HasPrefix(command, "_bbstopics;crea"):
		tokens := bbs.Tokens(command, ";")
		if len(tokens) < 3 {
			return
		}
		forumID, ok := parseJavaInt(tokens[2])
		if !ok {
			return
		}
		if f, ok := l.board.forums.Forum(forumID); !ok || f.Type != bbs.ForumMemo {
			l.sendBoard(live, bbs.ForumMissing(forumID))
			return
		}
		l.sendBoardEdit(live, bbs.RenderMemoNewTopic(forumID), " ", " ", "0")
	case strings.HasPrefix(command, "_bbstopics;del"):
		tokens := bbs.Tokens(command, ";")
		if len(tokens) < 4 {
			return
		}
		forumID, ok := parseJavaInt(tokens[2])
		if !ok {
			return
		}
		topicID, ok := parseJavaInt(tokens[3])
		if !ok {
			return
		}
		switch l.board.forums.DeleteTopic(forumID, topicID) {
		case bbs.FoundNothing:
			l.sendBoard(live, bbs.ForumMissing(forumID))
		case bbs.FoundForum:
			l.sendBoard(live, bbs.TopicMissing(topicID))
		default:
			l.showMemo(live)
		}
	default:
		l.sendBoard(live, bbs.NotImplemented(command))
	}
}

// showMemo shows the first topic list page of live's memo forum, creating
// the forum on first use.
func (l *GameClientLink) showMemo(live *livePlayer) {
	memo := l.board.forums.Memo(live.ObjectID())
	l.showTopics(live, memo.ID, 1)
}

// showTopics shows page index of memo forum forumID's topic list. A
// forum that is not a memo forum shows as missing.
func (l *GameClientLink) showTopics(live *livePlayer, forumID, index int32) {
	f, ok := l.board.forums.View(forumID)
	if !ok || f.Type != bbs.ForumMemo {
		l.sendBoard(live, bbs.ForumMissing(forumID))
		return
	}
	l.sendBoard(live, bbs.RenderMemoTopics(f, index, l.clanService().Table().Len()))
}

// boardTopicWrite submits a topic form: crea <forum> <_> <text> <name>
// opens a topic in the forum, del <forum> <topic> deletes one; either then
// shows the player's memo forum. Any other form shows the unknown-command
// page naming its first argument.
func (l *GameClientLink) boardTopicWrite(live *livePlayer, args [5]string) {
	switch args[0] {
	case "crea":
		forumID, ok := parseJavaInt(args[1])
		if !ok {
			return
		}
		if !l.board.forums.AddTopic(forumID, args[4], live.Name, live.ObjectID(), args[3], time.Now()) {
			l.sendBoard(live, bbs.NamedForumMissing(args[1]))
			return
		}
		l.showMemo(live)
	case "del":
		forumID, ok := parseJavaInt(args[1])
		if !ok {
			return
		}
		// The topic is read only once the forum is found: a missing
		// forum is named whatever the topic argument holds.
		if _, ok := l.board.forums.Forum(forumID); !ok {
			l.sendBoard(live, bbs.NamedForumMissing(args[1]))
			return
		}
		topicID, ok := parseJavaInt(args[2])
		if !ok {
			return
		}
		switch l.board.forums.DeleteTopic(forumID, topicID) {
		case bbs.FoundNothing:
			l.sendBoard(live, bbs.NamedForumMissing(args[1]))
		case bbs.FoundForum:
			l.sendBoard(live, bbs.NamedTopicMissing(args[2]))
		default:
			l.showMemo(live)
		}
	default:
		l.sendBoard(live, bbs.NotImplemented(args[0]))
	}
}

// boardPosts runs a post command: _bbsposts;read;<forum>;<topic> shows a
// memo topic's post page, _bbsposts;edit;<forum>;<topic> opens a topic's
// edit form filled with its text. Any other command shows the
// unknown-command page.
func (l *GameClientLink) boardPosts(live *livePlayer, command string) {
	read := strings.HasPrefix(command, "_bbsposts;read;")
	if !read && !strings.HasPrefix(command, "_bbsposts;edit;") {
		l.sendBoard(live, bbs.NotImplemented(command))
		return
	}
	tokens := bbs.Tokens(command, ";")
	if len(tokens) < 4 {
		return
	}
	forumID, ok := parseJavaInt(tokens[2])
	if !ok {
		return
	}
	topicID, ok := parseJavaInt(tokens[3])
	if !ok {
		return
	}
	if read {
		l.showPost(live, forumID, topicID)
		return
	}
	f, t, p, found := l.board.forums.TopicPost(forumID, topicID, 0)
	switch found {
	case bbs.FoundNothing:
		l.sendBoard(live, bbs.PostForumMissing)
	case bbs.FoundForum:
		l.sendBoard(live, bbs.PostTopicMissing)
	case bbs.FoundTopic:
		l.sendBoard(live, bbs.PostMissing)
	default:
		l.sendBoardEdit(live, bbs.RenderPostEdit(f.ID, t), p.Text, t.Name, bbs.ShortDate(t.Date))
	}
}

// showPost shows the post page of topic topicID of memo forum forumID. A
// topic of another forum type is off-limits; a topic without its post 0
// shows nothing.
func (l *GameClientLink) showPost(live *livePlayer, forumID, topicID int32) {
	f, t, p, found := l.board.forums.TopicPost(forumID, topicID, 0)
	switch {
	case found == bbs.FoundNothing:
		l.sendBoard(live, bbs.PostForumMissing)
	case found == bbs.FoundForum:
		l.sendBoard(live, bbs.PostTopicMissing)
	case f.Type != bbs.ForumMemo:
		l.sendBoard(live, bbs.PostOffLimits)
	case found == bbs.FoundPost:
		l.sendBoard(live, bbs.RenderMemoPost(t, p))
	default:
		l.log.Debug().Int32("forum", forumID).Int32("topic", topicID).Msg("community board: topic has no post")
	}
}

// boardPostWrite submits a post form, <forum>;<topic>;<post> <_> <_>
// <text>: the post takes the text, then the topic's post page shows.
func (l *GameClientLink) boardPostWrite(live *livePlayer, args [5]string) {
	tokens := bbs.Tokens(args[0], ";")
	if len(tokens) < 3 {
		return
	}
	var ids [3]int32
	for i := range ids {
		n, ok := parseJavaInt(tokens[i])
		if !ok {
			return
		}
		ids[i] = n
	}
	forumID, topicID, postID := ids[0], ids[1], ids[2]
	switch l.board.forums.EditPost(forumID, topicID, postID, args[3]) {
	case bbs.FoundNothing:
		l.sendBoard(live, bbs.NamedForumMissing(itoa32(forumID)))
	case bbs.FoundForum:
		l.sendBoard(live, bbs.NamedTopicMissing(itoa32(topicID)))
	case bbs.FoundTopic:
		l.sendBoard(live, bbs.NamedPostMissing(postID))
	default:
		l.showPost(live, forumID, topicID)
	}
}

func itoa32(n int32) string { return strconv.Itoa(int(n)) }
