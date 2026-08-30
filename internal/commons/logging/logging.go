package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/rs/zerolog"
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
	Level           zerolog.Level
	ConsoleLevel    zerolog.Level
	Levels          map[Sink]zerolog.Level
	Patterns        map[Sink]string
	Limits          map[Sink]int64
	Counts          map[Sink]int
	Append          map[Sink]bool
	UnsupportedKeys []string
}

// Runtime owns open log files and logger instances.
type Runtime struct {
	Logger  zerolog.Logger
	Chat    zerolog.Logger
	GMAudit zerolog.Logger
	Item    zerolog.Logger

	paths map[Sink]string
	files []io.Closer
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
	"java.util.logging.FileHandler.append":                           true,
	"java.util.logging.FileHandler.formatter":                        true,
	"java.util.logging.FileHandler.level":                            true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.pattern":     true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.limit":       true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.count":       true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.append":      true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.formatter":   true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.filter":      true,
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.level":       true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.pattern":      true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.limit":        true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.count":        true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.append":       true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.formatter":    true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.filter":       true,
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.level":        true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.pattern":   true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.limit":     true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.count":     true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.append":    true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.formatter": true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.filter":    true,
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.level":     true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.pattern":      true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.limit":        true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.count":        true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.append":       true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.formatter":    true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.filter":       true,
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.level":        true,
	"net.sf.l2j.gameserver.level":                                    true,
	"net.sf.l2j.loginserver.level":                                   true,
}

// fileHandlerPropertyKey maps each sink to its java.util.logging.FileHandler-family property prefix.
var fileHandlerPropertyKey = map[Sink]string{
	SinkConsole: "java.util.logging.FileHandler",
	SinkError:   "net.sf.l2j.commons.logging.handler.ErrorLogHandler",
	SinkChat:    "net.sf.l2j.commons.logging.handler.ChatLogHandler",
	SinkGMAudit: "net.sf.l2j.commons.logging.handler.GMAuditLogHandler",
	SinkItem:    "net.sf.l2j.commons.logging.handler.ItemLogHandler",
}

func init() {
	zerolog.MessageFieldName = "msg"
}

// DefaultConfig returns the logging setup used when logging.properties is not loaded yet.
func DefaultConfig() Config {
	return Config{
		Level:        zerolog.InfoLevel,
		ConsoleLevel: zerolog.InfoLevel,
		Levels: map[Sink]zerolog.Level{
			SinkConsole: zerolog.InfoLevel,
			SinkError:   zerolog.InfoLevel,
			SinkChat:    zerolog.InfoLevel,
			SinkGMAudit: zerolog.InfoLevel,
			SinkItem:    zerolog.InfoLevel,
		},
		Patterns: map[Sink]string{
			SinkConsole: "log/console/console_%g.txt",
			SinkError:   "log/error/error_%g.txt",
			SinkChat:    "log/chat/chat_%g.txt",
			SinkGMAudit: "log/gmaudit/gmaudit_%g.txt",
			SinkItem:    "log/item/item_%g.txt",
		},
		// java.util.logging.FileHandler class defaults: no size limit, one generation, no append.
		Limits: map[Sink]int64{
			SinkConsole: 0,
			SinkError:   0,
			SinkChat:    0,
			SinkGMAudit: 0,
			SinkItem:    0,
		},
		Counts: map[Sink]int{
			SinkConsole: 1,
			SinkError:   1,
			SinkChat:    1,
			SinkGMAudit: 1,
			SinkItem:    1,
		},
		Append: map[Sink]bool{
			SinkConsole: false,
			SinkError:   false,
			SinkChat:    false,
			SinkGMAudit: false,
			SinkItem:    false,
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
	if cfg.ConsoleLevel, err = propertyLevel(p, "java.util.logging.ConsoleHandler.level", cfg.ConsoleLevel); err != nil {
		return Config{}, err
	}
	for sink, key := range map[Sink]string{
		SinkConsole: "java.util.logging.FileHandler.level",
		SinkError:   "net.sf.l2j.commons.logging.handler.ErrorLogHandler.level",
		SinkChat:    "net.sf.l2j.commons.logging.handler.ChatLogHandler.level",
		SinkGMAudit: "net.sf.l2j.commons.logging.handler.GMAuditLogHandler.level",
		SinkItem:    "net.sf.l2j.commons.logging.handler.ItemLogHandler.level",
	} {
		if cfg.Levels[sink], err = propertyLevel(p, key, cfg.Levels[sink]); err != nil {
			return Config{}, err
		}
	}

	cfg.Patterns[SinkConsole] = p.String("java.util.logging.FileHandler.pattern", cfg.Patterns[SinkConsole])
	cfg.Patterns[SinkError] = p.String("net.sf.l2j.commons.logging.handler.ErrorLogHandler.pattern", cfg.Patterns[SinkError])
	cfg.Patterns[SinkChat] = p.String("net.sf.l2j.commons.logging.handler.ChatLogHandler.pattern", cfg.Patterns[SinkChat])
	cfg.Patterns[SinkGMAudit] = p.String("net.sf.l2j.commons.logging.handler.GMAuditLogHandler.pattern", cfg.Patterns[SinkGMAudit])
	cfg.Patterns[SinkItem] = p.String("net.sf.l2j.commons.logging.handler.ItemLogHandler.pattern", cfg.Patterns[SinkItem])

	for sink, prefix := range fileHandlerPropertyKey {
		limit, err := p.Int64(prefix+".limit", cfg.Limits[sink])
		if err != nil {
			return Config{}, err
		}
		cfg.Limits[sink] = limit

		count, err := p.Int(prefix+".count", cfg.Counts[sink])
		if err != nil {
			return Config{}, err
		}
		if count < 1 {
			count = 1
		}
		cfg.Counts[sink] = count

		cfg.Append[sink] = p.Bool(prefix+".append", cfg.Append[sink])
	}

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

	rt := &Runtime{paths: make(map[Sink]string)}
	open := func(sink Sink) (*rotatingFile, error) {
		pattern := cfg.Patterns[sink]
		if pattern == "" {
			return nil, fmt.Errorf("missing log pattern for %s", sink)
		}
		rf, err := newRotatingFile(root, pattern, cfg.Limits[sink], cfg.Counts[sink], cfg.Append[sink])
		if err != nil {
			return nil, err
		}
		rt.paths[sink] = rf.currentPath()
		rt.files = append(rt.files, rf)
		return rf, nil
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

	console := zerolog.MultiLevelWriter(
		levelWriter{Writer: stderr, Level: cfg.ConsoleLevel},
		levelWriter{Writer: consoleFile, Level: cfg.Levels[SinkConsole]},
		errorWriter{Writer: errorFile, Level: cfg.Levels[SinkError]},
	)
	rt.Logger = newLogger(cfg.Level, console)
	rt.Chat = newLogger(effectiveLevel(cfg.Level, cfg.Levels[SinkChat]), chatFile)
	rt.GMAudit = newLogger(effectiveLevel(cfg.Level, cfg.Levels[SinkGMAudit]), gmFile)
	rt.Item = newLogger(effectiveLevel(cfg.Level, cfg.Levels[SinkItem]), itemFile)

	return rt, nil
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

// Per-packet callers should use Debug so a disabled event stops before field allocation.
func newLogger(level zerolog.Level, out io.Writer) zerolog.Logger {
	return zerolog.New(out).With().Timestamp().Logger().Level(level)
}

func propertyLevel(p *config.Properties, key string, fallback zerolog.Level) (zerolog.Level, error) {
	return parseLevel(p.String(key, fallback.String()))
}

func effectiveLevel(level, handler zerolog.Level) zerolog.Level {
	if handler > level {
		return handler
	}
	return level
}

func parseLevel(s string) (zerolog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ALL", "FINEST", "FINER":
		return zerolog.TraceLevel, nil
	case "FINE", "CONFIG":
		return zerolog.DebugLevel, nil
	case "INFO":
		return zerolog.InfoLevel, nil
	case "WARNING":
		return zerolog.WarnLevel, nil
	case "SEVERE":
		return zerolog.ErrorLevel, nil
	case "OFF":
		return zerolog.Disabled, nil
	default:
		return zerolog.InfoLevel, fmt.Errorf("unsupported log level %q", s)
	}
}

type levelWriter struct {
	io.Writer
	Level zerolog.Level
}

func (w levelWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level < w.Level {
		return len(p), nil
	}
	return w.Writer.Write(p)
}

// rotatingFile reproduces java.util.logging.FileHandler's size-based rollover: writes go to
// generation 0 (%g substituted with the generation index); once its size exceeds limit, existing
// generation files shift up by one index (oldest, at count-1, is discarded) and a fresh empty
// generation 0 file is opened. limit <= 0 disables rotation entirely, matching the JDK default.
type rotatingFile struct {
	mu      sync.Mutex
	dir     string
	pattern string
	limit   int64
	count   int
	file    *os.File
	size    int64
}

func newRotatingFile(root, pattern string, limit int64, count int, appendMode bool) (*rotatingFile, error) {
	if count < 1 {
		count = 1
	}
	dir := filepath.Join(root, filepath.Dir(filepath.FromSlash(pattern)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rf := &rotatingFile{dir: root, pattern: pattern, limit: limit, count: count}
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	name := rf.generationPath(0)
	file, err := os.OpenFile(name, flags, 0o644)
	if err != nil {
		return nil, err
	}
	size := int64(0)
	if appendMode {
		if info, err := file.Stat(); err == nil {
			size = info.Size()
		}
	}
	rf.file = file
	rf.size = size
	return rf, nil
}

func (rf *rotatingFile) generationPath(generation int) string {
	name := strings.ReplaceAll(rf.pattern, "%g", strconv.Itoa(generation))
	name = strings.ReplaceAll(name, "%u", "0")
	return filepath.Join(rf.dir, filepath.FromSlash(name))
}

func (rf *rotatingFile) currentPath() string {
	return rf.generationPath(0)
}

func (rf *rotatingFile) Write(p []byte) (int, error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	n, err := rf.file.Write(p)
	rf.size += int64(n)
	if err == nil && rf.limit > 0 && rf.size >= rf.limit {
		if rerr := rf.rotate(); rerr != nil {
			return n, rerr
		}
	}
	return n, err
}

// rotate shifts generation files up by one index, discarding whatever occupied the last
// generation, then reopens an empty generation 0 for continued writing.
func (rf *rotatingFile) rotate() error {
	if err := rf.file.Close(); err != nil {
		return err
	}
	for g := rf.count - 1; g >= 1; g-- {
		_ = os.Remove(rf.generationPath(g))
		if err := os.Rename(rf.generationPath(g-1), rf.generationPath(g)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	file, err := os.OpenFile(rf.generationPath(0), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	rf.file = file
	rf.size = 0
	return nil
}

func (rf *rotatingFile) Close() error {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.file.Close()
}

type errorWriter struct {
	io.Writer
	Level zerolog.Level
}

func (w errorWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level < w.Level {
		return len(p), nil
	}
	var event map[string]json.RawMessage
	if err := json.Unmarshal(p, &event); err != nil {
		return len(p), nil
	}
	errValue, ok := event[zerolog.ErrorFieldName]
	if !ok || bytes.Equal(errValue, []byte("null")) {
		return len(p), nil
	}
	return w.Writer.Write(p)
}
