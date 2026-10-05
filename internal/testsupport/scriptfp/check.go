package scriptfp

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// helperOwner is the reference class whose calls form the helper surface
// a port must not add calls to: a Go call mapped from one of its rows is
// reported when none of the ported classes makes that call.
const helperOwner = "Quest"

// Check compares the Go script package at dir with the committed reference
// fingerprints of classes (script keys, such as "quest.Q001_LettersOfLove";
// a family package names every class it ports) and returns one problem per
// difference, sorted:
//
//   - a distinctive literal (number, string, char) on one side only;
//   - an engine call of the classes with no Go name in apimap.txt, or whose
//     Go name the package never calls; a Go call mapped from a Quest helper
//     that none of the classes makes;
//   - a hook (on* override) whose name or parent-call shape differs: the
//     Go hook is the OnX field of a Hooks literal, its parent call a call
//     to X on any receiver but the hook's first parameter.
//
// exempt lists problem keys to accept, written as the problem starts:
// `number 906`, `string "x"`, `char "x"`, `call Quest.giveItems`,
// `hook onTalk none`. An exemption that matches nothing is reported.
//
// Test files of the package are not read.
func Check(dir string, classes []string, exempt ...string) ([]string, error) {
	ref, err := Reference()
	if err != nil {
		return nil, err
	}
	amap, err := APIMap()
	if err != nil {
		return nil, err
	}
	return check(ref, amap, dir, classes, exempt)
}

func check(ref map[string]Fingerprint, amap map[string]string, dir string, classes, exempt []string) ([]string, error) {
	if len(classes) == 0 {
		return nil, fmt.Errorf("scriptfp: Check needs at least one class")
	}
	pkg, err := readGoPackage(dir)
	if err != nil {
		return nil, err
	}

	numbers, strs, chars := map[string]bool{}, map[string]bool{}, map[string]bool{}
	calls := map[string]bool{}
	var hooks []string
	for _, class := range classes {
		fp, ok := ref[class]
		if !ok {
			return nil, fmt.Errorf("scriptfp: no reference fingerprint for %s", class)
		}
		for _, n := range fp.Numbers {
			numbers[n] = true
		}
		for _, s := range fp.Strings {
			strs[s] = true
		}
		for _, c := range fp.Chars {
			chars[c] = true
		}
		for row := range fp.Calls {
			calls[row] = true
		}
		for _, h := range fp.Hooks {
			if isHookName(h.Name) {
				hooks = append(hooks, h.String())
			}
		}
	}

	var keys []string // problem key + "\x00" + message
	add := func(key, msg string) { keys = append(keys, key+"\x00"+msg) }
	diffSets := func(kind string, want, got map[string]bool, quote bool) {
		render := func(v string) string {
			if quote {
				return kind + " " + strconv.Quote(v)
			}
			return kind + " " + v
		}
		for _, v := range sortedKeys(want) {
			if !got[v] {
				add(render(v), "is in the reference but not in the Go package")
			}
		}
		for _, v := range sortedKeys(got) {
			if !want[v] {
				add(render(v), "is in the Go package but not in the reference")
			}
		}
	}
	diffSets("number", numbers, pkg.numbers, false)
	diffSets("string", strs, pkg.strs, true)
	diffSets("char", chars, pkg.chars, true)

	for _, row := range sortedKeys(calls) {
		goName, ok := amap[row]
		switch {
		case !ok:
			add("call "+row, "has no Go name in apimap.txt")
		case goName == Inline:
		case pkg.calls[lastSegment(goName)] == 0:
			add("call "+row, fmt.Sprintf("is in the reference but the Go package never calls %s", goName))
		}
	}
	for _, row := range slices.Sorted(maps.Keys(amap)) {
		goName := amap[row]
		owner, _, _ := strings.Cut(row, ".")
		if owner != helperOwner || goName == Inline || calls[row] || pkg.calls[lastSegment(goName)] == 0 {
			continue
		}
		mappedHere := false
		for r := range calls {
			mappedHere = mappedHere || amap[r] == goName
		}
		if !mappedHere {
			add("call "+row, fmt.Sprintf("is not in the reference but the Go package calls %s", goName))
		}
	}

	goHooks := make([]string, 0, len(pkg.hooks))
	for _, h := range pkg.hooks {
		goHooks = append(goHooks, h.String())
	}
	for _, h := range multisetMinus(hooks, goHooks) {
		add("hook "+h, "is in the reference but not in the Go package")
	}
	for _, h := range multisetMinus(goHooks, hooks) {
		add("hook "+h, "is in the Go package but not in the reference")
	}

	unused := map[string]bool{}
	for _, e := range exempt {
		unused[e] = true
	}
	scope := strings.Join(classes, "+")
	var problems []string
	for _, k := range keys {
		key, msg, _ := strings.Cut(k, "\x00")
		if _, ok := unused[key]; ok {
			unused[key] = false
			continue
		}
		problems = append(problems, scope+": "+key+" "+msg)
	}
	for _, e := range exempt {
		if unused[e] {
			problems = append(problems, scope+": exemption "+strconv.Quote(e)+" matches no difference")
		}
	}
	slices.Sort(problems)
	return problems, nil
}

func lastSegment(name string) string { return name[strings.LastIndexByte(name, '.')+1:] }

// multisetMinus returns the elements of a not matched one-for-one in b.
func multisetMinus(a, b []string) []string {
	left := map[string]int{}
	for _, v := range b {
		left[v]++
	}
	var out []string
	for _, v := range a {
		if left[v] > 0 {
			left[v]--
			continue
		}
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}
