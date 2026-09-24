package callbacks

import "testing"

func TestParsePositiveInt(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int
		ok    bool
	}{
		{"1", 1, true},
		{"42", 42, true},
		{"0", 0, false},
		{"-1", 0, false},
		{"", 0, false},
		{"abc", 0, false},
		{"1.5", 0, false},
	} {
		got, ok := parsePositiveInt(tc.input)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parsePositiveInt(%q) = (%d, %v), want (%d, %v)", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}
