package pet

import (
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

const baseWeightLimit = 34500

// Config holds the operator-tuned pet rates and inventory limits.
type Config struct {
	ExpRate               float64
	SinEaterExpRate       float64
	WeightLimitMultiplier float64
	InventorySlots        int
}

// DefaultConfig returns the shipped pet configuration defaults.
func DefaultConfig() Config {
	return Config{
		ExpRate:               1,
		SinEaterExpRate:       1,
		WeightLimitMultiplier: 1,
		InventorySlots:        12,
	}
}

// ConfigFromProperties reads pet configuration from server.properties and
// players.properties.
func ConfigFromProperties(serverProps, playersProps *config.Properties) (Config, error) {
	cfg := DefaultConfig()

	server := config.NewFields(serverProps, "pet server config")
	cfg.ExpRate = server.Float64("PetXpRate", cfg.ExpRate)
	cfg.SinEaterExpRate = server.Float64("SinEaterXpRate", cfg.SinEaterExpRate)
	if err := server.Err(); err != nil {
		return Config{}, err
	}

	players := config.NewFields(playersProps, "pet player config")
	cfg.InventorySlots = players.Int("MaximumSlotsForPet", cfg.InventorySlots)
	cfg.WeightLimitMultiplier = players.Float64("WeightLimit", cfg.WeightLimitMultiplier)
	if err := players.Err(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// ScaledExpGain applies cfg's configured experience rate to rawExp.
func (c Config) ScaledExpGain(npcID int, rawExp int64) int64 {
	return ScaledExpGain(npcID, rawExp, c.ExpRate, c.SinEaterExpRate)
}

// InventoryLimits returns the configured pet slot limit and CON-derived
// carried-weight limit.
func (c Config) InventoryLimits(con int) (slots, weight int) {
	idx := statbonus.ClampIndex(con)
	return c.InventorySlots, int(baseWeightLimit * statbonus.CONBonus[idx] * c.WeightLimitMultiplier)
}
