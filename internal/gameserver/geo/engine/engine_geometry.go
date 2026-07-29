package engine

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

func localCell(geo int) int {
	return geo % block.CellsX
}

func alignCell(world int) int {
	return world &^ (block.CellSize - 1)
}

type moveDirection struct {
	stepX   int
	stepY   int
	signumX int
	signumY int
	offsetX int
	offsetY int
	dirX    block.NSWE
	dirY    block.NSWE
}

func moveDirectionFor(gdx, gdy int) moveDirection {
	signumX := cmp(gdx)
	signumY := cmp(gdy)
	return moveDirection{
		stepX:   signumX * block.CellSize,
		stepY:   signumY * block.CellSize,
		signumX: signumX,
		signumY: signumY,
		offsetX: ternary(signumX >= 0, block.CellSize-1, 0),
		offsetY: ternary(signumY >= 0, block.CellSize-1, 0),
		dirX:    directionFlag(signumX, block.West, block.East),
		dirY:    directionFlag(signumY, block.North, block.South),
	}
}

func cmp(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	default:
		return 0
	}
}

func directionFlag(signum int, negative, positive block.NSWE) block.NSWE {
	switch {
	case signum < 0:
		return negative
	case signum > 0:
		return positive
	default:
		return 0
	}
}

func ternary(ok bool, yes, no int) int {
	if ok {
		return yes
	}
	return no
}
