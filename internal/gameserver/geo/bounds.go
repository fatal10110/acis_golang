package geo

// World tile span and coordinate bounds.
const (
	TileXMin = 16
	TileXMax = 26
	TileYMin = 10
	TileYMax = 25

	TileSize  = 32768
	WorldXMin = (TileXMin - 20) * TileSize
	WorldXMax = (TileXMax-19)*TileSize - 1
	WorldYMin = (TileYMin - 18) * TileSize
	WorldYMax = (TileYMax-17)*TileSize - 1
)
