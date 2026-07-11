package item

// Augmentation is a life-stone bonus applied to one item instance: the
// encoded stat-bonus id (the augmentations table's "attributes" column)
// plus the skill it grants, if any (SkillID zero means none). Resolving
// Attributes/SkillID into actual stat boni and a skill definition is the
// augmentation catalog and skill table's job, not this package's — this
// type only carries the persisted assignment.
type Augmentation struct {
	Attributes int32
	SkillID    int32
	SkillLevel int32
}
