// Command botswarm drives simulated players against a running login server
// and game server over the real client protocol, for load testing. It
// prints a latency, throughput and error summary and can write it as JSON.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fatal10110/acis_golang/internal/botclient"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run parses args, runs the swarm and reports; it returns the exit code:
// 0 when every bot entered the world with no error or disconnect, 1 when
// some did not, 2 on a usage error.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cfg := botclient.DefaultConfig()
	fs := flag.NewFlagSet("botswarm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.LoginAddr, "login", cfg.LoginAddr, "login server client address (host:port)")
	fs.StringVar(&cfg.GameAddr, "game-addr", "", "game server address overriding the one the ServerList advertises")
	serverID := fs.Uint("server", uint(cfg.ServerID), "game server id to join")
	revision := fs.Int("revision", int(cfg.Revision), "client protocol revision")
	fs.IntVar(&cfg.Bots, "bots", cfg.Bots, "number of bots; bot i plays account <prefix><i>")
	fs.StringVar(&cfg.Prefix, "prefix", cfg.Prefix, "account and character name prefix (lowercase letters and digits; prefix plus index at most 14 characters)")
	fs.StringVar(&cfg.Password, "password", cfg.Password, "password of every bot account (1-16 bytes)")
	fs.Float64Var(&cfg.Rate, "rate", cfg.Rate, "bots started per second (0: all at once)")
	fs.DurationVar(&cfg.Duration, "duration", cfg.Duration, "run length (0: until every bot spends its action budget)")
	fs.IntVar(&cfg.Actions, "actions", cfg.Actions, "behavior ticks per bot (0: unbounded)")
	fs.DurationVar(&cfg.ActionInterval, "interval", cfg.ActionInterval, "pause between a bot's behavior ticks")
	behaviors := fs.String("behaviors", joinBehaviors(cfg.Behaviors), "comma-separated behavior cycle: move, validate, chat, attack, idle")
	moveRadius := fs.Int("move-radius", int(cfg.MoveRadius), "largest move offset per axis from the bot's position")
	fs.DurationVar(&cfg.SelectWait, "select-wait", cfg.SelectWait, "wait between creating a character and selecting it (above the server's CharacterSelectTime)")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "handshake read/write and logout timeout")
	fs.Uint64Var(&cfg.Seed, "seed", cfg.Seed, "move offset random seed")
	jsonOut := fs.String("json", "", "also write the report as JSON to this file (- for stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *serverID > 255 {
		fmt.Fprintln(stderr, "botswarm: -server must be 0-255")
		return 2
	}
	cfg.ServerID, cfg.Revision, cfg.MoveRadius = byte(*serverID), int32(*revision), int32(*moveRadius)
	parsed, err := botclient.ParseBehaviors(strings.Split(*behaviors, ","))
	if err != nil {
		fmt.Fprintln(stderr, "botswarm:", err)
		return 2
	}
	cfg.Behaviors = parsed
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(stderr, "botswarm:", err)
		return 2
	}

	report, err := botclient.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "botswarm:", err)
		return 2
	}
	if err := report.WriteText(stdout); err != nil {
		fmt.Fprintln(stderr, "botswarm:", err)
		return 1
	}
	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, stdout, &report); err != nil {
			fmt.Fprintln(stderr, "botswarm:", err)
			return 1
		}
	}
	if report.Entered != int64(cfg.Bots) || report.Disconnects > 0 || len(report.Errors) > 0 {
		return 1
	}
	return 0
}

func joinBehaviors(bs []botclient.Behavior) string {
	s := make([]string, len(bs))
	for i, b := range bs {
		s[i] = string(b)
	}
	return strings.Join(s, ",")
}

func writeJSON(path string, stdout io.Writer, r *botclient.Report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "-" {
		_, err = stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
