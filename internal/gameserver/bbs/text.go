package bbs

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Text lengths and cuts on the board count UTF-16 code units, the unit the
// client and the stored limits use.

// trimOr cuts s to max UTF-16 code units, or returns fallback for an empty
// s.
func trimOr(s string, max int, fallback string) string {
	if s == "" {
		return fallback
	}
	return commons.UTF16Cut(s, max)
}

// ellipsize cuts s longer than max UTF-16 code units to max-3 of them and
// appends "...".
func ellipsize(s string, max int) string {
	if commons.UTF16Len(s) > max {
		return commons.UTF16Cut(s, max-3) + "..."
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
