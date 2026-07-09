package commons

// ReverseMap builds the string->E lookup table for an E->string table
// declared next to an enum's constants, so the two stay in sync by
// construction instead of being maintained as two separate literals.
func ReverseMap[E comparable](m map[E]string) map[string]E {
	names := make(map[string]E, len(m))
	for e, s := range m {
		names[s] = e
	}
	return names
}

// NameIndex builds the string->E lookup table for an enum whose values are
// small sequential ordinals into names (names[i] is the canonical spelling
// of E(i)), so the reverse lookup stays in sync with names by construction
// instead of being maintained as a separate literal.
func NameIndex[E ~uint8 | ~int](names []string) map[string]E {
	m := make(map[string]E, len(names))
	for i, name := range names {
		m[name] = E(i)
	}
	return m
}
