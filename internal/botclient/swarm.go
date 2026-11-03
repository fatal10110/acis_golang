package botclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Account and character name limits: the login credential block holds 14
// bytes of name, and character names are 1-16 ASCII letters or digits. Each
// bot plays account prefix+index and names its character the same.
const maxAccountLen = 14

// maxErrorSamples caps the distinct error texts a report keeps.
const maxErrorSamples = 10

// Config drives one swarm run.
type Config struct {
	// LoginAddr is the login server's client address (host:port).
	LoginAddr string
	// GameAddr, when set, replaces the game address the ServerList
	// advertises (for a server registered behind NAT or on loopback).
	GameAddr string
	// ServerID is the game server to join.
	ServerID byte
	// Revision is the client protocol revision sent in ProtocolVersion.
	Revision int32

	// Bots is how many bots to run; bot i plays account Prefix+i.
	Bots int
	// Prefix starts every account and character name; letters and digits.
	Prefix string
	// Password is every bot's password. With AutoCreateAccounts on, a
	// first login registers the account with it.
	Password string
	// Rate is how many bots start per second; 0 starts them all at once.
	Rate float64
	// Duration ends the run; 0 lets each bot run its Actions out.
	Duration time.Duration

	// Actions caps each bot's behavior ticks; 0 is unbounded.
	Actions int
	// ActionInterval paces each bot's ticks; 0 sends them back to back.
	ActionInterval time.Duration
	// Behaviors is the cycle each bot runs, starting at its own index.
	Behaviors []Behavior
	// MoveRadius bounds a move's offset from the bot's position on each axis.
	MoveRadius int32
	// Timeout bounds every handshake read and write and the logout wait.
	Timeout time.Duration
	// Seed seeds the move offsets.
	Seed uint64
}

// DefaultConfig returns the settings the command line starts from.
func DefaultConfig() Config {
	return Config{
		LoginAddr:      "127.0.0.1:2106",
		ServerID:       1,
		Revision:       746,
		Bots:           10,
		Prefix:         "bot",
		Password:       "botpass",
		Rate:           5,
		Duration:       time.Minute,
		ActionInterval: time.Second,
		Behaviors:      DefaultBehaviors,
		MoveRadius:     300,
		Timeout:        30 * time.Second,
		Seed:           1,
	}
}

// Validate reports the first setting a run cannot use.
func (c *Config) Validate() error {
	switch {
	case c.LoginAddr == "":
		return errors.New("login address is required")
	case c.Bots <= 0:
		return errors.New("bots must be positive")
	case c.Rate < 0:
		return errors.New("rate must not be negative")
	case c.Duration == 0 && c.Actions == 0:
		return errors.New("set a duration or an action budget")
	case len(c.Behaviors) == 0:
		return errors.New("no behaviors")
	case c.MoveRadius < 0:
		return errors.New("move radius must not be negative")
	case c.Timeout <= 0:
		return errors.New("timeout must be positive")
	case c.Password == "" || len(c.Password) > authPassMax:
		return fmt.Errorf("password must be 1-%d bytes", authPassMax)
	}
	if c.Prefix == "" {
		return errors.New("account prefix is required")
	}
	for _, r := range c.Prefix {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return fmt.Errorf("account prefix %q: use lowercase ASCII letters and digits", c.Prefix)
		}
	}
	if n := len(c.Prefix) + len(strconv.Itoa(c.Bots-1)); n > maxAccountLen {
		return fmt.Errorf("account %s%d is %d characters, the limit is %d", c.Prefix, c.Bots-1, n, maxAccountLen)
	}
	return nil
}

// Latency summarizes one phase's durations in milliseconds.
type Latency struct {
	Count int     `json:"count"`
	P50ms float64 `json:"p50_ms"`
	P99ms float64 `json:"p99_ms"`
	Maxms float64 `json:"max_ms"`
}

// Report is a run's outcome.
type Report struct {
	Bots int `json:"bots"`
	// Started counts the bots the ramp launched; a duration that ends
	// mid-ramp leaves the rest unstarted.
	Started     int     `json:"started"`
	Entered     int64   `json:"entered_world"`
	ElapsedSecs float64 `json:"elapsed_seconds"`

	Connect    Latency `json:"connect_latency"`
	Login      Latency `json:"login_latency"`
	EnterWorld Latency `json:"enter_world_latency"`

	PacketsSent       int64            `json:"packets_sent"`
	PacketsReceived   int64            `json:"packets_received"`
	PacketsSentPerSec float64          `json:"packets_sent_per_second"`
	PacketsRecvPerSec float64          `json:"packets_received_per_second"`
	Actions           map[string]int64 `json:"actions"`

	// Disconnects counts bots the server dropped after entering the world.
	Disconnects int64 `json:"disconnects"`
	// Errors counts failed bots by phase: "login" or "game" (before the
	// world was entered).
	Errors map[string]int64 `json:"errors"`
	// Aborted counts bots the run's end interrupted before they entered.
	Aborted      int64    `json:"aborted"`
	ErrorSamples []string `json:"error_samples,omitempty"`
}

// WriteText prints r for a terminal.
func (r *Report) WriteText(w io.Writer) error {
	line := func(name string, l Latency) string {
		return fmt.Sprintf("%-12s n=%-5d p50=%8.1fms p99=%8.1fms max=%8.1fms\n", name, l.Count, l.P50ms, l.P99ms, l.Maxms)
	}
	text := fmt.Sprintf("bots %d, started %d, entered world %d, elapsed %.1fs\n", r.Bots, r.Started, r.Entered, r.ElapsedSecs) +
		line("connect", r.Connect) + line("login", r.Login) + line("enter world", r.EnterWorld) +
		fmt.Sprintf("packets sent %d (%.1f/s), received %d (%.1f/s)\n", r.PacketsSent, r.PacketsSentPerSec, r.PacketsReceived, r.PacketsRecvPerSec) +
		fmt.Sprintf("actions %v\n", sortedCounts(r.Actions)) +
		fmt.Sprintf("disconnects %d, errors %v, aborted %d\n", r.Disconnects, sortedCounts(r.Errors), r.Aborted)
	for _, s := range r.ErrorSamples {
		text += "  error: " + s + "\n"
	}
	_, err := io.WriteString(w, text)
	return err
}

func sortedCounts(m map[string]int64) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := "{"
	for i, k := range keys {
		if i > 0 {
			out += " "
		}
		out += k + "=" + strconv.FormatInt(m[k], 10)
	}
	return out + "}"
}

// recorder collects every bot's metrics; mu guards the slices and maps.
type recorder struct {
	counters Counters
	entered  atomic.Int64

	mu                         sync.Mutex
	connect, login, enterWorld []time.Duration
	actions                    map[string]int64
	errors                     map[string]int64
	samples                    []string
	disconnects, aborted       int64
}

func (r *recorder) latency(into *[]time.Duration, d time.Duration) {
	r.mu.Lock()
	*into = append(*into, d)
	r.mu.Unlock()
}

func (r *recorder) action(name string) {
	r.mu.Lock()
	r.actions[name]++
	r.mu.Unlock()
}

// fail records a bot lost before entering the world; one the run's end
// interrupted counts as aborted, not as an error.
func (r *recorder) fail(ctx context.Context, phase string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil {
		r.aborted++
		return
	}
	r.errors[phase]++
	r.sample(phase + ": " + err.Error())
}

func (r *recorder) disconnect(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.disconnects++
	r.sample("disconnect: " + err.Error())
}

func (r *recorder) sample(s string) {
	if len(r.samples) < maxErrorSamples && !slices.Contains(r.samples, s) {
		r.samples = append(r.samples, s)
	}
}

// Run starts cfg.Bots bots at cfg.Rate per second and waits until every bot
// has logged out: its action budget is spent, the duration has passed or ctx
// ended.
func Run(ctx context.Context, cfg Config) (Report, error) {
	if err := cfg.Validate(); err != nil {
		return Report{}, err
	}
	if cfg.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Duration)
		defer cancel()
	}
	rec := &recorder{actions: make(map[string]int64), errors: make(map[string]int64)}
	start := time.Now()

	var wg sync.WaitGroup
	var gap time.Duration
	started := 0
	if cfg.Rate > 0 {
		gap = time.Duration(float64(time.Second) / cfg.Rate)
	}
	for i := range cfg.Bots {
		if i > 0 && wait(ctx, gap) != nil {
			break
		}
		b := &bot{
			cfg:     &cfg,
			index:   i,
			account: cfg.Prefix + strconv.Itoa(i),
			rec:     rec,
			rng:     rand.New(rand.NewPCG(cfg.Seed, uint64(i))),
		}
		wg.Go(func() { b.run(ctx) })
		started++
	}
	wg.Wait()

	elapsed := time.Since(start)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	r := Report{
		Bots:            cfg.Bots,
		Started:         started,
		Entered:         rec.entered.Load(),
		ElapsedSecs:     elapsed.Seconds(),
		Connect:         summarize(rec.connect),
		Login:           summarize(rec.login),
		EnterWorld:      summarize(rec.enterWorld),
		PacketsSent:     rec.counters.Sent.Load(),
		PacketsReceived: rec.counters.Received.Load(),
		Actions:         rec.actions,
		Disconnects:     rec.disconnects,
		Errors:          rec.errors,
		Aborted:         rec.aborted,
		ErrorSamples:    rec.samples,
	}
	if s := elapsed.Seconds(); s > 0 {
		r.PacketsSentPerSec = float64(r.PacketsSent) / s
		r.PacketsRecvPerSec = float64(r.PacketsReceived) / s
	}
	return r, nil
}

// summarize returns the nearest-rank percentiles of ds.
func summarize(ds []time.Duration) Latency {
	if len(ds) == 0 {
		return Latency{}
	}
	sorted := slices.Clone(ds)
	slices.Sort(sorted)
	rank := func(p float64) float64 {
		i := int(math.Ceil(p*float64(len(sorted)))) - 1
		i = max(0, min(i, len(sorted)-1))
		return ms(sorted[i])
	}
	return Latency{Count: len(sorted), P50ms: rank(0.50), P99ms: rank(0.99), Maxms: ms(sorted[len(sorted)-1])}
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
