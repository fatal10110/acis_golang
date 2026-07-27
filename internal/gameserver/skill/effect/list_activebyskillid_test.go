package effect

import "testing"

func TestListActiveBySkillIDFindsActiveEffectLevel(t *testing.T) {
	list := NewList(nil)
	list.Add(&Effect{Skill: Skill{ID: 1285}, Level: 3, Type: TypeBuff})

	level, ok := list.ActiveBySkillID(1285)
	if !ok || level != 3 {
		t.Fatalf("ActiveBySkillID(1285) = (%d, %v), want (3, true)", level, ok)
	}
}

func TestListActiveBySkillIDMissReportsNotFound(t *testing.T) {
	list := NewList(nil)
	list.Add(&Effect{Skill: Skill{ID: 1285}, Level: 3, Type: TypeBuff})

	level, ok := list.ActiveBySkillID(5104)
	if ok || level != 0 {
		t.Fatalf("ActiveBySkillID(5104) = (%d, %v), want (0, false)", level, ok)
	}
}

func TestListActiveBySkillIDIgnoresRemovedEffect(t *testing.T) {
	list := NewList(nil)
	e := &Effect{Skill: Skill{ID: 5104}, Level: 2, Type: TypeBuff}
	list.Add(e)
	list.Remove(e)

	if level, ok := list.ActiveBySkillID(5104); ok {
		t.Fatalf("ActiveBySkillID(5104) after Remove = (%d, %v), want ok=false", level, ok)
	}
}

func TestListActiveBySkillIDNilListReportsNotFound(t *testing.T) {
	var list *List
	if level, ok := list.ActiveBySkillID(5104); ok || level != 0 {
		t.Fatalf("nil list ActiveBySkillID = (%d, %v), want (0, false)", level, ok)
	}
}
