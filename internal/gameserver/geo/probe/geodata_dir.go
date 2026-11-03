package probe

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveDir returns the geodata directory for geoengine.properties'
// GeoDataPath value configured: empty means <dataRoot>/data/geodata, a
// relative path under data/ is taken from dataRoot, and any other path is
// used as given.
func ResolveDir(dataRoot, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return filepath.Join(dataRoot, "data", "geodata")
	}
	if filepath.IsAbs(configured) {
		return configured
	}

	clean := filepath.Clean(configured)
	if clean == "data" || strings.HasPrefix(clean, "data"+string(os.PathSeparator)) {
		return filepath.Join(dataRoot, clean)
	}
	return clean
}
