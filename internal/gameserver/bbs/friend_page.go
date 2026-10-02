package bbs

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// The friends board's pages, under the board's page folder.
const (
	FriendListPage = "friend/friend-list.htm"
	BlockListPage  = "friend/friend-blocklist.htm"
	FriendMailPage = "friend/friend-mail.htm"
)

// The delete-all confirmations the friends and block lists show.
const (
	friendDeleteAll = "<br>\n<table><tr><td width=10></td><td>Are you sure you want to delete all friends from your Friends List?</td><td width=20></td><td><button value=\"OK\" action=\"bypass _friend;delall\" back=\"l2ui_ch3.smallbutton2_down\" width=65 height=20 fore=\"l2ui_ch3.smallbutton2\"></td></tr></table>"
	blockDeleteAll  = "<br>\n<table><tr><td width=10></td><td>Are you sure you want to delete all players from your Block List?</td><td width=20></td><td><button value=\"OK\" action=\"bypass _block;delall\" back=\"l2ui_ch3.smallbutton2_down\" width=65 height=20 fore=\"l2ui_ch3.smallbutton2\"></td></tr></table>"
)

// Contacts looks up the characters a friends board page lists.
type Contacts interface {
	// Name is the character's name; false when no character has the id.
	Name(id int32) (string, bool)
	// Online reports whether the character is in the world.
	Online(id int32) bool
}

// Selection is a set of character ids picked on a friends board list,
// kept in a hash table that starts at 16 buckets and doubles, never
// shrinking, once it holds three quarters of them. The zero value is
// empty. It is not safe for concurrent use.
type Selection struct {
	ids     []int32 // in the order picked
	buckets int
}

// Add puts id in the selection.
func (s *Selection) Add(id int32) {
	if slices.Contains(s.ids, id) {
		return
	}
	s.ids = append(s.ids, id)
	s.buckets = max(s.buckets, selectionBuckets)
	for len(s.ids) >= s.buckets-s.buckets/4 {
		s.buckets *= 2
	}
}

// Remove takes id out of the selection.
func (s *Selection) Remove(id int32) {
	s.ids = slices.DeleteFunc(s.ids, func(have int32) bool { return have == id })
}

// Clear empties the selection.
func (s *Selection) Clear() { s.ids = nil }

// Empty reports whether nothing is selected.
func (s *Selection) Empty() bool { return len(s.ids) == 0 }

// selectionBuckets is a selection's starting hash table size.
const selectionBuckets = 16

// IDs returns the selected ids in the order the board lists them: by
// bucket of the selection's hash table (an id folded onto its own high
// half, masked to the table size), lowest first, and in the order they
// were picked within a bucket.
func (s *Selection) IDs() []int32 {
	ids := slices.Clone(s.ids)
	mask := uint32(max(s.buckets, selectionBuckets) - 1)
	bucket := func(id int32) uint32 { h := uint32(id); return (h ^ h>>16) & mask }
	slices.SortStableFunc(ids, func(a, b int32) int { return cmp.Compare(bucket(a), bucket(b)) })
	return ids
}

// contactRows lists ids as the friends board does, one link per character
// with a name, marked on or off, each followed by a line break. Every link
// runs command;action;<id>.
func contactRows(ids []int32, skip func(int32) bool, contacts Contacts, command, action, label string) string {
	var b strings.Builder
	for _, id := range ids {
		if skip != nil && skip(id) {
			continue
		}
		name, ok := contacts.Name(id)
		if !ok {
			continue
		}
		state := "(off)"
		if contacts.Online(id) {
			state = "(on)"
		}
		b.WriteString(`<a action="bypass ` + command + ";" + action + ";" + strconv.Itoa(int(id)) + `">` + label + "</a>&nbsp;" + name + " " + state + "<br1>")
	}
	return b.String()
}

// RenderFriendList fills page, the friends list, with friends not picked
// and the picked ones; confirm adds the delete-all confirmation.
func RenderFriendList(page string, friends, picked []int32, contacts Contacts, confirm bool) string {
	isPicked := func(id int32) bool { return slices.Contains(picked, id) }
	page = strings.ReplaceAll(page, "%friendslist%", contactRows(friends, isPicked, contacts, "_friend", "select", "[Select]"))
	page = strings.ReplaceAll(page, "%selectedFriendsList%", contactRows(picked, nil, contacts, "_friend", "deselect", "[Deselect]"))
	return strings.ReplaceAll(page, "%deleteMSG%", confirmation(confirm, friendDeleteAll))
}

// RenderBlockList fills page, the block list, with blocked not picked and
// the picked ones; confirm adds the delete-all confirmation.
func RenderBlockList(page string, blocked, picked []int32, contacts Contacts, confirm bool) string {
	isPicked := func(id int32) bool { return slices.Contains(picked, id) }
	page = strings.ReplaceAll(page, "%blocklist%", contactRows(blocked, isPicked, contacts, "_block", "select", "[Select]"))
	page = strings.ReplaceAll(page, "%selectedBlocksList%", contactRows(picked, nil, contacts, "_block", "deselect", "[Deselect]"))
	return strings.ReplaceAll(page, "%deleteMSG%", confirmation(confirm, blockDeleteAll))
}

// RenderFriendMail fills page, the mail form to the picked friends, with
// their names joined by ';'.
func RenderFriendMail(page string, picked []int32, contacts Contacts) string {
	names := make([]string, 0, len(picked))
	for _, id := range picked {
		if name, ok := contacts.Name(id); ok {
			names = append(names, name)
		}
	}
	return strings.ReplaceAll(page, "%list%", strings.Join(names, ";"))
}

func confirmation(show bool, text string) string {
	if show {
		return text
	}
	return ""
}
