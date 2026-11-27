package cli

import (
	"reflect"
	"testing"
)

var testSpecs = []Option{
	{Short: 'b'},
	{Short: 'c'},
	{Short: 'd'},
	{Short: 'e', HasArg: true},
	{Short: 'h'},
	{Short: 'r', HasArg: true},
	{Short: 'v'},
	{Long: "verbose"},
	{Long: "version"},
	{Long: "copyright"},
}

func names(matches []Match) []string {
	if len(matches) == 0 {
		return nil
	}
	result := make([]string, len(matches))
	for i, match := range matches {
		if match.Option.Short != 0 {
			result[i] = string(match.Option.Short)
			if match.Arg != "" {
				result[i] += "=" + match.Arg
			}
			continue
		}
		result[i] = "--" + match.Option.Long
		if match.Arg != "" {
			result[i] += "=" + match.Arg
		}
	}
	return result
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
		rest []string
	}{
		{name: "plain", args: []string{"-v"}, want: []string{"v"}},
		{name: "cluster", args: []string{"-cvd"}, want: []string{"c", "v", "d"}},
		{name: "attached arg", args: []string{"-e1+1"}, want: []string{"e=1+1"}},
		{name: "separate arg", args: []string{"-e", "1+1"}, want: []string{"e=1+1"}},
		{name: "cluster before arg", args: []string{"-de1+1"}, want: []string{"d", "e=1+1"}},
		{name: "arg takes rest", args: []string{"-ed"}, want: []string{"e=d"}},
		{name: "attached library", args: []string{"-rlib.rb"}, want: []string{"r=lib.rb"}},
		{name: "long", args: []string{"--verbose"}, want: []string{"--verbose"}},
		{name: "long and short", args: []string{"--verbose", "-v"}, want: []string{"--verbose", "v"}},
		{name: "terminator", args: []string{"-v", "--", "-x"}, want: []string{"v"}, rest: []string{"-x"}},
		{name: "dash is program", args: []string{"-v", "-"}, want: []string{"v"}, rest: []string{"-"}},
		{name: "program and args", args: []string{"-v", "script.rb", "-x"}, want: []string{"v"}, rest: []string{"script.rb", "-x"}},
		{name: "no args", args: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matches, rest, err := Parse(test.args, testSpecs)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := names(matches); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("matches = %v, want %v", got, test.want)
			}
			if !reflect.DeepEqual(rest, test.rest) {
				t.Fatalf("rest = %v, want %v", rest, test.rest)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	_, _, err := Parse([]string{"-z"}, testSpecs)
	if err == nil {
		t.Fatal("expected unknown short option")
	}
	parseErr, ok := err.(*ParseError)
	if !ok || parseErr.Long || parseErr.Name != "z" {
		t.Fatalf("unexpected error: %#v", err)
	}

	_, _, err = Parse([]string{"--unknown"}, testSpecs)
	if err == nil {
		t.Fatal("expected unknown long option")
	}
	parseErr, ok = err.(*ParseError)
	if !ok || !parseErr.Long || parseErr.Name != "unknown" {
		t.Fatalf("unexpected error: %#v", err)
	}

	_, _, err = Parse([]string{"-e"}, testSpecs)
	if err == nil {
		t.Fatal("expected missing argument")
	}
	parseErr, ok = err.(*ParseError)
	if !ok || !parseErr.Missing || parseErr.Option.Short != 'e' {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestParseReturnsEarlierMatchesOnError(t *testing.T) {
	matches, _, err := Parse([]string{"-v", "-z"}, testSpecs)
	if err == nil {
		t.Fatal("expected unknown option")
	}
	if len(matches) != 1 || matches[0].Option.Short != 'v' {
		t.Fatalf("matches = %v, want the earlier -v", names(matches))
	}

	matches, _, err = Parse([]string{"-v", "-e"}, testSpecs)
	if err == nil {
		t.Fatal("expected missing argument")
	}
	if len(matches) != 1 || matches[0].Option.Short != 'v' {
		t.Fatalf("matches = %v, want the earlier -v", names(matches))
	}
}
