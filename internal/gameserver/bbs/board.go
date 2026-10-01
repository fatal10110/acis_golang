package bbs

import (
	"strconv"
	"strings"
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
// line breaks become <br> and the words "action" and "bypass" are taken
// out, so it carries no link. The window's substitution then reads a
// backslash as quoting the character after it, which it keeps alone; a
// backslash ending the notice is kept.
func NoticeText(notice string) string {
	notice = strings.ReplaceAll(notice, "\r\n", "<br>")
	notice = strings.ReplaceAll(notice, "action", "")
	notice = strings.ReplaceAll(notice, "bypass", "")
	if !strings.Contains(notice, `\`) {
		return notice
	}
	var b strings.Builder
	for i := 0; i < len(notice); i++ {
		if notice[i] == '\\' && i+1 < len(notice) {
			i++
		}
		b.WriteByte(notice[i])
	}
	return b.String()
}
