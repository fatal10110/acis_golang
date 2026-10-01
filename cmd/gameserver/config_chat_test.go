package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadChatConfig pins the chat settings to server.properties with their
// shipped defaults: no chat log, no bot filter, no general or trade reuse
// delay, and 10 s between two hero lines.
func TestLoadChatConfig(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		name, props string
		want        chatConfig
	}{
		{"default", "", chatConfig{HeroVoiceDelay: 10 * time.Second}},
		{
			"set", "LogChat = True\nL2WalkerProtection = True\nGlobalChatTime = 500\nTradeChatTime = 750\nHeroVoiceTime = 0\n",
			chatConfig{LogChat: true, WalkerProtection: true, GlobalDelay: 500 * time.Millisecond, TradeDelay: 750 * time.Millisecond},
		},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadChatConfig(gameServerPaths{ConfigPath: path})
		if err != nil || got != tt.want {
			t.Errorf("%s: loadChatConfig = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}
