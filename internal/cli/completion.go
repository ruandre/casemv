package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ruandre/casemv/internal/casing"
)

// completer answers shell completion requests. A config it cannot load counts
// as no config: completion must never fail where a run would explain why.
type completer struct {
	v          *viper.Viper
	configFile *string
}

func (c completer) completeArgs(
	cmd *cobra.Command, args []string, toComplete string,
) ([]string, cobra.ShellCompDirective) {
	isText := c.isText(cmd)
	switch {
	case len(args) == 0:
		return withPrefix(conventionNames(), toComplete), cobra.ShellCompDirectiveNoFileComp
	case len(args) == 1 && !isText:
		return nil, cobra.ShellCompDirectiveFilterDirs
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// isText reports text mode, which takes no target. Bindings made before a
// config error still answer, so the error itself is not needed.
func (c completer) isText(cmd *cobra.Command) bool {
	//nolint:errcheck,gosec // a config that fails to load must not break completion
	loadConfig(c.v, cmd, *c.configFile)
	return c.v.GetBool("text")
}

func withPrefix(candidates []string, prefix string) []string {
	var matches []string
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, prefix) {
			matches = append(matches, candidate)
		}
	}
	return matches
}

func conventionNames() []string {
	conventions := casing.Conventions()
	names := make([]string, len(conventions))
	for i, c := range conventions {
		names[i] = string(c)
	}
	return names
}
