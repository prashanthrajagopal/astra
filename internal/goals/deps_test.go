package goals

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseTextArrayLiteralEmpty(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{"empty string", "", nil, false},
		{"NULL", "NULL", nil, false},
		{"empty array", "{}", []string{}, false},
		{"single element", "{abc}", []string{"abc"}, false},
		{"two elements", "{a,b}", []string{"a", "b"}, false},
		{"three elements", "{x,y,z}", []string{"x", "y", "z"}, false},
		{"uuid values", "{550e8400-e29b-41d4-a716-446655440000,550e8400-e29b-41d4-a716-446655440001}",
			[]string{"550e8400-e29b-41d4-a716-446655440000", "550e8400-e29b-41d4-a716-446655440001"}, false},
		{"invalid no braces", "abc", nil, true},
		{"invalid only open", "{abc", nil, true},
		{"invalid only close", "abc}", nil, true},
		{"single char no braces", "x", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTextArrayLiteral(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseTextArrayLiteral(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len: got %d (%v), want %d (%v)", len(got), got, len(tc.want), tc.want)
			}
			for i, v := range got {
				if v != tc.want[i] {
					t.Errorf("element[%d]: got %q, want %q", i, v, tc.want[i])
				}
			}
		})
	}
}

func TestUUIDSliceToArrayLiteral(t *testing.T) {
	id1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	id2 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	tests := []struct {
		name string
		ids  []uuid.UUID
		want string
	}{
		{"empty slice", []uuid.UUID{}, "{}"},
		{"nil slice", nil, "{}"},
		{"single id", []uuid.UUID{id1}, "{550e8400-e29b-41d4-a716-446655440000}"},
		{"two ids", []uuid.UUID{id1, id2}, "{550e8400-e29b-41d4-a716-446655440000,550e8400-e29b-41d4-a716-446655440001}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := uuidSliceToArrayLiteral(tc.ids)
			if got != tc.want {
				t.Errorf("uuidSliceToArrayLiteral(%v) = %q, want %q", tc.ids, got, tc.want)
			}
		})
	}
}

func TestParseTextArrayLiteralRoundTrip(t *testing.T) {
	ids := []uuid.UUID{
		uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8"),
	}
	literal := uuidSliceToArrayLiteral(ids)
	parsed, err := parseTextArrayLiteral(literal)
	if err != nil {
		t.Fatalf("parseTextArrayLiteral error: %v", err)
	}
	if len(parsed) != len(ids) {
		t.Fatalf("len: got %d, want %d", len(parsed), len(ids))
	}
	for i, s := range parsed {
		got, err := uuid.Parse(s)
		if err != nil {
			t.Fatalf("uuid.Parse(%q): %v", s, err)
		}
		if got != ids[i] {
			t.Errorf("element[%d]: got %v, want %v", i, got, ids[i])
		}
	}
}
