package botswarm

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/botclient"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBotSwarmPlaysThroughLoginAndGameServer runs a few bots over real
// sockets: login-server handshake with account auto-creation, game-server
// crypt, character creation (or selection of an existing one), EnterWorld,
// then the full behavior cycle against a visible monster.
func TestBotSwarmPlaysThroughLoginAndGameServer(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithProductionTickers(),
	)
	// swarm0 already has a character and selects it; the others create one.
	srv.SeedCharacterFor(t, "swarm0", "Swarm0", 1, 0)
	spawn := location.Location{X: 10, Y: 20, Z: gameservertest.SpawnZ}
	monster := location.Location{X: spawn.X + 60, Y: spawn.Y, Z: spawn.Z}
	srv.SpawnMovingHostileNPCTemplate(t, gameservertest.MovingHostileTemplate("Monster"), monster, monster)

	cfg := botclient.DefaultConfig()
	cfg.LoginAddr = srv.StartLoginClients(t)
	cfg.GameAddr = srv.Addr()
	cfg.Bots = 3
	cfg.Prefix = "swarm"
	cfg.Rate = 0
	cfg.Duration = 0
	cfg.Actions = 2 * len(botclient.DefaultBehaviors)
	cfg.ActionInterval = 0 // back to back: the run is bounded by the action budget
	cfg.SelectWait = 0     // WithReuseDelays(0, 0) lifts the character-select gate

	report, err := botclient.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.Entered != 3 || len(report.Errors) != 0 || report.Disconnects != 0 || report.Aborted != 0 {
		t.Fatalf("report = %+v", report)
	}
	for name, l := range map[string]botclient.Latency{"connect": report.Connect, "login": report.Login, "enter world": report.EnterWorld} {
		if l.Count != 3 || l.P50ms <= 0 || l.P99ms < l.P50ms {
			t.Errorf("%s latency = %+v", name, l)
		}
	}
	if report.PacketsSent == 0 || report.PacketsReceived == 0 || report.PacketsRecvPerSec <= 0 {
		t.Errorf("packets sent %d, received %d (%.1f/s)", report.PacketsSent, report.PacketsReceived, report.PacketsRecvPerSec)
	}
	for _, b := range []botclient.Behavior{botclient.BehaviorMove, botclient.BehaviorValidate, botclient.BehaviorChat, botclient.BehaviorIdle} {
		if report.Actions[string(b)] != 6 {
			t.Errorf("%s actions = %d, want 6 (actions %v)", b, report.Actions[string(b)], report.Actions)
		}
	}
	if report.Actions[string(botclient.BehaviorAttack)] == 0 {
		t.Errorf("no bot attacked the monster in sight (actions %v)", report.Actions)
	}

	var accounts, created int
	if err := srv.DB.QueryRow("SELECT COUNT(*) FROM accounts WHERE login IN ('swarm0', 'swarm1', 'swarm2')").Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.QueryRow("SELECT COUNT(*) FROM characters WHERE account_name = char_name AND char_name IN ('swarm1', 'swarm2')").Scan(&created); err != nil {
		t.Fatal(err)
	}
	if accounts != 3 || created != 2 {
		t.Fatalf("auto-created accounts = %d (want 3), created characters = %d (want 2)", accounts, created)
	}
}
