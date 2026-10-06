package quest

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// traceStep is one client packet of a reference quest trace and what the
// server answered it with: the packets the goldens render, and the journal
// statements.
type traceStep struct {
	client string
	sent   []string
	stmts  []string
}

// traceRenders are the opcodes scriptcontract.Packet renders: the
// dialog's, the quest's and the item messages. The rest of a trace (the
// selection, the approach, the talk animation, NPC info and the deferred
// updates) is not the quest's.
var traceRenders = []byte{
	serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodePlaySound, serverpackets.OpcodeQuestList, 0xfe,
}

var traceAction = regexp.MustCompile(`^C Action (\d+) \(object (\d+)\)$`)

// readTrace reads a reference quest trace: its steps, the player's object
// id and the object id of each NPC id it talks to.
func readTrace(t *testing.T, name string) (steps []traceStep, playerObj int32, npcObjs map[int32]int32) {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "internal", "gameserver", "script", "testdata", "oracle", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	npcObjs = map[int32]int32{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var raw []struct {
		client string
		lines  []string
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "# player object "):
			obj, _, _ := strings.Cut(strings.TrimPrefix(line, "# player object "), ";")
			n, err := strconv.ParseInt(obj, 10, 32)
			if err != nil {
				t.Fatal(err)
			}
			playerObj = int32(n)
		case strings.HasPrefix(line, "C "):
			if m := traceAction.FindStringSubmatch(line); m != nil {
				id, _ := strconv.Atoi(m[1])
				obj, _ := strconv.Atoi(m[2])
				npcObjs[int32(id)] = int32(obj)
			}
			raw = append(raw, struct {
				client string
				lines  []string
			}{client: line})
		case (strings.HasPrefix(line, "S ") || strings.HasPrefix(line, "Q ")) && len(raw) > 0:
			raw[len(raw)-1].lines = append(raw[len(raw)-1].lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	roles := map[int32]string{playerObj: "player"}
	for id, obj := range npcObjs {
		roles[obj] = strconv.Itoa(int(id))
	}
	for _, r := range raw {
		step := traceStep{client: r.client}
		for _, line := range r.lines {
			if strings.HasPrefix(line, "Q ") {
				st, err := scriptcontract.ParseStatement(line)
				if err != nil {
					t.Fatal(err)
				}
				step.stmts = append(step.stmts, string(st.Kind)+" "+strings.Join(st.Params[1:], " | "))
				continue
			}
			fields := strings.Fields(line)
			payload, err := hex.DecodeString(fields[len(fields)-1])
			if err != nil {
				t.Fatalf("trace line %q: %v", line, err)
			}
			if !slices.Contains(traceRenders, payload[0]) {
				continue
			}
			rendered, err := scriptcontract.Packet(payload, roles)
			if err != nil {
				continue // an extended packet other than the quest mark
			}
			step.sent = append(step.sent, rendered)
		}
		steps = append(steps, step)
	}
	return steps, playerObj, npcObjs
}

// q001TracePages reads the pages the Q001 trace shows from the datapack:
// the three NPCs' chat windows and Q001's pages.
func q001TracePages(t *testing.T) map[string]string {
	t.Helper()
	root := datapack.Path(t, "data", "html")
	pages := map[string]string{}
	for _, name := range []string{"default/30048.htm", "gatekeeper/30006.htm", "trainer/30033.htm"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		pages[name] = string(b)
	}
	dir := filepath.Join(root, "script", "quest", "Q001_LettersOfLove")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		pages["script/quest/Q001_LettersOfLove/"+e.Name()] = string(b)
	}
	return pages
}

// TestQ001PlaysAsTheReferenceTrace plays Letters of Love through client
// packets exactly as the reference trace does (talk to Darin, accept,
// deliver to Roxxy through her quest choice list, back to Darin, to
// Baulro, back to Darin, talk once more) and requires, step by step, the
// reference's dialog, quest and item packets in order and its
// character_quests statements in order. Then the character relogs: the
// completed quest is loaded from its one remaining row, the necklace is
// kept, and Darin still answers that the quest is completed.
func TestQ001PlaysAsTheReferenceTrace(t *testing.T) {
	t.Parallel()
	steps, _, traceNPCs := readTrace(t, "trace_q001.golden")
	w := bootDialog(t, q001TracePages(t), nil)
	kinds := map[int32]string{darinID: "Folk", roxxyID: "Gatekeeper", baulroID: "Trainer"}
	goObjs := map[int32]int32{}
	for i, id := range []int32{darinID, roxxyID, baulroID} {
		f := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate(kinds[id], int(id)), location.Location{X: w.at.X + 10*(i+1), Y: w.at.Y, Z: w.at.Z})
		goObjs[id] = f.ObjectID()
		w.roles[f.ObjectID()] = strconv.Itoa(int(id))
	}
	w.roles[w.player] = "player"
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.TakeJournalWrites()

	packets, statements := 0, 0
	for i, step := range steps {
		packets, statements = packets+len(step.sent), statements+len(step.stmts)
		cmd := step.client
		switch {
		case strings.HasPrefix(cmd, "C Action "):
			m := traceAction.FindStringSubmatch(cmd)
			id, _ := strconv.Atoi(m[1])
			w.srv.Client.Send(encodeTutorialAction(goObjs[int32(id)], w.at))
		case strings.HasPrefix(cmd, "C RequestBypassToServer "):
			bypass := strings.TrimPrefix(cmd, "C RequestBypassToServer ")
			for id, obj := range traceNPCs {
				bypass = strings.ReplaceAll(bypass, strconv.Itoa(int(obj)), strconv.Itoa(int(goObjs[id])))
			}
			w.srv.Client.Send(encodeBypass(bypass))
		default:
			t.Fatalf("step %d: unknown client packet %q", i, cmd)
		}
		var got []string
		for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
			if !slices.Contains(traceRenders, f[0]) {
				continue
			}
			line, err := scriptcontract.Packet(f, w.roles)
			if err != nil {
				continue
			}
			got = append(got, line)
		}
		if !slices.Equal(got, step.sent) {
			t.Fatalf("step %d %s packets:\n got %q\nwant %q", i, cmd, got, step.sent)
		}
		if stmts, want := statementLines(w.srv.TakeJournalWrites()), step.stmts; !slices.Equal(stmts, want) {
			t.Fatalf("step %d %s statements:\n got %q\nwant %q", i, cmd, stmts, want)
		}
	}
	// The trace's dialog and quest packets and its statements, all compared.
	if packets != 74 || statements != 7 {
		t.Fatalf("compared %d packets and %d statements, want the trace's 74 and 7", packets, statements)
	}
	w.srv.FlushPersistence(t)
	if got := questRows(t, w.srv, w.player, q001); fmt.Sprint(got) != "map[<state>:COMPLETED]" {
		t.Fatalf("rows after the quest = %v, want only <state>=COMPLETED", got)
	}
	for _, id := range q001Items {
		if n := w.srv.PlayerItemCount(t, w.player, id); n != 0 {
			t.Fatalf("quest item %d held after the quest: %d", id, n)
		}
	}
	restart(t, w.srv)
	enterWorld(t, w.srv)
	if n := w.srv.PlayerItemCount(t, w.player, necklace); n != 1 {
		t.Fatalf("necklaces after relog = %d, want 1", n)
	}
	if got := w.states(t); got != q001+":"+questlog.StatusCompleted.String() {
		t.Fatalf("states after relog = %s, want Q001 completed", got)
	}
	w.interact(t, goObjs[darinID])
	w.offerAll(t)
	got := w.bypass(t, fmt.Sprintf("npc_%d_Quest", goObjs[darinID]))
	want := []string{`S NpcHtmlMessage obj=30048 item=0 html="<html><body>This quest has already been completed.</body></html>"`, "S ActionFailed", "S ActionFailed"}
	if !slices.Equal(got, want) {
		t.Fatalf("Darin after relog:\n got %q\nwant %q", got, want)
	}
}
