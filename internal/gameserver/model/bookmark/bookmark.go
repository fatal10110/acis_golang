// Package bookmark keeps the named spots a game master saves to teleport
// back to.
package bookmark

import (
	"errors"
	"strings"
	"unicode/utf16"
)

// MaxNameLength is the longest name a bookmark takes, in UTF-16 units.
const MaxNameLength = 15

// Bookmark is a named spot.
type Bookmark struct {
	Name    string
	X, Y, Z int32
}

// Refusals of a book change.
var (
	ErrNameTooLong = errors.New("bookmark: name too long")
	ErrExists      = errors.New("bookmark: name already exists")
	ErrMissing     = errors.New("bookmark: no such name")
)

// Book is one owner's bookmarks, in the order listed: the ones read back
// first, then the ones added since. Names match case-insensitively. The
// zero value is an empty book.
type Book struct {
	marks []Bookmark
}

// NewBook returns a book of marks, in their order.
func NewBook(marks []Bookmark) *Book {
	return &Book{marks: marks}
}

// All returns the book's bookmarks in order. The slice is the book's own.
func (b *Book) All() []Bookmark { return b.marks }

// Add appends m, refusing a name longer than MaxNameLength or one the book
// holds already.
func (b *Book) Add(m Bookmark) error {
	if len(utf16.Encode([]rune(m.Name))) > MaxNameLength {
		return ErrNameTooLong
	}
	if b.find(m.Name) >= 0 {
		return ErrExists
	}
	b.marks = append(b.marks, m)
	return nil
}

// Remove deletes the bookmark called name, refusing a name the book does
// not hold.
func (b *Book) Remove(name string) error {
	i := b.find(name)
	if i < 0 {
		return ErrMissing
	}
	b.marks = append(b.marks[:i], b.marks[i+1:]...)
	return nil
}

func (b *Book) find(name string) int {
	for i, m := range b.marks {
		if strings.EqualFold(m.Name, name) {
			return i
		}
	}
	return -1
}
