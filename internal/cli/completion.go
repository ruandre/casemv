package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

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

//go:embed completion.fish
var fishScript string

func registerCompletions(cmd *cobra.Command, c completer) {
	cmd.ValidArgsFunction = c.completeArgs
	cmd.PersistentPreRunE = useOwnFishScript
	err := errors.Join(
		cmd.RegisterFlagCompletionFunc(conventionKey, completeConventions),
		cmd.MarkFlagFilename(configKey, "yaml", "yml"),
	)
	if err != nil {
		panic("casemv: register completions: " + err.Error())
	}
}

// useOwnFishScript makes `completion fish` print casemv's script instead of
// cobra's, which drops directory and extension filters. It swaps RunE because
// cobra adds its completion command only when called; a permanent one makes
// cobra add help, then offer it as a first-argument completion.
func useOwnFishScript(cmd *cobra.Command, _ []string) error {
	if isFishCompletionCommand(cmd) {
		cmd.RunE = writeFishScript
	}
	return nil
}

func isFishCompletionCommand(cmd *cobra.Command) bool {
	parent := cmd.Parent()
	return cmd.Name() == "fish" && parent != nil && parent.Name() == "completion" && parent.Parent() == cmd.Root()
}

func writeFishScript(cmd *cobra.Command, _ []string) error {
	noDescriptions, err := cmd.Flags().GetBool("no-descriptions")
	if err != nil {
		return fmt.Errorf("fish completion: %w", err)
	}
	script := fishScript
	if noDescriptions {
		script = strings.Replace(script, "__complete ", "__completeNoDesc ", 1)
	}
	if _, err := io.WriteString(cmd.OutOrStdout(), script); err != nil {
		return fmt.Errorf("write fish completion: %w", err)
	}
	return nil
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
// following symlinks, each with a trailing slash. Names holding control
// characters are left out: cobra's output separates candidates with newlines
// and descriptions with tabs.
func directoriesWithPrefix(prefix string) []string {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		name := entry.Name()
		isHidden := strings.HasPrefix(name, ".")
		if isHidden || !strings.HasPrefix(name, prefix) || strings.ContainsFunc(name, unicode.IsControl) {
			continue
		}
		if info, err := os.Stat(name); err == nil && info.IsDir() {
			dirs = append(dirs, name+"/")
		}
	}
	return dirs
}
