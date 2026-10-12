package castle

import (
	"slices"

	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
)

// SpawnArtifacts places every castle's holy artifacts once, at boot,
// before any player connects. spawn places one and returns the object id
// of the NPC it placed, false when it placed none. Each castle keeps the
// ids of its placed artifacts. It returns how many were placed.
func (m *Manager) SpawnArtifacts(spawn func(castledata.Artifact) (int32, bool)) int {
	placed := 0
	for _, c := range m.order {
		for _, a := range c.Artifacts {
			id, ok := spawn(a)
			if !ok {
				continue
			}
			c.mu.Lock()
			c.artifacts = append(c.artifacts, id)
			c.mu.Unlock()
			placed++
		}
	}
	return placed
}

// ArtifactIDs returns the object ids of the castle's placed artifacts.
func (c *Castle) ArtifactIDs() []int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.artifacts)
}

// ByArtifact returns the castle that placed the NPC objectID as one of its
// holy artifacts. Another NPC of an artifact's template belongs to no
// castle by being one.
func (m *Manager) ByArtifact(objectID int32) (*Castle, bool) {
	if m == nil {
		return nil, false
	}
	for _, c := range m.order {
		c.mu.Lock()
		placed := slices.Contains(c.artifacts, objectID)
		c.mu.Unlock()
		if placed {
			return c, true
		}
	}
	return nil, false
}

// ArtifactNPC reports whether npcID is the template of one of the castle's
// artifacts.
func (c *Castle) ArtifactNPC(npcID int) bool {
	return slices.ContainsFunc(c.Artifacts, func(a castledata.Artifact) bool { return a.NPCID == npcID })
}
