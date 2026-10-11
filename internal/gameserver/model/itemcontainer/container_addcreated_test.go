package itemcontainer

import (
	"errors"
	"testing"
)

// TestContainer_AddCreated pins when AddCreated takes an object id: never
// for units joining a held stack, exactly once for a new instance, and a
// refused id (an error or zero) leaves the container unchanged.
func TestContainer_AddCreated(t *testing.T) {
	t.Parallel()
	errExhausted := errors.New("exhausted")
	tests := []struct {
		name       string
		held       bool // the container already holds 100 adena under 0x20000001
		templateID int32
		id         int32
		idErr      error
		wantCalls  int
		wantJoined bool
		wantID     int32 // 0: nil result
		wantCount  int
		wantSize   int
		wantErr    error
	}{
		{name: "held stack", held: true, templateID: adenaTemplateID, id: 0x20000002, wantJoined: true, wantID: 0x20000001, wantCount: 150, wantSize: 1},
		{name: "new instance", templateID: potionTemplateID, id: 0x20000002, wantCalls: 1, wantID: 0x20000002, wantCount: 50, wantSize: 1},
		{name: "non-stackable new instance", held: true, templateID: daggerTemplateID, id: 0x20000002, wantCalls: 1, wantID: 0x20000002, wantCount: 1, wantSize: 2},
		{name: "id error", held: true, templateID: daggerTemplateID, idErr: errExhausted, wantCalls: 1, wantSize: 1, wantErr: errExhausted},
		{name: "zero id", templateID: potionTemplateID, wantCalls: 1, wantErr: errNoObjectID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newTestContainer()
			if tt.held {
				c.AddNew(adenaTemplateID, 100, 0x20000001)
			}
			calls := 0
			got, joined, err := c.addCreated(tt.templateID, 50, func() (int32, error) {
				calls++
				return tt.id, tt.idErr
			})
			if calls != tt.wantCalls {
				t.Errorf("nextID calls = %d, want %d", calls, tt.wantCalls)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			if joined != tt.wantJoined {
				t.Errorf("joined = %v, want %v", joined, tt.wantJoined)
			}
			switch {
			case tt.wantID == 0 && got != nil:
				t.Errorf("result = %+v, want nil", got)
			case tt.wantID != 0 && (got == nil || got.ObjectID != tt.wantID || got.Count != tt.wantCount):
				t.Errorf("result = %+v, want object %d count %d", got, tt.wantID, tt.wantCount)
			}
			if c.Size() != tt.wantSize {
				t.Errorf("Size() = %d, want %d", c.Size(), tt.wantSize)
			}
			if tt.held && tt.templateID != adenaTemplateID {
				if adena := c.ItemByObjectID(0x20000001); adena == nil || adena.Count != 100 {
					t.Errorf("held adena = %+v, want 100 untouched", adena)
				}
			}
		})
	}
}
