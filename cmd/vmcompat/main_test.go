package main

import "testing"

func TestNormalizeStderr(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "warnings removed",
			in:   "x.rb:1:1: warning: ambiguous first argument\nreal error\n",
			want: "real error",
		},
		{
			name: "plain error kept",
			in:   "x.rb:1:1: syntax error\n",
			want: "x.rb:1:1: syntax error",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeStderr(tc.in); got != tc.want {
				t.Fatalf("normalizeStderr(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
