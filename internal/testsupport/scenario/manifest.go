package scenario

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// manifestPath is the reference registration manifest, below the module
// root (format: the oracle directory's README).
const manifestPath = "internal/gameserver/script/testdata/oracle/manifest.golden"

// manifestStub builds the script the manifest records for path with no
// hook: its quest id, title and items when it is a real quest, and its NPC
// bindings.
func manifestStub(file, path string) (script.Script, error) {
	f, err := os.Open(file)
	if err != nil {
		return script.Script{}, err
	}
	defer f.Close()
	events := map[string]script.NPCEvent{}
	for e := script.NPCEvent(0); e.String() != "NPCEvent(?)"; e++ {
		events[e.String()] = e
	}
	var (
		stub  = script.Script{Bind: script.Bindings{}}
		found bool
	)
	sb := bufio.NewScanner(f)
	sb.Buffer(make([]byte, 1<<20), 1<<20)
	for sb.Scan() {
		fields := strings.Fields(sb.Text())
		if len(fields) == 0 {
			if found {
				break
			}
			continue
		}
		if fields[0] == "script" {
			if found {
				break
			}
			found = len(fields) == 2 && fields[1] == path
			continue
		}
		if !found {
			continue
		}
		switch fields[0] {
		case "quest":
			// quest <id> name <name> descr <rest of line>
			id, err := strconv.ParseInt(fields[1], 10, 32)
			if err != nil {
				return script.Script{}, fmt.Errorf("manifest %s quest id %q: %w", path, fields[1], err)
			}
			if id > 0 {
				stub.QuestID = int32(id)
				_, stub.Title, _ = strings.Cut(sb.Text(), " descr ")
			}
		case "items":
			ids, err := manifestIDs(fields[1])
			if err != nil {
				return script.Script{}, fmt.Errorf("manifest %s items: %w", path, err)
			}
			stub.Items = ids
		case "bind":
			ids, err := manifestIDs(fields[2])
			if err != nil {
				return script.Script{}, fmt.Errorf("manifest %s bind: %w", path, err)
			}
			for _, name := range strings.Split(fields[1], ",") {
				e, ok := events[name]
				if !ok {
					return script.Script{}, fmt.Errorf("manifest %s binds unknown event %s", path, name)
				}
				stub.Bind[e] = append(stub.Bind[e], ids...)
			}
		case "kind":
			if fields[1] == "behavior" {
				return script.Script{}, fmt.Errorf("%s is a behavior; a stub registers no hook, so it would bind nothing", path)
			}
		}
	}
	if err := sb.Err(); err != nil {
		return script.Script{}, err
	}
	if !found {
		return script.Script{}, fmt.Errorf("the manifest has no script %s", path)
	}
	return stub, nil
}

// manifestIDs reads a manifest id list: comma separated, a-b for a run.
func manifestIDs(s string) ([]int32, error) {
	var out []int32
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.ParseInt(lo, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("id %q: %w", part, err)
		}
		b := a
		if isRange {
			if b, err = strconv.ParseInt(hi, 10, 32); err != nil {
				return nil, fmt.Errorf("id range %q: %w", part, err)
			}
		}
		for id := a; id <= b; id++ {
			out = append(out, int32(id))
		}
	}
	return out, nil
}
