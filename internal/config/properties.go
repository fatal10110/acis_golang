package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DefaultDelimiters is the split pattern used by the legacy typed array getters.
const DefaultDelimiters = `[\s,;]+`

// ShippedFiles is the config/*.properties set expected by the current server.
var ShippedFiles = []string{
	"banned_ips.properties",
	"clans.properties",
	"events.properties",
	"geoengine.properties",
	"logging.properties",
	"loginserver.properties",
	"npcs.properties",
	"players.properties",
	"server.properties",
	"siege.properties",
}

var defaultDelimitersRE = regexp.MustCompile(DefaultDelimiters)

// Properties holds one parsed .properties file.
type Properties struct {
	values map[string]string
}

// FileSet holds the shipped config files keyed by basename.
type FileSet struct {
	Files map[string]*Properties
}

// KeyRef identifies a key in a config file.
type KeyRef struct {
	File string
	Key  string
}

// CountMismatch reports a loaded key count that differs from an expected count.
type CountMismatch struct {
	File string
	Got  int
	Want int
}

// IntPair is a pair parsed from values shaped like "57-100;6651-3".
type IntPair struct {
	First  int
	Second int
}

// Parse reads .properties content: key=value lines (also accepting ':' or
// whitespace as the separator), '#'/'!' comment lines, backslash line
// continuations, and '\t'/'\n'/'\r'/'\f'/'\uXXXX' escapes in keys and
// values.
func Parse(r io.Reader) (*Properties, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	lines := logicalLines(string(data))
	p := &Properties{values: make(map[string]string)}
	for i, line := range lines {
		line = strings.TrimLeftFunc(line, isSpaceRune)
		if line == "" || line[0] == '#' || line[0] == '!' {
			continue
		}
		key, value, err := splitProperty(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		p.values[key] = value
	}
	return p, nil
}

// ParseString reads .properties content (see Parse) from a string.
func ParseString(s string) (*Properties, error) {
	return Parse(strings.NewReader(s))
}

// LoadFile reads one .properties file.
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

// LoadDirectory reads the ten shipped config files from dir and rejects unknown .properties files.
func LoadDirectory(dir string) (*FileSet, error) {
	known := make(map[string]bool, len(ShippedFiles))
	for _, name := range ShippedFiles {
		known[name] = true
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.properties"))
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		if name := filepath.Base(match); !known[name] {
			return nil, fmt.Errorf("unsupported config file %s", name)
		}
	}

	set := &FileSet{Files: make(map[string]*Properties, len(ShippedFiles))}
	for _, name := range ShippedFiles {
		p, err := LoadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		set.Files[name] = p
	}
	return set, nil
}

// CountMismatches compares loaded key counts with expected counts.
func (s *FileSet) CountMismatches(expected map[string]int) []CountMismatch {
	var out []CountMismatch
	for file, want := range expected {
		got := 0
		if p := s.Files[file]; p != nil {
			got = p.Len()
		}
		if got != want {
			out = append(out, CountMismatch{File: file, Got: got, Want: want})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out
}

// UnknownKeys returns loaded keys not present in the provided supported-key list.
func (s *FileSet) UnknownKeys(supported map[string][]string) []KeyRef {
	var out []KeyRef
	for file, p := range s.Files {
		allowed := make(map[string]bool, len(supported[file]))
		for _, key := range supported[file] {
			allowed[key] = true
		}
		for _, key := range p.Keys() {
			if !allowed[key] {
				out = append(out, KeyRef{File: file, Key: key})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File == out[j].File {
			return out[i].Key < out[j].Key
		}
		return out[i].File < out[j].File
	})
	return out
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

// String returns a string property or def when key is missing.
func (p *Properties) String(key, def string) string {
	if value, ok := p.Lookup(key); ok {
		return value
	}
	return def
}

// Bool returns a bool property or def when key is missing.
func (p *Properties) Bool(key string, def bool) bool {
	if value, ok := p.Lookup(key); ok {
		return strings.EqualFold(value, "true")
	}
	return def
}

// Int returns an int property or def when key is missing.
func (p *Properties) Int(key string, def int) (int, error) {
	if value, ok := p.Lookup(key); ok {
		n, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("parse %s as int: %w", key, err)
		}
		return n, nil
	}
	return def, nil
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
	return def, nil
}

// Float64 returns a float64 property or def when key is missing.
func (p *Properties) Float64(key string, def float64) (float64, error) {
	if value, ok := p.Lookup(key); ok {
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %s as float64: %w", key, err)
		}
		return n, nil
	}
	return def, nil
}

// Strings returns a string slice split with DefaultDelimiters, or def when key is missing.
func (p *Properties) Strings(key string, def []string) []string {
	if value, ok := p.Lookup(key); ok {
		return splitTrimTrailingEmpty(defaultDelimitersRE, value)
	}
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
	return def
}

// Ints returns an int slice split with DefaultDelimiters, or def when key is missing.
func (p *Properties) Ints(key string, def []int) ([]int, error) {
	if value, ok := p.Lookup(key); ok {
		parts := splitTrimTrailingEmpty(defaultDelimitersRE, value)
		out := make([]int, len(parts))
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("parse %s[%d] as int: %w", key, i, err)
			}
			out[i] = n
		}
		return out, nil
	}
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
	return def, nil
}

// Float64s returns a float64 slice split with DefaultDelimiters, or def when key is missing.
func (p *Properties) Float64s(key string, def []float64) ([]float64, error) {
	if value, ok := p.Lookup(key); ok {
		parts := splitTrimTrailingEmpty(defaultDelimitersRE, value)
		out := make([]float64, len(parts))
		for i, part := range parts {
			n, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return nil, fmt.Errorf("parse %s[%d] as float64: %w", key, i, err)
			}
			out[i] = n
		}
		return out, nil
	}
	return def, nil
}

// IntPairs returns pairs parsed from values shaped like "57-100;6651-3".
func (p *Properties) IntPairs(key, def string) ([]IntPair, error) {
	value := def
	if found, ok := p.Lookup(key); ok {
		value = found
	}
	if value == "" {
		return nil, nil
	}

	parts := splitLiteralTrimTrailingEmpty(value, ";")
	out := make([]IntPair, len(parts))
	for i, part := range parts {
		bounds := splitLiteralTrimTrailingEmpty(part, "-")
		if len(bounds) != 2 {
			return nil, fmt.Errorf("parse %s[%d]: want first-second", key, i)
		}
		first, err := strconv.Atoi(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("parse %s[%d] first: %w", key, i, err)
		}
		second, err := strconv.Atoi(bounds[1])
		if err != nil {
			return nil, fmt.Errorf("parse %s[%d] second: %w", key, i, err)
		}
		out[i] = IntPair{First: first, Second: second}
	}
	return out, nil
}

func logicalLines(data string) []string {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")

	var lines []string
	var current strings.Builder
	continued := false
	for _, line := range strings.Split(data, "\n") {
		if continued {
			line = strings.TrimLeftFunc(line, isSpaceRune)
		}
		if hasContinuation(line) {
			current.WriteString(line[:len(line)-1])
			continued = true
			continue
		}
		current.WriteString(line)
		lines = append(lines, current.String())
		current.Reset()
		continued = false
	}
	if continued {
		lines = append(lines, current.String())
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
			n, err := strconv.ParseInt(s[i+1:i+5], 16, 32)
			if err != nil {
				return "", fmt.Errorf("bad unicode escape %q", s[i+1:i+5])
			}
			out.WriteRune(rune(n))
			i += 4
		default:
			out.WriteByte(s[i])
		}
	}
	return out.String(), nil
}

func splitTrimTrailingEmpty(re *regexp.Regexp, s string) []string {
	return trimTrailingEmpty(re.Split(s, -1))
}

// splitLiteralTrimTrailingEmpty is splitTrimTrailingEmpty for a plain,
// literal-character separator that doesn't need a regex.
func splitLiteralTrimTrailingEmpty(s, sep string) []string {
	return trimTrailingEmpty(strings.Split(s, sep))
}

func trimTrailingEmpty(parts []string) []string {
	for len(parts) > 1 && parts[len(parts)-1] == "" {
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
