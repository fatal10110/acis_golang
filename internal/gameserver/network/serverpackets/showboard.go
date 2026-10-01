package serverpackets

import (
	"strings"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodeShowBoard is the wire opcode for ShowBoard, the community board
// window.
const OpcodeShowBoard = 0x6e

// The links of the board window's tab bar, sent ahead of every page.
var showBoardTabs = [...]string{
	"bypass _bbshome",
	"bypass _bbsgetfav",
	"bypass _bbsloc",
	"bypass _bbsclan",
	"bypass _bbsmemo",
	"bypass _maillist_0_1_0_",
	"bypass _friendlist_0_",
	"bypass _bbsgetfav_add",
}

// ShowBoardNoPage is the content of a page part a page leaves unused: the
// part's id followed by the text "null".
const ShowBoardNoPage = "null"

// showBoardSeparator ends a page part's id and each edit field.
const showBoardSeparator = "\b"

// FrameShowBoard shows one part of a board page: id names the part (101,
// 102 and 103 for the three parts of a page, 1001 for a page with an edit
// form) and html is its content.
func FrameShowBoard(id, html string) wire.Frame {
	return frameShowBoard(id + showBoardSeparator + html)
}

// FrameShowBoardFields fills the edit form of the last 1001 page with
// fields, each followed by a space and the separator.
func FrameShowBoardFields(fields []string) wire.Frame {
	var b strings.Builder
	b.WriteString("1002" + showBoardSeparator)
	for _, f := range fields {
		b.WriteString(f)
		b.WriteString(" " + showBoardSeparator)
	}
	return frameShowBoard(b.String())
}

func frameShowBoard(content string) wire.Frame {
	w := newFrameWriter(OpcodeShowBoard)
	w.WriteUint8(1)
	for _, tab := range showBoardTabs {
		w.WriteString(tab)
	}
	w.WriteString(content)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// The board page limits, in UTF-16 code units.
const (
	// showBoardPartLength is the most one part of a page holds.
	showBoardPartLength = 4090
	// showBoardEditLength bounds a page with an edit form: it must be
	// shorter.
	showBoardEditLength = 2 * showBoardPartLength
	// showBoardPageLength bounds a page: it must be shorter.
	showBoardPageLength = 3 * showBoardPartLength
)

// FramesShowBoardPage cuts html into the three parts of a board page, 101,
// 102 and 103, each holding up to 4090 UTF-16 code units in order; a part
// html does not reach carries ShowBoardNoPage. A page of 12270 units or
// more is not shown: nil. A character whose two code units straddle a cut
// reaches the client as two replacement characters.
func FramesShowBoardPage(html string) []wire.Frame {
	units := utf16.Encode([]rune(html))
	n := len(units)
	if n >= showBoardPageLength {
		return nil
	}
	part := func(from, to int) string { return string(utf16.Decode(units[from:to])) }
	switch {
	case n < showBoardPartLength:
		return []wire.Frame{
			FrameShowBoard("101", html),
			FrameShowBoard("102", ShowBoardNoPage),
			FrameShowBoard("103", ShowBoardNoPage),
		}
	case n < showBoardEditLength:
		return []wire.Frame{
			FrameShowBoard("101", part(0, showBoardPartLength)),
			FrameShowBoard("102", part(showBoardPartLength, n)),
			FrameShowBoard("103", ShowBoardNoPage),
		}
	default:
		return []wire.Frame{
			FrameShowBoard("101", part(0, showBoardPartLength)),
			FrameShowBoard("102", part(showBoardPartLength, showBoardEditLength)),
			FrameShowBoard("103", part(showBoardEditLength, n)),
		}
	}
}

// FrameShowBoardEdit shows html as a page with an edit form, part 1001. It
// reports false, building nothing, for a page of 8180 UTF-16 code units or
// more.
func FrameShowBoardEdit(html string) (wire.Frame, bool) {
	if len(utf16.Encode([]rune(html))) >= showBoardEditLength {
		return wire.Frame{}, false
	}
	return FrameShowBoard("1001", html), true
}
