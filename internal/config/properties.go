package config

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/rs/zerolog"
)

// warnLog receives the missing-key and malformed-value warnings raised while
// reading properties. It starts as an unconfigured stderr logger so tools
// that never build one still report, and SetLogger swaps in the process
// logger so the warnings reach its configured sinks and levels.
var warnLog atomic.Pointer[zerolog.Logger]

// SetLogger routes config warnings to l. A composition root calls it once,
// as soon as its logger exists and before it reads any property value.
func SetLogger(l zerolog.Logger) {
	warnLog.Store(&l)
}

func logger() *zerolog.Logger {
	if l := warnLog.Load(); l != nil {
		return l
	}
	return &defaultWarnLog
}

var defaultWarnLog = zerolog.New(os.Stderr).With().Timestamp().Logger()

// DefaultDelimiters is the split pattern used by the legacy typed array
// getters. Its whitespace class includes the vertical tab, which RE2's \s
// leaves out.
const DefaultDelimiters = `[\s\x0B,;]+`

var defaultDelimitersRE = regexp.MustCompile(DefaultDelimiters)

// Properties holds one parsed .properties file.
type Properties struct {
	values map[string]string
}

// IntPair is a pair parsed from values shaped like "57-100;6651-3". Both
// numbers fit an int32: a number outside it is malformed.
type IntPair struct {
	First  int
	Second int
}

// Parse reads .properties content: key=value lines (also accepting ':' or
// whitespace as the separator), '#'/'!' comment lines, backslash line
// continuations, and '\t'/'\n'/'\r'/'\f'/'\uXXXX' escapes in keys and
// values. Each input byte is one ISO-8859-1 character, so a non-ASCII value
// is reachable only through a \uXXXX escape; an escaped UTF-16 surrogate
// pair reads as its one character.
func Parse(r io.Reader) (*Properties, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	p := &Properties{values: make(map[string]string)}
	for _, line := range logicalLines(decodeLatin1(data)) {
		key, value, err := splitProperty(line.text)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line.number, err)
		}
		p.values[key] = value
	}
	return p, nil
}

// ParseString reads .properties content (see Parse) from a string holding
// the file's bytes.
func ParseString(s string) (*Properties, error) {
	return Parse(strings.NewReader(s))
}

// LoadFile reads one .properties file. A missing or malformed file is an
// error, and the servers stop booting on it rather than carrying on with
// defaults: strict loading is kept as a parity exception (#4772).
func LoadFile(name string) (*Properties, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", name, err)
	}
	defer f.Close()

	p, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", name, err)
	}
	return p, nil
}

// Len returns the number of loaded properties.
func (p *Properties) Len() int {
	return len(p.values)
}

// Keys returns loaded property keys in sorted order.
func (p *Properties) Keys() []string {
	keys := make([]string, 0, len(p.values))
	for key := range p.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Lookup returns the raw property value.
func (p *Properties) Lookup(key string) (string, bool) {
	value, ok := p.values[key]
	return value, ok
}

func warnMissing(key string, def any) {
	logger().Warn().Str("key", key).Interface("default", def).Msg("config property missing; using default value")
}

// String returns a string property or def when key is missing.
func (p *Properties) String(key, def string) string {
	if value, ok := p.Lookup(key); ok {
		return value
	}
	warnMissing(key, def)
	return def
}

// Bool returns a bool property or def when key is missing.
func (p *Properties) Bool(key string, def bool) bool {
	if value, ok := p.Lookup(key); ok {
		return strings.EqualFold(value, "true")
	}
	warnMissing(key, def)
	return def
}

// parseInt32 parses a base-10 int that must fit in 32 bits, as every int
// config value does.
func parseInt32(s string) (int, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	return int(n), err
}

// Int returns an int property or def when key is missing. The value must
// fit in 32 bits.
func (p *Properties) Int(key string, def int) (int, error) {
	if value, ok := p.Lookup(key); ok {
		n, err := parseInt32(value)
		if err != nil {
			return 0, fmt.Errorf("parse %s as int: %w", key, err)
		}
		return n, nil
	}
	warnMissing(key, def)
	return def, nil
}

// OptionalInt returns an int property and whether the key was present. An
// absent key yields (0, false, nil) with no missing-property warning, for
// keys that are genuinely optional and absent from every shipped
// .properties file — Int would log on every boot for those.
func (p *Properties) OptionalInt(key string) (int, bool, error) {
	value, ok := p.Lookup(key)
	if !ok {
		return 0, false, nil
	}
	n, err := parseInt32(value)
	if err != nil {
		return 0, false, fmt.Errorf("parse %s as int: %w", key, err)
	}
	return n, true, nil
}

// Int64 returns an int64 property or def when key is missing.
func (p *Properties) Int64(key string, def int64) (int64, error) {
	if value, ok := p.Lookup(key); ok {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %s as int64: %w", key, err)
		}
		return n, nil
	}
	warnMissing(key, def)
	return def, nil
}

// Float64 returns a float64 property or def when key is missing. The value
// follows the parseDouble literal grammar.
func (p *Properties) Float64(key string, def float64) (float64, error) {
	if value, ok := p.Lookup(key); ok {
		n, err := parseDouble(value)
		if err != nil {
			return 0, fmt.Errorf("parse %s as float64: %w", key, err)
		}
		return n, nil
	}
	warnMissing(key, def)
	return def, nil
}

// Strings returns a string slice split with DefaultDelimiters, or def when key is missing.
func (p *Properties) Strings(key string, def []string) []string {
	if value, ok := p.Lookup(key); ok {
		return splitTrimTrailingEmpty(defaultDelimitersRE, value)
	}
	warnMissing(key, def)
	return def
}

// Bools returns a bool slice split with DefaultDelimiters, or def when key is missing.
func (p *Properties) Bools(key string, def []bool) []bool {
	if value, ok := p.Lookup(key); ok {
		parts := splitTrimTrailingEmpty(defaultDelimitersRE, value)
		out := make([]bool, len(parts))
		for i, part := range parts {
			out[i] = strings.EqualFold(part, "true")
		}
		return out
	}
	warnMissing(key, def)
	return def
}

// Ints returns an int slice split with DefaultDelimiters, or def when key is missing.
func (p *Properties) Ints(key string, def []int) ([]int, error) {
	if value, ok := p.Lookup(key); ok {
		parts := splitTrimTrailingEmpty(defaultDelimitersRE, value)
		out := make([]int, len(parts))
		for i, part := range parts {
			n, err := parseInt32(part)
			if err != nil {
				return nil, fmt.Errorf("parse %s[%d] as int: %w", key, i, err)
			}
			out[i] = n
		}
		return out, nil
	}
	warnMissing(key, def)
	return def, nil
}

// Int64s returns an int64 slice split with DefaultDelimiters, or def when key is missing.
func (p *Properties) Int64s(key string, def []int64) ([]int64, error) {
	if value, ok := p.Lookup(key); ok {
		parts := splitTrimTrailingEmpty(defaultDelimitersRE, value)
		out := make([]int64, len(parts))
		for i, part := range parts {
			n, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse %s[%d] as int64: %w", key, i, err)
			}
			out[i] = n
		}
		return out, nil
	}
	warnMissing(key, def)
	return def, nil
}

// Float64s returns a float64 slice split with DefaultDelimiters, or def when
// key is missing. Each element follows the parseDouble literal grammar.
func (p *Properties) Float64s(key string, def []float64) ([]float64, error) {
	if value, ok := p.Lookup(key); ok {
		parts := splitTrimTrailingEmpty(defaultDelimitersRE, value)
		out := make([]float64, len(parts))
		for i, part := range parts {
			n, err := parseDouble(part)
			if err != nil {
				return nil, fmt.Errorf("parse %s[%d] as float64: %w", key, i, err)
			}
			out[i] = n
		}
		return out, nil
	}
	warnMissing(key, def)
	return def, nil
}

// IntPairs returns pairs parsed from a value shaped like "57-100;6651-3",
// or def parsed the same way when key is missing; a missing key is not
// logged. Entries and numbers are taken as written, untrimmed. A malformed
// entry — not exactly two '-'-separated parts, or a number that is not an
// int32 — logs and returns an empty list.
func (p *Properties) IntPairs(key, def string) []IntPair {
	value, ok := p.Lookup(key)
	if !ok {
		value = def
	}
	entries := splitLiteralTrimTrailingEmpty(value, ";")
	if len(entries) == 0 {
		return nil
	}
	out := make([]IntPair, len(entries))
	for i, entry := range entries {
		bounds := splitLiteralTrimTrailingEmpty(entry, "-")
		if len(bounds) != 2 {
			logger().Warn().Str("key", key).Str("entry", entry).Msg("config item pair malformed, want first-second; using empty list")
			return nil
		}
		pair, err := parseIntPair(bounds)
		if err != nil {
			logger().Error().Err(err).Str("key", key).Msg("config item pair has non-numeric value; using empty list")
			return nil
		}
		out[i] = pair
	}
	return out
}

// IntPairsComma returns pairs parsed from a comma-separated list of
// first-second entries, or def parsed the same way when key is missing.
// Entries and numbers are taken as written, untrimmed, and parts after the
// second are ignored. An entry with fewer than two parts or a number that
// is not an int32 is an error: this form backs config keys whose readers
// let a bad value abort loading.
func (p *Properties) IntPairsComma(key, def string) ([]IntPair, error) {
	value, ok := p.Lookup(key)
	if !ok {
		warnMissing(key, def)
		value = def
	}
	entries := splitLiteralTrimTrailingEmpty(value, ",")
	out := make([]IntPair, len(entries))
	for i, entry := range entries {
		bounds := splitLiteralTrimTrailingEmpty(entry, "-")
		if len(bounds) < 2 {
			return nil, fmt.Errorf("parse %s[%d] %q: want first-second", key, i, entry)
		}
		pair, err := parseIntPair(bounds[:2])
		if err != nil {
			return nil, fmt.Errorf("parse %s[%d]: %w", key, i, err)
		}
		out[i] = pair
	}
	return out, nil
}

func parseIntPair(bounds []string) (IntPair, error) {
	first, err := parseInt32(bounds[0])
	if err != nil {
		return IntPair{}, fmt.Errorf("first: %w", err)
	}
	second, err := parseInt32(bounds[1])
	if err != nil {
		return IntPair{}, fmt.Errorf("second: %w", err)
	}
	return IntPair{First: first, Second: second}, nil
}

// decodeLatin1 reads every byte of data as the ISO-8859-1 character of the
// same value.
func decodeLatin1(data []byte) string {
	var out strings.Builder
	out.Grow(len(data))
	for _, b := range data {
		out.WriteRune(rune(b))
	}
	return out.String()
}

// logicalLine is one key/value entry with its continuations joined and
// leading whitespace removed; number is the 1-based line it starts on.
type logicalLine struct {
	number int
	text   string
}

// logicalLines joins backslash continuations and drops blank and comment
// lines. A '#' or '!' starts a comment only where it is the first
// character of an entry, so a comment never continues onto the next line
// and a continued line starting with one is ordinary text.
func logicalLines(data string) []logicalLine {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")

	var lines []logicalLine
	var current strings.Builder
	start := 0
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimLeftFunc(line, isSpaceRune)
		if current.Len() == 0 {
			start = i + 1
			if line != "" && (line[0] == '#' || line[0] == '!') {
				continue
			}
		}
		if hasContinuation(line) {
			current.WriteString(line[:len(line)-1])
			continue
		}
		current.WriteString(line)
		if current.Len() > 0 {
			lines = append(lines, logicalLine{number: start, text: current.String()})
		}
		current.Reset()
	}
	if current.Len() > 0 {
		lines = append(lines, logicalLine{number: start, text: current.String()})
	}
	return lines
}

func hasContinuation(line string) bool {
	slashes := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		slashes++
	}
	return slashes%2 == 1
}

func splitProperty(line string) (string, string, error) {
	sep := -1
	spaceSep := false
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '=' || c == ':' {
			sep = i
			break
		}
		if isSpaceByte(c) {
			sep = i
			spaceSep = true
			break
		}
	}

	keyText := line
	valueText := ""
	if sep >= 0 {
		keyText = line[:sep]
		i := sep
		if spaceSep {
			for i < len(line) && isSpaceByte(line[i]) {
				i++
			}
			if i < len(line) && (line[i] == '=' || line[i] == ':') {
				i++
			}
		} else {
			i++
		}
		for i < len(line) && isSpaceByte(line[i]) {
			i++
		}
		valueText = line[i:]
	}

	key, err := unescape(keyText)
	if err != nil {
		return "", "", fmt.Errorf("key: %w", err)
	}
	value, err := unescape(valueText)
	if err != nil {
		return "", "", fmt.Errorf("value: %w", err)
	}
	return key, value, nil
}

func unescape(s string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			out.WriteByte('\\')
			break
		}
		switch s[i] {
		case 't':
			out.WriteByte('\t')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			if i+4 >= len(s) {
				return "", fmt.Errorf("short unicode escape")
			}
			r, ok := hexUnit(s[i+1 : i+5])
			if !ok {
				return "", fmt.Errorf("bad unicode escape %q", s[i+1:i+5])
			}
			i += 4
			// A high surrogate escape followed by a low one is a single
			// character; an unpaired surrogate has no UTF-8 form and
			// reads as U+FFFD.
			if utf16.IsSurrogate(r) && i+6 < len(s) && s[i+1] == '\\' && s[i+2] == 'u' {
				if low, ok := hexUnit(s[i+3 : i+7]); ok {
					if pair := utf16.DecodeRune(r, low); pair != utf8.RuneError {
						r = pair
						i += 6
					}
				}
			}
			out.WriteRune(r)
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String(), nil
}

// hexUnit reads exactly four hex digits, with no sign or prefix, as one
// UTF-16 code unit.
func hexUnit(s string) (rune, bool) {
	var r rune
	for i := range len(s) {
		c := s[i]
		switch {
		case '0' <= c && c <= '9':
			r = r<<4 | rune(c-'0')
		case 'a' <= c && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case 'A' <= c && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}

func splitTrimTrailingEmpty(re *regexp.Regexp, s string) []string {
	return trimTrailingEmpty(re.Split(s, -1))
}

// splitLiteralTrimTrailingEmpty is splitTrimTrailingEmpty for a plain,
// literal-character separator that doesn't need a regex.
func splitLiteralTrimTrailingEmpty(s, sep string) []string {
	return trimTrailingEmpty(strings.Split(s, sep))
}

// trimTrailingEmpty gives a split result the shape the config format
// expects: an input the separator never matched stays one element, even
// when empty, and otherwise every trailing empty element is dropped, so a
// value made only of separators has none.
func trimTrailingEmpty(parts []string) []string {
	if len(parts) == 1 {
		return parts
	}
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func isSpaceRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\f'
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\f'
}
