package main

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
)

// chatConfig holds the server.properties chat settings: whether chat is
// logged (LogChat), whether bot whispers are dropped (L2WalkerProtection),
// and the per-client reuse delays of the general and shout channels
// (GlobalChatTime), the trade channel (TradeChatTime) and the hero channel
// (HeroVoiceTime), in milliseconds.
type chatConfig struct {
	LogChat          bool
	WalkerProtection bool
	GlobalDelay      time.Duration
	TradeDelay       time.Duration
	HeroVoiceDelay   time.Duration
}

func loadChatConfig(paths gameServerPaths) (chatConfig, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return chatConfig{}, err
	}
	f := config.NewFields(props, "chat")
	ms := func(key string, def int) time.Duration { return time.Duration(f.Int(key, def)) * time.Millisecond }
	cfg := chatConfig{
		LogChat:          f.Bool("LogChat", false),
		WalkerProtection: f.Bool("L2WalkerProtection", false),
		GlobalDelay:      ms("GlobalChatTime", 0),
		TradeDelay:       ms("TradeChatTime", 0),
		HeroVoiceDelay:   ms("HeroVoiceTime", 10000),
	}
	return cfg, f.Err()
}
