package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
)

// TestLoadLotteryConfig pins the server.properties AllowLottery switch
// (default on) and the events.properties Lottery* keys, with their defaults
// (50000, 2000, 0.6, 0.2, 0.2, 200); a malformed value, or a whole number
// outside the int range, fails boot.
func TestLoadLotteryConfig(t *testing.T) {
	def := lottery.Config{Enabled: true, Prize: 50000, TicketPrice: 2000, FiveNumberRate: 0.6, FourNumberRate: 0.2, ThreeNumberRate: 0.2, TwoAndOneNumberPrize: 200}
	for _, tc := range []struct {
		name    string
		server  string
		events  string
		want    lottery.Config
		wantErr bool
	}{
		{name: "defaults", want: def},
		{
			name:   "set",
			server: "AllowLottery = False\n",
			events: "LotteryPrize = 70000\nLotteryTicketPrice = 500\nLottery5NumberRate = 0.5\nLottery4NumberRate = 0.3\nLottery3NumberRate = 0.1\nLottery2and1NumberPrize = 150\n",
			want:   lottery.Config{Prize: 70000, TicketPrice: 500, FiveNumberRate: 0.5, FourNumberRate: 0.3, ThreeNumberRate: 0.1, TwoAndOneNumberPrize: 150},
		},
		{name: "malformed rate", events: "Lottery5NumberRate = most\n", wantErr: true},
		{name: "prize past int", events: "LotteryPrize = 2147483648\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			paths := gameServerPaths{ConfigPath: filepath.Join(dir, "server.properties"), EventsConfigPath: filepath.Join(dir, "events.properties")}
			for path, body := range map[string]string{paths.ConfigPath: tc.server, paths.EventsConfigPath: tc.events} {
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := loadLotteryConfig(paths)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("loadLotteryConfig() = %+v, want an error", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("loadLotteryConfig() = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
