package bbs

import (
	"strings"
	"unicode/utf16"
)

// Text lengths and cuts on the board count UTF-16 code units, the unit the
// client and the stored limits use.

// textLen is s's length in UTF-16 code units.
func textLen(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// cut returns the first n UTF-16 code units of s, or s when it is no
// longer. A character that would straddle the cut is left out.
func cut(s string, n int) string {
	used := 0
	for i, r := range s {
		w := utf16.RuneLen(r)
		if used+w > n {
			return s[:i]
		}
		used += w
	}
	return s
}

// trimOr cuts s to max UTF-16 code units, or returns fallback for an empty
// s.
func trimOr(s string, max int, fallback string) string {
	if s == "" {
		return fallback
	}
	return cut(s, max)
}

// ellipsize cuts s longer than max UTF-16 code units to max-3 of them and
// appends "...".
func ellipsize(s string, max int) string {
	if textLen(s) > max {
		return cut(s, max-3) + "..."
	}
	return s
}

// splitList splits s on sep and drops the trailing empty fields, so a list
// ending in sep, or made of nothing but sep, loses those fields; an empty
// s is one empty field.
func splitList(s, sep string) []string {
	if s == "" {
		return []string{""}
	}
	fields := strings.Split(s, sep)
	for len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	return fields
}

// Tokens splits a board command or argument on any of the characters in
// sep, dropping the empty fields between consecutive separators.
func Tokens(s, sep string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(sep, r) })
}
