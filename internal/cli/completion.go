package cli

import (
	"errors"
	"os"
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

func registerCompletions(cmd *cobra.Command, c completer) {
	cmd.ValidArgsFunction = c.completeArgs
	err := errors.Join(
		cmd.RegisterFlagCompletionFunc(conventionKey, completeConventions),
		cmd.MarkFlagFilename("config", "yaml", "yml"),
	)
	if err != nil {
		panic("casemv: register completions: " + err.Error())
	}
}

func (c completer) completeArgs(
	cmd *cobra.Command, args []string, toComplete string,
) ([]string, cobra.ShellCompDirective) {
	defaultConvention, isText := c.load(cmd)
	switch {
	case len(args) == 0:
		return completeFirstArgument(toComplete, defaultConvention != "" && !isText)
	case len(args) == 1 && !isText && isConvention(args[0]):
		return nil, cobra.ShellCompDirectiveFilterDirs
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// load reads the settings completion depends on. A config that fails to load
// leaves no default convention.
func (c completer) load(cmd *cobra.Command) (defaultConvention string, isText bool) {
	err := loadConfig(c.v, cmd, *c.configFile)
	isText = c.v.GetBool("text")
	if err != nil {
		return "", isText
	}
	return c.v.GetString(conventionKey), isText
}

// completeFirstArgument offers conventions and, when a default convention lets
// the argument be the target, directories. Once no convention matches, the
// shell's own directory completion takes over: it handles paths, ~, and quoting.
func completeFirstArgument(toComplete string, mayBeTarget bool) ([]string, cobra.ShellCompDirective) {
	conventions := withPrefix(conventionNames(), toComplete)
	if !mayBeTarget {
		return conventions, cobra.ShellCompDirectiveNoFileComp
	}
	if len(conventions) == 0 {
		return nil, cobra.ShellCompDirectiveFilterDirs
	}
	return append(conventions, directoriesWithPrefix(toComplete)...), cobra.ShellCompDirectiveNoFileComp
}

func completeConventions(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return withPrefix(conventionNames(), toComplete), cobra.ShellCompDirectiveNoFileComp
}

func isConvention(name string) bool {
	_, err := casing.ParseConvention(name)
	return err == nil
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

// directoriesWithPrefix lists visible directories in the working directory,
// following symlinks, each with a trailing slash.
func directoriesWithPrefix(prefix string) []string {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		name := entry.Name()
		isHidden := strings.HasPrefix(name, ".")
		if isHidden || !strings.HasPrefix(name, prefix) {
			continue
		}
		if info, err := os.Stat(name); err == nil && info.IsDir() {
			dirs = append(dirs, name+"/")
		}
	}
	return dirs
}
