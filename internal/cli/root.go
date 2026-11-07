// Package cli wires the casemv command line to the casing and rename packages.
package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ruandre/casemv/internal/casing"
)

const (
	configName = "casemv"
	configKey  = "config"
	excludeKey = "exclude"
	textKey    = "text"
)

// envPrefix starts every environment variable casemv reads.
var envPrefix = strings.ToUpper(configName)

// exitInterrupted is conventional shell code for SIGINT termination.
const exitInterrupted = 130

// argsWithTarget counts convention plus target directory.
const argsWithTarget = 2

// textIncompatibleFlags act on files, which --text never touches.
var textIncompatibleFlags = []string{"recursive", "dir", "hidden", excludeKey, "dry-run", "quiet", "lower-ext"}

// Execute runs the root command and returns the process exit code. An interrupt
// cancels the run between renames rather than part-way through one.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Restore the default handler once the first signal lands, so a second one
	// still kills a run stuck in a syscall, such as on a hung network mount.
	context.AfterFunc(ctx, stop)

	return report(os.Stderr, NewRootCmd().ExecuteContext(ctx))
}

// report writes err, returns process exit code.
func report(w io.Writer, err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) {
		//nolint:errcheck // the exit code carries the failure; a failed stderr write cannot
		fmt.Fprintln(w, "casemv: interrupted")
		return exitInterrupted
	}
	//nolint:errcheck // the exit code carries the failure; a failed stderr write cannot
	fmt.Fprintln(w, "casemv:", errorText(err))
	return 1
}

// NewRootCmd builds the root command with its own viper instance, so tests and
// embedders can run it without sharing global configuration state. A --text
// run cancelled while waiting for input returns at once, but leaves that read
// pending on its own goroutine, where it consumes the next chunk of input.
func NewRootCmd() *cobra.Command {
	v := viper.New()
	var configFile string

	cmd := &cobra.Command{
		Use:   "casemv [convention] [target]",
		Short: "Rename files and directories to a naming convention",
		Long: "casemv renames the files in a directory to a naming convention such as\n" +
			"kebab-case or snake_case. File extensions are preserved unless --lower-ext is\n" +
			"set; directory names are converted whole, so \"1. Something\" becomes\n" +
			"\"1-something\" rather than being split at the dot.\n\n" +
			"The convention may be omitted when --convention, CASEMV_CONVENTION, or the\n" +
			"config file sets a default; a lone argument naming a convention is the\n" +
			"convention, anything else is the target.\n\nConventions: " + casing.Names(),
		Example: "  casemv kebab\n" +
			"  casemv snake ./docs --recursive --dir\n" +
			"  casemv pascal ./src --dry-run --exclude '*.min.js'\n" +
			"  casemv ./docs -c kebab -x\n" +
			"  echo 'My File Name' | casemv kebab --text",
		Args:          cobra.MaximumNArgs(argsWithTarget),
		Version:       buildVersion(),
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveSettings(v, cmd, args)
			if err != nil {
				return err
			}
			return run(cmd.Context(), cmd.OutOrStdout(), cmd.InOrStdin(), s)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&configFile, configKey, "",
		"config file (default: $CASEMV_CONFIG, else $XDG_CONFIG_HOME/casemv/casemv.yaml)")
	flags.StringP(conventionKey, "c", "", "convention to use when no convention argument is given")
	flags.BoolP("recursive", "r", false, "rename entries in subdirectories as well")
	flags.BoolP("dir", "D", false, "rename directories as well as files")
	flags.BoolP("hidden", "H", false, "include dot-prefixed entries (skipped by default)")
	flags.StringArrayP(excludeKey, "e", nil,
		"skip glob matches; repeatable; a leading or inner slash matches the path, a trailing one only directories")
	flags.BoolP("dry-run", "n", false, "print the planned renames without touching the filesystem")
	flags.BoolP("quiet", "q", false, "suppress progress and summary output")
	flags.BoolP("lower-ext", "x", false, "lowercase file extensions as well")
	flags.Bool(textKey, false, "convert lines from stdin to stdout instead of renaming files")

	registerCompletions(cmd, completer{v: v, configFile: &configFile})

	// Not PersistentPreRunE: subcommands (completion, __complete) have none of
	// these flags, so loading config for them rejects every key.
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return loadConfig(v, cmd, configFile)
	}
	return cmd
}

func loadConfig(v *viper.Viper, cmd *cobra.Command, configFile string) error {
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("bind flags: %w", err)
	}

	configFile = cmp.Or(configFile, os.Getenv(envPrefix+"_CONFIG"))

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		// Deliberately not the working directory: a casemv.yaml in a
		// directory you did not author could silently turn a top-level rename
		// into a recursive, hidden-inclusive one. Project-local config must be
		// opted into with --config.
		v.SetConfigName(configName)
		if dir, err := os.UserConfigDir(); err == nil {
			v.AddConfigPath(filepath.Join(dir, configName))
		}
	}

	if err := v.ReadInConfig(); err != nil && !isMissingDefaultConfig(err, configFile) {
		return fmt.Errorf("read config: %w", err)
	}
	if err := validateSettings(v, cmd); err != nil {
		return err
	}
	return validateDefaultConvention(v)
}

// isMissingDefaultConfig reports absent optional config; a file named by
// --config or CASEMV_CONFIG must exist.
func isMissingDefaultConfig(err error, configFile string) bool {
	var notFound viper.ConfigFileNotFoundError
	return configFile == "" && (errors.As(err, &notFound) || errors.Is(err, os.ErrNotExist))
}

func resolveSettings(v *viper.Viper, cmd *cobra.Command, args []string) (settings, error) {
	parsed, err := parsePositional(args, v.GetString(conventionKey))
	if err != nil {
		return settings{}, err
	}
	text := isTextMode(cmd)
	if text {
		if err := validateTextMode(cmd, parsed.target); err != nil {
			return settings{}, err
		}
	}
	target := cmp.Or(parsed.target, ".")
	return settings{
		convention:     parsed.convention,
		target:         target,
		recursive:      v.GetBool("recursive"),
		dirs:           v.GetBool("dir"),
		hidden:         v.GetBool("hidden"),
		exclude:        excludePatterns(v),
		dryRun:         v.GetBool("dry-run"),
		quiet:          v.GetBool("quiet"),
		lowerExtension: v.GetBool("lower-ext"),
		text:           text,
	}, nil
}

// excludePatterns splits a string value, as CASEMV_EXCLUDE gives, like PATH:
// patterns hold spaces and commas far more often than colons.
func excludePatterns(v *viper.Viper) []string {
	if list, ok := v.Get(excludeKey).(string); ok {
		return filepath.SplitList(list)
	}
	return v.GetStringSlice(excludeKey)
}

// isTextMode reads --text from the command line alone: validateSettings
// rejects it anywhere else, and completion must see it even when the config
// fails to load.
func isTextMode(cmd *cobra.Command) bool {
	text, err := cmd.Flags().GetBool(textKey)
	return err == nil && text
}

// validateTextMode rejects only flags given on the command line: a config file
// or environment default meant for renaming must not break --text.
func validateTextMode(cmd *cobra.Command, target string) error {
	if target != "" {
		return errors.New("--text does not accept a target argument")
	}
	for _, name := range textIncompatibleFlags {
		// --dry-run=false asks for nothing --text would ignore.
		if flag := cmd.Flags().Lookup(name); flag.Changed && flag.Value.String() != "false" {
			return fmt.Errorf("--text cannot be used with --%s", name)
		}
	}
	return nil
}
