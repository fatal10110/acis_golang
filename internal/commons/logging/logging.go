package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/fatal10110/acis_golang/internal/config"
)

// Sink names an output stream with its own file.
type Sink string

const (
	SinkConsole Sink = "console"
	SinkError   Sink = "error"
	SinkChat    Sink = "chat"
	SinkGMAudit Sink = "gmaudit"
	SinkItem    Sink = "item"
)

// Config is the logging setup derived from logging.properties.
type Config struct {
	Level           slog.Level
	Patterns        map[Sink]string
	UnsupportedKeys []string
}

// Runtime owns open log files and logger instances.
type Runtime struct {
	Logger  *slog.Logger
	Chat    *slog.Logger
	GMAudit *slog.Logger
	Item    *slog.Logger

	paths map[Sink]string
	files []*os.File
	once  sync.Once
	err   error
}

var supportedKeys = map[string]bool{
	"handlers":                  true,
	"chat.handlers":             true,
	"chat.useParentHandlers":    true,
	"gmaudit.handlers":          true,
	"gmaudit.useParentHandlers": true,
	"item.handlers":             true,
	"item.useParentHandlers":    true,
	".level":                    true,
	"java.util.logging.ConsoleHandler.formatter":                     true,
	"java.util.logging.ConsoleHandler.level":                         true,
	"java.util.logging.FileHandler.pattern":                          true,
	"java.util.logging.FileHandler.limit":                            true,
	"java.util.logging.FileHandler.count":                            true,
	"java.util.logging.FileHandler.formatter":                        true,
	"java.util.logging.FileHandler.level":                            true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.pattern":     true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.limit":       true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.count":       true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.formatter":   true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.filter":      true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.level":       true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.pattern":      true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.limit":        true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.count":        true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.formatter":    true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.filter":       true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.append":       true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.level":        true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.pattern":   true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.limit":     true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.count":     true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.formatter": true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.filter":    true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.append":    true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.level":     true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.pattern":      true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.limit":        true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.count":        true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.formatter":    true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.filter":       true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.append":       true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.level":        true,
	"net.sf.l2j.gameserver.level":                                    true,
	"net.sf.l2j.loginserver.level":                                   true,
}

// DefaultConfig returns the logging setup used when logging.properties is not loaded yet.
func DefaultConfig() Config {
	return Config{
		Level: slog.LevelInfo,
		Patterns: map[Sink]string{
			SinkConsole: "log/console/console_%g.txt",
			SinkError:   "log/error/error_%g.txt",
			SinkChat:    "log/chat/chat_%g.txt",
			SinkGMAudit: "log/gmaudit/gmaudit_%g.txt",
			SinkItem:    "log/item/item_%g.txt",
		},
	}
}

// ConfigFromProperties derives logging setup from logging.properties.
func ConfigFromProperties(p *config.Properties) (Config, error) {
	cfg := DefaultConfig()
	level, err := parseLevel(p.String(".level", "INFO"))
	if err != nil {
		return Config{}, err
	}
	cfg.Level = level

	cfg.Patterns[SinkConsole] = p.String("java.util.logging.FileHandler.pattern", cfg.Patterns[SinkConsole])
	cfg.Patterns[SinkError] = p.String("net.sf.l2j.commons.logging.handler.ErrorLogHandler.pattern", cfg.Patterns[SinkError])
	cfg.Patterns[SinkChat] = p.String("net.sf.l2j.commons.logging.handler.ChatLogHandler.pattern", cfg.Patterns[SinkChat])
	cfg.Patterns[SinkGMAudit] = p.String("net.sf.l2j.commons.logging.handler.GMAuditLogHandler.pattern", cfg.Patterns[SinkGMAudit])
	cfg.Patterns[SinkItem] = p.String("net.sf.l2j.commons.logging.handler.ItemLogHandler.pattern", cfg.Patterns[SinkItem])

	for _, key := range p.Keys() {
		if !supportedKeys[key] {
			cfg.UnsupportedKeys = append(cfg.UnsupportedKeys, key)
		}
	}
	sort.Strings(cfg.UnsupportedKeys)
	return cfg, nil
}

// Setup opens log files and returns the configured loggers.
func Setup(root string, cfg Config, stderr io.Writer) (*Runtime, error) {
	if stderr == nil {
		stderr = io.Discard
	}

	opts := &slog.HandlerOptions{Level: cfg.Level}
	rt := &Runtime{paths: make(map[Sink]string)}

	open := func(sink Sink) (*os.File, error) {
		pattern := cfg.Patterns[sink]
		if pattern == "" {
			return nil, fmt.Errorf("missing log pattern for %s", sink)
		}
		name := filepath.Join(root, filepath.FromSlash(expandPattern(pattern)))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return nil, err
		}
		file, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		rt.paths[sink] = name
		rt.files = append(rt.files, file)
		return file, nil
	}

	consoleFile, err := open(SinkConsole)
	if err != nil {
		return nil, err
	}
	errorFile, err := open(SinkError)
	if err != nil {
		_ = rt.Close()
		return nil, err
	}
	chatFile, err := open(SinkChat)
	if err != nil {
		_ = rt.Close()
		return nil, err
	}
	gmFile, err := open(SinkGMAudit)
	if err != nil {
		_ = rt.Close()
		return nil, err
	}
	itemFile, err := open(SinkItem)
	if err != nil {
		_ = rt.Close()
		return nil, err
	}

	console := fanoutHandler{
		slog.NewTextHandler(stderr, opts),
		slog.NewJSONHandler(consoleFile, opts),
		filterHandler{next: slog.NewJSONHandler(errorFile, opts), min: slog.LevelError},
	}
	rt.Logger = slog.New(console)
	rt.Chat = slog.New(slog.NewJSONHandler(chatFile, opts)).With("sink", string(SinkChat))
	rt.GMAudit = slog.New(slog.NewJSONHandler(gmFile, opts)).With("sink", string(SinkGMAudit))
	rt.Item = slog.New(slog.NewJSONHandler(itemFile, opts)).With("sink", string(SinkItem))

	return rt, nil
}

// InstallDefault makes runtime.Logger the package-wide slog default.
func InstallDefault(runtime *Runtime) {
	slog.SetDefault(runtime.Logger)
}

// Path returns the opened file path for sink.
func (r *Runtime) Path(sink Sink) string {
	return r.paths[sink]
}

// Close closes all opened log files. It is safe to call more than once.
func (r *Runtime) Close() error {
	r.once.Do(func() {
		for _, file := range r.files {
			if err := file.Close(); err != nil && r.err == nil {
				r.err = err
			}
		}
	})
	return r.err
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ALL", "FINEST", "FINER", "FINE", "CONFIG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARNING":
		return slog.LevelWarn, nil
	case "SEVERE":
		return slog.LevelError, nil
	case "OFF":
		return slog.LevelError + 1000, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", s)
	}
}

func expandPattern(pattern string) string {
	pattern = strings.ReplaceAll(pattern, "%g", "0")
	pattern = strings.ReplaceAll(pattern, "%u", "0")
	return pattern
}

type fanoutHandler []slog.Handler

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, next := range h {
		if next.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, next := range h {
		if !next.Enabled(ctx, record.Level) {
			continue
		}
		if err := next.Handle(ctx, record.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanoutHandler, len(h))
	for i, next := range h {
		out[i] = next.WithAttrs(attrs)
	}
	return out
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	out := make(fanoutHandler, len(h))
	for i, next := range h {
		out[i] = next.WithGroup(name)
	}
	return out
}

type filterHandler struct {
	next slog.Handler
	min  slog.Level
}

func (h filterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.min && h.next.Enabled(ctx, level)
}

func (h filterHandler) Handle(ctx context.Context, record slog.Record) error {
	if !h.Enabled(ctx, record.Level) {
		return nil
	}
	return h.next.Handle(ctx, record)
}

func (h filterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return filterHandler{next: h.next.WithAttrs(attrs), min: h.min}
}

func (h filterHandler) WithGroup(name string) slog.Handler {
	return filterHandler{next: h.next.WithGroup(name), min: h.min}
}
