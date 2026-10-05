package scriptfp

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokKind uint8

const (
	tokIdent tokKind = iota + 1
	tokInt
	tokFloat
	tokString
	tokChar
	tokOp
)

// javaToken is one lexical javaToken of a Java source file. For literals, text is
// the decoded value: a canonical decimal number, or the string or character
// itself.
type javaToken struct {
	kind tokKind
	text string
	line int
}

func (t javaToken) is(op string) bool { return t.kind == tokOp && t.text == op }

func (t javaToken) ident(name string) bool { return t.kind == tokIdent && t.text == name }

// lexJava splits Java source into tokens, dropping comments and white space.
// Operators are single characters except "->", "::" and "...", so generic
// closers such as ">>" stay separate tokens.
func lexJava(src string) ([]javaToken, error) {
	var toks []javaToken
	line := 1
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			i++
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("line %d: unterminated comment", line)
			}
			line += strings.Count(src[i:i+2+end], "\n")
			i += end + 4
		case strings.HasPrefix(src[i:], `"""`):
			return nil, fmt.Errorf("line %d: text blocks are not supported", line)
		case c == '"' || c == '\'':
			value, n, err := javaQuoted(src[i:])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			kind := tokString
			if c == '\'' {
				kind = tokChar
			}
			toks = append(toks, javaToken{kind, value, line})
			i += n
		case isDigit(c) || c == '.' && i+1 < len(src) && isDigit(src[i+1]):
			kind, value, n, err := javaNumber(src[i:])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			toks = append(toks, javaToken{kind, value, line})
			i += n
		case c == '_' || c == '$' || c >= utf8.RuneSelf || unicode.IsLetter(rune(c)):
			j := i
			for j < len(src) {
				r, size := utf8.DecodeRuneInString(src[j:])
				if r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
					break
				}
				j += size
			}
			if j == i {
				return nil, fmt.Errorf("line %d: unexpected character %q", line, src[i:i+1])
			}
			toks = append(toks, javaToken{tokIdent, src[i:j], line})
			i = j
		default:
			op := src[i : i+1]
			for _, long := range []string{"->", "::", "..."} {
				if strings.HasPrefix(src[i:], long) {
					op = long
					break
				}
			}
			toks = append(toks, javaToken{tokOp, op, line})
			i += len(op)
		}
	}
	return toks, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// javaQuoted decodes the string or character literal at the start of s and
// returns its value and length in s.
func javaQuoted(s string) (string, int, error) {
	quote := s[0]
	var b strings.Builder
	for i := 1; i < len(s); {
		c := s[i]
		switch {
		case c == quote:
			return b.String(), i + 1, nil
		case c == '\n':
			return "", 0, fmt.Errorf("unterminated literal")
		case c != '\\':
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(s) {
			break
		}
		e := s[i+1]
		i += 2
		switch e {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 's':
			b.WriteByte(' ')
		case '"', '\'', '\\':
			b.WriteByte(e)
		case 'u':
			for i < len(s) && s[i] == 'u' {
				i++
			}
			if i+4 > len(s) {
				return "", 0, fmt.Errorf("short unicode escape")
			}
			r, err := strconv.ParseUint(s[i:i+4], 16, 16)
			if err != nil {
				return "", 0, fmt.Errorf("bad unicode escape %q", s[i:i+4])
			}
			b.WriteRune(rune(r))
			i += 4
		default:
			if e < '0' || e > '7' {
				return "", 0, fmt.Errorf("bad escape \\%c", e)
			}
			v := int(e - '0')
			for n := 1; n < 3 && i < len(s) && s[i] >= '0' && s[i] <= '7' && v*8+int(s[i]-'0') <= 0o377; n++ {
				v = v*8 + int(s[i]-'0')
				i++
			}
			b.WriteRune(rune(v))
		}
	}
	return "", 0, fmt.Errorf("unterminated literal")
}

// javaNumber scans the numeric literal at the start of s.
func javaNumber(s string) (tokKind, string, int, error) {
	n := 0
	isHex := strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X")
	isBin := strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B")
	if isHex || isBin {
		n = 2
	}
	float := false
	for n < len(s) {
		c := s[n]
		switch {
		case isDigit(c) || c == '_' || isHex && strings.IndexByte("abcdefABCDEF", c) >= 0:
		case c == '.' && !isHex && !isBin && !float:
			float = true
		case (c == 'e' || c == 'E') && !isHex && !isBin:
			float = true
			if n+1 < len(s) && (s[n+1] == '+' || s[n+1] == '-') {
				n++
			}
		default:
			goto done
		}
		n++
	}
done:
	text := strings.ReplaceAll(s[:n], "_", "")
	if n < len(s) {
		switch s[n] {
		case 'l', 'L':
			n++
		case 'f', 'F', 'd', 'D':
			if !isHex {
				float = true
				n++
			}
		}
	}
	if n < len(s) && (isIdentByte(s[n]) || s[n] == '.') {
		return 0, "", 0, fmt.Errorf("malformed number %q", s[:n+1])
	}
	if float {
		v, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, "", 0, fmt.Errorf("malformed number %q: %w", s[:n], err)
		}
		return tokFloat, canonicalFloat(v), n, nil
	}
	base := 10
	digits := text
	switch {
	case isHex:
		base, digits = 16, text[2:]
	case isBin:
		base, digits = 2, text[2:]
	case len(text) > 1 && text[0] == '0':
		base, digits = 8, text[1:]
	}
	v, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, "", 0, fmt.Errorf("malformed number %q: %w", s[:n], err)
	}
	return tokInt, strconv.FormatUint(v, 10), n, nil
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || isDigit(c) || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// canonicalFloat renders a floating-point literal value the same way for
// both languages: an integral value as an integer, any other as the
// shortest decimal that parses back to it.
func canonicalFloat(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
