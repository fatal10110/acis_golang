package bbs

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Config is the community board's server settings.
type Config struct {
	// Enabled opens the board; while it is off every board command is
	// refused with the board-offline message.
	Enabled bool
	// Home is the command the board window opens on.
	Home string
}

// DefaultConfig is the shipped setting: the board off, opening on its home
// page.
func DefaultConfig() Config { return Config{Home: "_bbshome"} }

// PageFolder is the folder of the board's pages under the HTML root.
const PageFolder = "data/html/CommunityBoard/"

// TopFolder is the home board's page folder under PageFolder; its index is
// the board's home page.
const (
	TopFolder = "top/"
	TopIndex  = "index.htm"
)

// NotImplemented is the page a board command no board handles shows.
func NotImplemented(command string) string {
	return "<html><body><br><br><center>The command: " + command + " isn't implemented.</center></body></html>"
}

// EditFields is the edit form of a board page: the viewer, then the text,
// title and date the form starts with.
func EditFields(name string, objectID int32, account, text, title, date string) []string {
	return []string{
		"0", "0", "0", "0", "0", "0",
		name, strconv.Itoa(int(objectID)), account, "9",
		title, title, text, date, date,
		"0", "0",
	}
}

// NoticeText is a clan notice as the login notice window shows it: its
// line breaks become <br>; the window's substitution reads a backslash as
// quoting the character after it, which it keeps alone, and keeps a
// backslash ending the notice. The words "action" and "bypass" are then
// taken out until none is left, so the notice carries no link. The
// reference removes each word once, before the substitution, which lets
// "acactiontion" or "a\ction" through as a live link the window's
// clicker would run (#3214).
func NoticeText(notice string) string {
	return commons.StripLinkWords(commons.HTMLValue(strings.ReplaceAll(notice, "\r\n", "<br>")))
}
