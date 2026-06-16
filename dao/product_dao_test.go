package dao

import (
	"testing"
)

func TestTagsRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  string
	}{
		{"通常タグ", []string{"スニーカー", "Nike"}, `["スニーカー","Nike"]`},
		{"空スライス", []string{}, "[]"},
		{"nil", nil, "[]"},
		{"単一要素", []string{"靴"}, `["靴"]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tagsToJSON(tc.input)
			if got != tc.want {
				t.Errorf("tagsToJSON(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTagsFromJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"通常JSON", `["a","b"]`, []string{"a", "b"}},
		{"空配列", "[]", []string{}},
		{"空文字列", "", []string{}},
		{"不正JSON", "not-json", []string{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tagsFromJSON(tc.input)
			if len(got) != len(tc.want) {
				t.Errorf("tagsFromJSON(%q) len=%d, want %d", tc.input, len(got), len(tc.want))
				return
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("tagsFromJSON(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTagsRoundTripConsistency(t *testing.T) {
	original := []string{"タグA", "タグB", "タグC"}
	json := tagsToJSON(original)
	restored := tagsFromJSON(json)

	if len(restored) != len(original) {
		t.Fatalf("round-trip len mismatch: got %d, want %d", len(restored), len(original))
	}
	for i := range original {
		if restored[i] != original[i] {
			t.Errorf("round-trip[%d] = %q, want %q", i, restored[i], original[i])
		}
	}
}
