package fleet

import (
	"fmt"
	"strconv"
	"strings"
)

// The loop-file subset of TOML: top-level `key = value`, `[[steps]]`,
// `[[steps.done]]`, `[[steps.next]]` and `[steps.params]`; values are
// single-line basic strings, integers, booleans and single-line arrays
// of strings. Anything else is an error naming its line, so a loop
// author never gets a silently different reading from a full parser.

type tomlValue struct {
	v    any // string | int | bool | []string
	line int
}

type tomlTable map[string]tomlValue

type tomlStep struct {
	line   int
	keys   tomlTable
	params tomlTable
	done   []tomlTable
	next   []tomlTable
}

type loopTOML struct {
	top   tomlTable
	steps []tomlStep
}

func parseLoopTOML(b []byte) (*loopTOML, error) {
	doc := &loopTOML{top: tomlTable{}}
	cur := doc.top
	for i, raw := range strings.Split(string(b), "\n") {
		n := i + 1
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			t, err := doc.openTable(line, n)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", n, err)
			}
			cur = t
			continue
		}
		key, val, err := parseTOMLKeyValue(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if prev, ok := cur[key]; ok {
			return nil, fmt.Errorf("line %d: duplicate key %q (first set on line %d)", n, key, prev.line)
		}
		cur[key] = tomlValue{v: val, line: n}
	}
	return doc, nil
}

func (doc *loopTOML) openTable(line string, n int) (tomlTable, error) {
	header := line
	if i := strings.Index(line, "#"); i >= 0 {
		header = strings.TrimSpace(line[:i])
	}
	if header == "[[steps]]" {
		doc.steps = append(doc.steps, tomlStep{line: n, keys: tomlTable{}})
		return doc.steps[len(doc.steps)-1].keys, nil
	}
	switch header {
	case "[[steps.done]]", "[[steps.next]]", "[steps.params]":
	default:
		return nil, fmt.Errorf("unsupported table %s (loop files allow [[steps]], [[steps.done]], [[steps.next]], [steps.params])", header)
	}
	if len(doc.steps) == 0 {
		return nil, fmt.Errorf("%s before any [[steps]]", header)
	}
	s := &doc.steps[len(doc.steps)-1]
	t := tomlTable{}
	switch header {
	case "[[steps.done]]":
		s.done = append(s.done, t)
	case "[[steps.next]]":
		s.next = append(s.next, t)
	default:
		if s.params != nil {
			return nil, fmt.Errorf("duplicate [steps.params] in one step")
		}
		s.params = t
	}
	return t, nil
}

func parseTOMLKeyValue(line string) (string, any, error) {
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return "", nil, fmt.Errorf("expected key = value")
	}
	key := strings.TrimSpace(line[:eq])
	if !isBareTOMLKey(key) {
		return "", nil, fmt.Errorf("unsupported key %q (bare keys only)", key)
	}
	val, rest, err := parseTOMLValue(strings.TrimSpace(line[eq+1:]))
	if err != nil {
		return "", nil, fmt.Errorf("key %q: %w", key, err)
	}
	if rest = strings.TrimSpace(rest); rest != "" && rest[0] != '#' {
		return "", nil, fmt.Errorf("key %q: unexpected %q after value", key, rest)
	}
	return key, val, nil
}

func isBareTOMLKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !isNameRune(r) {
			return false
		}
	}
	return true
}

func isNameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return r == '_' || r == '-'
}

// parseTOMLValue reads one value from the start of s and returns the
// unread remainder.
func parseTOMLValue(s string) (any, string, error) {
	switch {
	case s == "":
		return nil, "", fmt.Errorf("missing value")
	case strings.HasPrefix(s, `"""`):
		return nil, "", fmt.Errorf("multi-line strings are not supported")
	case s[0] == '"':
		return parseTOMLString(s)
	case s[0] == '\'':
		return nil, "", fmt.Errorf("literal strings are not supported; use double quotes")
	case s[0] == '{':
		return nil, "", fmt.Errorf("inline tables are not supported")
	case s[0] == '[':
		return parseTOMLStringArray(s)
	}
	end := strings.IndexAny(s, " \t#,]")
	if end < 0 {
		end = len(s)
	}
	word, rest := s[:end], s[end:]
	switch word {
	case "true":
		return true, rest, nil
	case "false":
		return false, rest, nil
	}
	if n, err := strconv.Atoi(word); err == nil {
		return n, rest, nil
	}
	return nil, "", fmt.Errorf("unsupported value %q (strings, integers, booleans, string arrays only)", word)
}

func parseTOMLString(s string) (string, string, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			return b.String(), s[i+1:], nil
		case '\\':
			i++
			if i == len(s) {
				return "", "", fmt.Errorf("unterminated string")
			}
			switch s[i] {
			case '"', '\\':
				b.WriteByte(s[i])
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				return "", "", fmt.Errorf("unsupported escape \\%c", s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", "", fmt.Errorf("unterminated string")
}

func parseTOMLStringArray(s string) ([]string, string, error) {
	out := []string{}
	rest := strings.TrimSpace(s[1:])
	for {
		if rest == "" || rest[0] == '#' {
			return nil, "", fmt.Errorf("unterminated array (arrays must fit on one line)")
		}
		if rest[0] == ']' {
			return out, rest[1:], nil
		}
		if rest[0] != '"' {
			return nil, "", fmt.Errorf("arrays hold strings only")
		}
		str, r, err := parseTOMLString(rest)
		if err != nil {
			return nil, "", err
		}
		out = append(out, str)
		rest = strings.TrimSpace(r)
		switch {
		case strings.HasPrefix(rest, ","):
			rest = strings.TrimSpace(rest[1:])
		case strings.HasPrefix(rest, "]"):
		default:
			return nil, "", fmt.Errorf("expected , or ] in array")
		}
	}
}
