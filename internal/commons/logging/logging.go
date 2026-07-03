package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/sirupsen/logrus"
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
	Level           logrus.Level
	Patterns        map[Sink]string
	UnsupportedKeys []string
}

// Runtime owns open log files and logger instances.
type Runtime struct {
	Logger  *logrus.Logger
	Chat    *logrus.Logger
	GMAudit *logrus.Logger
	Item    *logrus.Logger

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
		Level: logrus.InfoLevel,
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

	formatter := &logrus.JSONFormatter{}
	rt.Logger = newLogger(cfg.Level, io.MultiWriter(stderr, consoleFile), formatter)
	rt.Logger.AddHook(writerHook{
		writer:    errorFile,
		formatter: formatter,
		levels:    []logrus.Level{logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel},
	})
	rt.Chat = newLogger(cfg.Level, chatFile, formatter)
	rt.GMAudit = newLogger(cfg.Level, gmFile, formatter)
	rt.Item = newLogger(cfg.Level, itemFile, formatter)

	return rt, nil
}

// InstallDefault makes runtime.Logger the package-wide logrus default.
func InstallDefault(runtime *Runtime) {
	std := logrus.StandardLogger()
	std.SetOutput(runtime.Logger.Out)
	std.SetFormatter(runtime.Logger.Formatter)
	std.SetLevel(runtime.Logger.Level)
	std.ReplaceHooks(logrus.LevelHooks{})
	for _, hooks := range runtime.Logger.Hooks {
		for _, hook := range hooks {
			std.AddHook(hook)
		}
	}
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

func newLogger(level logrus.Level, out io.Writer, formatter logrus.Formatter) *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(level)
	logger.SetOutput(out)
	logger.SetFormatter(formatter)
	return logger
}

func parseLevel(s string) (logrus.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ALL", "FINEST", "FINER":
		return logrus.TraceLevel, nil
	case "FINE", "CONFIG":
		return logrus.DebugLevel, nil
	case "INFO":
		return logrus.InfoLevel, nil
	case "WARNING":
		return logrus.WarnLevel, nil
	case "SEVERE":
		return logrus.ErrorLevel, nil
	case "OFF":
		return logrus.PanicLevel, nil
	default:
		return logrus.InfoLevel, fmt.Errorf("unsupported log level %q", s)
	}
}

func expandPattern(pattern string) string {
	pattern = strings.ReplaceAll(pattern, "%g", "0")
	pattern = strings.ReplaceAll(pattern, "%u", "0")
	return pattern
}

type writerHook struct {
	writer    io.Writer
	formatter logrus.Formatter
	levels    []logrus.Level
}

func (h writerHook) Levels() []logrus.Level {
	return h.levels
}

func (h writerHook) Fire(entry *logrus.Entry) error {
	line, err := h.formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = h.writer.Write(line)
	return err
}
