package pets

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// askerID is the monster whose attacked hook asks its attacker whether it
// carries the asked item or adena, as Gordon asks for its two items.
const (
	askerID     = int32(20088)
	askedItemID = int32(8190)
	// servitorAttackAction is the servitor shortcut's attack.
	servitorAttackAction = int32(22)
)

// askLog records the attacked hooks of the asker: how many asked, and the
// answers of the ones that got past the question.
type askLog struct {
	mu      sync.Mutex
	asked   int
	answers []bool
	buf     bytes.Buffer
}

// Write is the server's log sink.
func (l *askLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *askLog) snapshot() (asked int, answers []bool, logs string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.asked, append([]bool(nil), l.answers...), l.buf.String()
}

// askerOptions registers the asker's behavior and sends the server's log
// to l.
func askerOptions(l *askLog) []gameservertest.Option {
	catalog := script.Catalog{
		"ai.Asker": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{askerID}, Hooks: script.Hooks{
				OnAttacked: func(_ *script.Script, e script.Attacked) {
					l.mu.Lock()
					l.asked++
					l.mu.Unlock()
					got := e.Attacker.HasAtLeastOneItem(askedItemID, item.AdenaID)
					l.mu.Lock()
					l.answers = append(l.answers, got)
					l.mu.Unlock()
				},
			}}
		},
	}
	return []gameservertest.Option{
		gameservertest.WithLog(zerolog.New(l)),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{askerID: script.KindHostile},
			[]script.Listing{{Path: "ai.Asker"}}, catalog),
	}
}

// sendAgainstAsker puts the summon beside the owner, spawns the asker
// within its reach, targets the asker and orders the summon to attack it
// through the shortcut action.
func sendAgainstAsker(t *testing.T, srv *gameservertest.Server, client *testsupport.ScriptedClient, ownerID int32, s *summon.Actor, action int32) *npc.Hostile {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, ownerID)
	placePet(t, s, location.Location{X: x, Y: y, Z: z})
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID, tmpl.HPMax = int(askerID), int(askerID), 1_000_000
	asker := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: x + 20, Y: y, Z: z})
	drainUntilQuiet(t, client)
	client.Send(encodeAction(asker.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, client)
	client.Send(encodeRequestActionUse(action, false))
	return asker
}

// TestScriptAsksAPetAttackerForItems: a pet that hits the asker answers
// from its own inventory, false while it carries neither item and true
// once its owner gave it adena; no invocation panics.
func TestScriptAsksAPetAttackerForItems(t *testing.T) {
	t.Parallel()
	l := &askLog{}
	h := bootOwnerWithCollarOpts(t, askerOptions(l), seedItem{TemplateID: item.AdenaID, Count: 40})
	wolf, _ := h.spawnWolf(t)
	sendAgainstAsker(t, h.srv, h.client, h.ownerID, wolf, petAttackAction)
	h.srv.AdvanceUntil(t, "the asker answered the pet's first hit", func() bool {
		_, answers, _ := l.snapshot()
		return len(answers) > 0
	})
	if _, answers, _ := l.snapshot(); answers[0] {
		t.Fatal("a pet carrying neither item answered true")
	}

	h.giveToPet(t, h.seededItem(t, item.AdenaID), 40)
	h.srv.AdvanceUntil(t, "the asker answered true to a hit of the pet carrying adena", func() bool {
		_, answers, _ := l.snapshot()
		return answers[len(answers)-1]
	})
	asked, answers, logs := l.snapshot()
	if asked != len(answers) || strings.Contains(logs, "script: hook panicked") {
		t.Fatalf("%d hooks asked, %d answered; logs: %s", asked, len(answers), logs)
	}
}

// TestScriptAsksAServitorAttackerForItemsAborts: a servitor has no
// inventory, so the question panics, the engine logs it and the hook goes
// no further.
func TestScriptAsksAServitorAttackerForItemsAborts(t *testing.T) {
	t.Parallel()
	l := &askLog{}
	o := bootServitorOwner(t, askerOptions(l)...)
	servitor := o.summonServitor(t)
	sendAgainstAsker(t, o.srv, o.client, o.id, servitor, servitorAttackAction)
	o.srv.AdvanceUntil(t, "the asker's hook panicked on the servitor's hit", func() bool {
		_, _, logs := l.snapshot()
		return strings.Contains(logs, "script: hook panicked")
	})
	asked, answers, logs := l.snapshot()
	if asked == 0 || len(answers) != 0 {
		t.Fatalf("%d hooks asked, answers %v; want every hook to stop at the question", asked, answers)
	}
	if !strings.Contains(logs, "ask a servitor for items") {
		t.Fatalf("the panic logged does not name the servitor's missing inventory: %s", logs)
	}
}
