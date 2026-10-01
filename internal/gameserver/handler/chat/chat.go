package chat

import (
	"strings"
	"unicode/utf16"
)

// Type is a chat channel, as the client numbers it in Say2 and CreatureSay.
type Type int32

// Chat channels.
const (
	All Type = iota
	Shout
	Tell
	Party
	Clan
	GM
	PetitionPlayer
	PetitionGM
	Trade
	Alliance
	Announcement
	Boat
	L2Friend
	MSNChat
	PartyMatchRoom
	PartyRoomCommander
	PartyRoomAll
	HeroVoice
	CriticalAnnounce
	typeCount
)

var typeNames = [typeCount]string{
	"ALL", "SHOUT", "TELL", "PARTY", "CLAN", "GM", "PETITION_PLAYER", "PETITION_GM", "TRADE",
	"ALLIANCE", "ANNOUNCEMENT", "BOAT", "L2FRIEND", "MSNCHAT", "PARTYMATCH_ROOM",
	"PARTYROOM_COMMANDER", "PARTYROOM_ALL", "HERO_VOICE", "CRITICAL_ANNOUNCE",
}

// Valid reports whether t is a channel the client can name.
func (t Type) Valid() bool { return t >= 0 && t < typeCount }

// String is the channel's name as the chat log writes it.
func (t Type) String() string {
	if !t.Valid() {
		return "UNKNOWN"
	}
	return typeNames[t]
}

// MaxTextLength is the longest line, in UTF-16 code units, a player may
// say.
const MaxTextLength = 100

// Line is one admitted chat line.
type Line struct {
	Type Type
	Text string
	// Target is the name a whisper is addressed to; empty on every other
	// channel.
	Target string
}

// walkerCommands are the script commands a whisper starting with one of
// marks as sent by a bot.
var walkerCommands = [...]string{
	"USESKILL", "USEITEM", "BUYITEM", "SELLITEM", "SAVEITEM", "LOADITEM", "MSG", "DELAY", "LABEL",
	"JMP", "CALL", "RETURN", "MOVETO", "NPCSEL", "NPCDLG", "DLGSEL", "CHARSTATUS", "POSOUTRANGE",
	"POSINRANGE", "GOHOME", "SAY", "EXIT", "PAUSE", "STRINDLG", "STRNOTINDLG", "CHANGEWAITTYPE",
	"FORCEATTACK", "ISMEMBER", "REQUESTJOINPARTY", "REQUESTOUTPARTY", "QUITPARTY", "MEMBERSTATUS",
	"CHARBUFFS", "ITEMCOUNT", "FOLLOWTELEPORT",
}

// Admit applies the checks every line passes before its channel's handler
// sees it, in order: a channel the client cannot name, an empty or over-long
// line, a bot's whisper (when walkerProtection is on) and an announcement
// from a player that is no game master are all dropped without an answer.
// A petition line from a game master moves to the game master's side of the
// petition. gm is whether the speaker plays under a game-master access
// level. The returned line is the one to log; Clean gives the text to
// deliver.
func Admit(typ int32, text, target string, gm, walkerProtection bool) (Line, bool) {
	t := Type(typ)
	if !t.Valid() {
		return Line{}, false
	}
	if text == "" || len(utf16.Encode([]rune(text))) > MaxTextLength {
		return Line{}, false
	}
	if walkerProtection && t == Tell && isWalkerCommand(text) {
		return Line{}, false
	}
	if !gm && (t == Announcement || t == CriticalAnnounce) {
		return Line{}, false
	}
	if t == PetitionPlayer && gm {
		t = PetitionGM
	}
	return Line{Type: t, Text: text, Target: target}, true
}

func isWalkerCommand(text string) bool {
	for _, command := range walkerCommands {
		if strings.HasPrefix(text, command) {
			return true
		}
	}
	return false
}

// Clean returns the text delivered for text: every literal backslash-n pair
// removed.
func Clean(text string) string {
	return strings.ReplaceAll(text, `\n`, "")
}

// LogEntry is the chat log's record of a line said by speaker: the channel,
// who spoke (and to whom, for a whisper), and the line as it was said.
func LogEntry(line Line, speaker string) string {
	if line.Type == Tell {
		return line.Type.String() + " [" + speaker + " to " + line.Target + "] " + line.Text
	}
	return line.Type.String() + " [" + speaker + "] " + line.Text
}

// FriendLogEntry is the chat log's record of a friend message from sender
// to recipient.
func FriendLogEntry(sender, recipient, message string) string {
	return "PRIV_MSG [" + sender + " to " + recipient + "] " + message
}
