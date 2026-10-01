package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/rs/zerolog"
)

func TestLoadPvPFlagOptionsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte(`
PvPVsNormalTime = 1234
PvPVsPvPTime = 5678
CanGMDropEquipment = True
`), 0o600); err != nil {
		t.Fatal(err)
	}

	opts, err := loadPvPFlagOptions(gameServerPaths{PlayersConfigPath: configPath}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadPvPFlagOptions() error = %v", err)
	}
	if opts.Normal != 1234*time.Millisecond || opts.Flagged != 5678*time.Millisecond {
		t.Fatalf("durations = normal %s flagged %s, want 1234ms/5678ms", opts.Normal, opts.Flagged)
	}
	if len(opts.UnsupportedKeys) != 1 || opts.UnsupportedKeys[0] != "CanGMDropEquipment" {
		t.Fatalf("UnsupportedKeys = %v, want [CanGMDropEquipment]", opts.UnsupportedKeys)
	}
}

func TestLoadAutoLearnSkillsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("AutoLearnSkills = True\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadAutoLearnSkills(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadAutoLearnSkills() error = %v", err)
	}
	if !got {
		t.Fatal("loadAutoLearnSkills() = false, want true")
	}
}

func TestLoadKarmaServiceGatesUsesPlayersProperties(t *testing.T) {
	dir := t.TempDir()
	set := filepath.Join(dir, "set.properties")
	if err := os.WriteFile(set, []byte("KarmaPlayerCanShop = True\nKarmaPlayerCanUseGK = True\nKarmaPlayerCanUseWareHouse = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unset := filepath.Join(dir, "unset.properties")
	if err := os.WriteFile(unset, []byte("AutoLearnSkills = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadKarmaServiceGates(gameServerPaths{PlayersConfigPath: set})
	if err != nil {
		t.Fatalf("loadKarmaServiceGates() error = %v", err)
	}
	if want := (karmaServiceGates{CanShop: true, CanUseGK: true, CanUseWareHouse: false}); got != want {
		t.Fatalf("loadKarmaServiceGates() = %+v, want %+v", got, want)
	}
	got, err = loadKarmaServiceGates(gameServerPaths{PlayersConfigPath: unset})
	if err != nil {
		t.Fatalf("loadKarmaServiceGates() defaults error = %v", err)
	}
	if want := (karmaServiceGates{CanShop: false, CanUseGK: false, CanUseWareHouse: true}); got != want {
		t.Fatalf("loadKarmaServiceGates() defaults = %+v, want %+v", got, want)
	}
}

// TestLoadKarmaPlayerCanTradeUsesPlayersProperties pins the trade karma
// switch: players.properties turns it off, and a file without the key keeps
// the shipped default of allowing karma trades.
func TestLoadKarmaPlayerCanTradeUsesPlayersProperties(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"KarmaPlayerCanTrade = False\n", false},
		{"KarmaPlayerCanTrade = True\n", true},
		{"KarmaPlayerCanShop = False\n", true},
	} {
		configPath := filepath.Join(t.TempDir(), "players.properties")
		if err := os.WriteFile(configPath, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadKarmaPlayerCanTrade(gameServerPaths{PlayersConfigPath: configPath})
		if err != nil {
			t.Fatalf("loadKarmaPlayerCanTrade(%q) error = %v", tc.body, err)
		}
		if bool(got) != tc.want {
			t.Fatalf("loadKarmaPlayerCanTrade(%q) = %v, want %v", tc.body, got, tc.want)
		}
	}
}

func TestLoadDeathPenaltyChanceUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("DeathPenaltyChance = 73\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadDeathPenaltyChance(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadDeathPenaltyChance() error = %v", err)
	}
	if got != 73 {
		t.Fatalf("loadDeathPenaltyChance() = %d, want 73", got)
	}
}

func TestLoadDeathPenaltyChanceDefaultsToTwenty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadDeathPenaltyChance(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadDeathPenaltyChance() error = %v", err)
	}
	if got != 20 {
		t.Fatalf("loadDeathPenaltyChance() = %d, want 20", got)
	}
}

func TestLoadMaxBuffsAmountUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("MaxBuffsAmount = 24\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMaxBuffsAmount(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMaxBuffsAmount() error = %v", err)
	}
	if got != 24 {
		t.Fatalf("loadMaxBuffsAmount() = %d, want 24", got)
	}
}

func TestLoadMaxBuffsAmountDefaultsToTwenty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMaxBuffsAmount(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMaxBuffsAmount() error = %v", err)
	}
	if got != 20 {
		t.Fatalf("loadMaxBuffsAmount() = %d, want 20", got)
	}
}

func TestLoadPerfectShieldBlockRateUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("PerfectShieldBlockRate = 15\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadPerfectShieldBlockRate(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPerfectShieldBlockRate() error = %v", err)
	}
	if got != 15 {
		t.Fatalf("loadPerfectShieldBlockRate() = %d, want 15", got)
	}
}

func TestLoadPerfectShieldBlockRateDefaultsToFive(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadPerfectShieldBlockRate(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPerfectShieldBlockRate() error = %v", err)
	}
	if got != 5 {
		t.Fatalf("loadPerfectShieldBlockRate() = %d, want 5", got)
	}
}

func TestLoadMagicFailuresUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("MagicFailures = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMagicFailures(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMagicFailures() error = %v", err)
	}
	if got {
		t.Fatalf("loadMagicFailures() = %v, want false", got)
	}
}

func TestLoadMagicFailuresDefaultsToTrue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadMagicFailures(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadMagicFailures() error = %v", err)
	}
	if !got {
		t.Fatalf("loadMagicFailures() = %v, want true", got)
	}
}

func TestLoadCancelLesserEffectUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("CancelLesserEffect = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadCancelLesserEffect(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadCancelLesserEffect() error = %v", err)
	}
	if got {
		t.Fatalf("loadCancelLesserEffect() = %v, want false", got)
	}
}

func TestLoadCancelLesserEffectDefaultsToTrue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadCancelLesserEffect(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadCancelLesserEffect() error = %v", err)
	}
	if !got {
		t.Fatalf("loadCancelLesserEffect() = %v, want true", got)
	}
}

func TestLoadStoreSkillCooltimeUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("StoreSkillCooltime = False\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStoreSkillCooltime(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadStoreSkillCooltime() error = %v", err)
	}
	if got {
		t.Fatalf("loadStoreSkillCooltime() = %v, want false", got)
	}
}

func TestLoadStoreSkillCooltimeDefaultsToTrue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStoreSkillCooltime(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadStoreSkillCooltime() error = %v", err)
	}
	if !got {
		t.Fatalf("loadStoreSkillCooltime() = %v, want true", got)
	}
}

func TestLoadPlayerSpawnProtectionUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("PlayerSpawnProtection = 12\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadPlayerSpawnProtection(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPlayerSpawnProtection() error = %v", err)
	}
	if got != playerSpawnProtection(12*time.Second) {
		t.Fatalf("loadPlayerSpawnProtection() = %v, want 12s", got)
	}
}

func TestLoadPlayerSpawnProtectionDefaultsToDisabled(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadPlayerSpawnProtection(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadPlayerSpawnProtection() error = %v", err)
	}
	if got != playerSpawnProtection(0) {
		t.Fatalf("loadPlayerSpawnProtection() = %v, want disabled", got)
	}
}

func TestLoadPetConfigUsesServerAndPlayersProperties(t *testing.T) {
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "server.properties")
	playersPath := filepath.Join(dir, "players.properties")
	if err := os.WriteFile(serverPath, []byte(`
PetXpRate = 1.5
SinEaterXpRate = 4.0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(playersPath, []byte(`
MaximumSlotsForPet = 19
WeightLimit = 1.25
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadPetConfig(gameServerPaths{ConfigPath: serverPath, PlayersConfigPath: playersPath}, zerolog.Nop())
	if err != nil {
		t.Fatalf("loadPetConfig() error = %v", err)
	}

	if got := cfg.ScaledExpGain(12077, 1000); got != 1500 {
		t.Errorf("ordinary pet configured exp = %d, want 1500", got)
	}
	if got := cfg.ScaledExpGain(12564, 1000); got != 4000 {
		t.Errorf("sin eater configured exp = %d, want 4000", got)
	}
	if cfg.InventorySlots != 19 {
		t.Errorf("pet inventory slots = %d, want 19", cfg.InventorySlots)
	}
	if cfg == pet.DefaultConfig() {
		t.Fatal("loadPetConfig returned defaults, want values from both files")
	}
}

func TestLoadInventorySlotsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("MaximumSlotsForNoDwarf = 90\nMaximumSlotsForDwarf = 117\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadInventorySlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadInventorySlots() error = %v", err)
	}
	if got != (player.InventorySlots{NoDwarf: 90, Dwarf: 117, Configured: true}) {
		t.Fatalf("loadInventorySlots() = %+v, want {NoDwarf:90 Dwarf:117}", got)
	}
}

// TestLoadInventorySlotsKeepsExplicitZero pins the reference's as-is read:
// an explicit 0 is a 0-slot base, not the shipped default.
func TestLoadInventorySlotsKeepsExplicitZero(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte("MaximumSlotsForNoDwarf = 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadInventorySlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadInventorySlots() error = %v", err)
	}
	if got != (player.InventorySlots{NoDwarf: 0, Dwarf: 100, Configured: true}) {
		t.Fatalf("loadInventorySlots() = %+v, want {NoDwarf:0 Dwarf:100 Configured:true}", got)
	}
}

func TestLoadInventorySlotsDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadInventorySlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadInventorySlots() error = %v", err)
	}
	if got != player.DefaultInventorySlots {
		t.Fatalf("loadInventorySlots() = %+v, want {NoDwarf:80 Dwarf:100}", got)
	}
}

func TestLoadStorageSlotsUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	props := "MaximumWarehouseSlotsForNoDwarf = 11\nMaximumWarehouseSlotsForDwarf = 12\n" +
		"MaximumFreightSlots = 13\nMaxPvtStoreSlotsOther = 14\nMaxPvtStoreSlotsDwarf = 15\n" +
		"DwarfRecipeLimit = 16\nCommonRecipeLimit = 0\nMaximumWarehouseSlotsForClan = 17\n"
	if err := os.WriteFile(configPath, []byte(props), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStorageSlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadStorageSlots() error = %v", err)
	}
	want := player.StorageSlots{
		WarehouseNoDwarf: 11, WarehouseDwarf: 12, Freight: 13,
		PrivateStoreNoDwarf: 14, PrivateStoreDwarf: 15,
		DwarfRecipe: 16, CommonRecipe: 0, ClanWarehouse: 17, Configured: true,
	}
	if got != want {
		t.Fatalf("loadStorageSlots() = %+v, want %+v", got, want)
	}
}

func TestLoadStorageSlotsDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadStorageSlots(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadStorageSlots() error = %v", err)
	}
	// A missing clan warehouse size is the reference's 150, not the
	// shipped file's 200.
	want := player.DefaultStorageSlots
	want.ClanWarehouse = 150
	if got != want {
		t.Fatalf("loadStorageSlots() = %+v, want %+v", got, want)
	}
}

func TestLoadEnchantConfigUsesPlayersProperties(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(configPath, []byte(`
EnchantChanceMagicWeapon = 0.5
EnchantChanceMagicWeapon15Plus = 0.25
EnchantChanceNonMagicWeapon = 0.8
EnchantChanceNonMagicWeapon15Plus = 0.45
EnchantChanceArmor = 0.7
EnchantMaxWeapon = 16
EnchantMaxArmor = 12
EnchantSafeMax = 4
EnchantSafeMaxFull = 5
`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadEnchantConfig(gameServerPaths{PlayersConfigPath: configPath})
	if err != nil {
		t.Fatalf("loadEnchantConfig() error = %v", err)
	}
	want := enchant.Config{
		ChanceMagicWeapon: 0.5, ChanceMagicWeapon15Plus: 0.25,
		ChanceWeapon: 0.8, ChanceWeapon15Plus: 0.45, ChanceArmor: 0.7,
		MaxWeapon: 16, MaxArmor: 12, SafeMax: 4, SafeMaxFull: 5,
	}
	if got != want {
		t.Fatalf("loadEnchantConfig() = %+v, want %+v", got, want)
	}

	empty := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = loadEnchantConfig(gameServerPaths{PlayersConfigPath: empty})
	if err != nil {
		t.Fatalf("loadEnchantConfig(empty) error = %v", err)
	}
	if got != enchant.DefaultConfig() {
		t.Fatalf("loadEnchantConfig(empty) = %+v, want the shipped defaults %+v", got, enchant.DefaultConfig())
	}
}

func TestLoadSubclassConfig(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.properties")
	set := filepath.Join(dir, "set.properties")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set, []byte("SubclassTime = 500\nSubClassWithoutQuests = True\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadSubclassDelay(gameServerPaths{ConfigPath: empty}); err != nil || got != subclassDelay(2000*time.Millisecond) {
		t.Fatalf("loadSubclassDelay(default) = %v, %v; want 2s", got, err)
	}
	if got, err := loadSubclassDelay(gameServerPaths{ConfigPath: set}); err != nil || got != subclassDelay(500*time.Millisecond) {
		t.Fatalf("loadSubclassDelay(set) = %v, %v; want 500ms", got, err)
	}
	if got, err := loadSubclassWithoutQuests(gameServerPaths{PlayersConfigPath: empty}); err != nil || bool(got) {
		t.Fatalf("loadSubclassWithoutQuests(default) = %v, %v; want false", got, err)
	}
	if got, err := loadSubclassWithoutQuests(gameServerPaths{PlayersConfigPath: set}); err != nil || !bool(got) {
		t.Fatalf("loadSubclassWithoutQuests(set) = %v, %v; want true", got, err)
	}
}
