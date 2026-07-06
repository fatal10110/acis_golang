package player

import "testing"

func TestParseSex(t *testing.T) {
	tests := []struct {
		in      int32
		want    Sex
		wantErr bool
	}{
		{0, SexMale, false},
		{1, SexFemale, false},
		{2, 0, true},
		{-1, 0, true},
	}
	for _, tt := range tests {
		got, err := ParseSex(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseSex(%d) = %v, nil; want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSex(%d) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseSex(%d) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
