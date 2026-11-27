// Package cli implements the option syntax used by mruby's commands:
// clustered short options (-cv), attached option arguments (-eCODE, -rLIB),
// long options (--verbose), and the "--" terminator.
package cli

import (
	"fmt"
	"strings"
)

// Option describes one accepted command-line option. Short is zero for a
// long-only option; Long is empty for a short-only option. HasArg says the
// option consumes the rest of its token or the next argument.
type Option struct {
	Short  byte
	Long   string
	HasArg bool
}

// Match is one parsed option occurrence, in command-line order.
type Match struct {
	Option Option
	Arg    string
}

// ParseError reports an unknown option or a missing option argument.
type ParseError struct {
	// Name is the option name without leading dashes.
	Name string
	// Long reports that Name came from a --long option.
	Long bool
	// Missing reports that Option needed an argument but none followed.
	Missing bool
	// Option is set when Missing is true.
	Option Option
}

func (e *ParseError) Error() string {
	if e.Missing {
		if e.Option.Long != "" {
			return fmt.Sprintf("option --%s requires an argument", e.Option.Long)
		}
		return fmt.Sprintf("option -%c requires an argument", e.Option.Short)
	}
	if e.Long {
		return fmt.Sprintf("unknown option --%s", e.Name)
	}
	return fmt.Sprintf("unknown option -%s", e.Name)
}

// Parse reads args using specs. It returns matched options in order and the
// remaining arguments starting at the first non-option (or after "--"). When
// an option is invalid, the matches parsed before it are returned together
// with the error so callers can act on them in command-line order.
func Parse(args []string, specs []Option) ([]Match, []string, error) {
	byShort := make(map[byte]Option, len(specs))
	byLong := make(map[string]Option, len(specs))
	for _, spec := range specs {
		if spec.Short != 0 {
			byShort[spec.Short] = spec
		}
		if spec.Long != "" {
			byLong[spec.Long] = spec
		}
	}

	var matches []Match
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return matches, args[i+1:], nil
		}
		if len(arg) < 2 || arg[0] != '-' {
			return matches, args[i:], nil
		}
		if strings.HasPrefix(arg, "--") {
			name := arg[2:]
			spec, ok := byLong[name]
			if !ok {
				return matches, nil, &ParseError{Name: name, Long: true}
			}
			if !spec.HasArg {
				matches = append(matches, Match{Option: spec})
				continue
			}
			if i+1 >= len(args) {
				return matches, nil, &ParseError{Missing: true, Option: spec}
			}
			i++
			matches = append(matches, Match{Option: spec, Arg: args[i]})
			continue
		}

		cluster := arg[1:]
		for j := 0; j < len(cluster); j++ {
			short := cluster[j]
			spec, ok := byShort[short]
			if !ok {
				return matches, nil, &ParseError{Name: string(short)}
			}
			if !spec.HasArg {
				matches = append(matches, Match{Option: spec})
				continue
			}
			if j+1 < len(cluster) {
				matches = append(matches, Match{Option: spec, Arg: cluster[j+1:]})
				break
			}
			if i+1 >= len(args) {
				return matches, nil, &ParseError{Missing: true, Option: spec}
			}
			i++
			matches = append(matches, Match{Option: spec, Arg: args[i]})
			break
		}
	}
	return matches, nil, nil
}
