package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"

	"github.com/ruandre/casemv/internal/casing"
)

const conventionKey = "convention"

var errMissingConvention = errors.New(
	"missing naming convention: pass one as the first argument, or set --convention, " +
		"CASEMV_CONVENTION, or convention in the config file")

// positional is the meaning of the command-line arguments. target is empty when none was given.
type positional struct {
	convention casing.Convention
	target     string
}

// parsePositional reads [convention] [target]. A lone argument naming a
// convention is the convention; otherwise it is the target and fallback, the
// configured default, supplies the convention.
func parsePositional(args []string, fallback string) (positional, error) {
	switch len(args) {
	case 0:
		return withDefault(fallback, "")
	case 1:
		convention, err := casing.ParseConvention(args[0])
		if err == nil || fallback == "" {
			return positional{convention: convention}, err
		}
		return withDefault(fallback, args[0])
	default:
		convention, err := casing.ParseConvention(args[0])
		return positional{convention: convention, target: args[1]}, err
	}
}

func withDefault(fallback, target string) (positional, error) {
	if fallback == "" {
		return positional{}, errMissingConvention
	}
	convention, err := casing.ParseConvention(fallback)
	return positional{convention: convention, target: target}, err
}

// validateDefaultConvention rejects a bad default even when an argument would
// override it, so a broken config fails on first use rather than later.
func validateDefaultConvention(v *viper.Viper) error {
	name := v.GetString(conventionKey)
	if name == "" {
		return nil
	}
	if _, err := casing.ParseConvention(name); err != nil {
		return fmt.Errorf("default convention: %w", err)
	}
	return nil
}

func conventionNames() []string {
	conventions := casing.Conventions()
	names := make([]string, len(conventions))
	for i, c := range conventions {
		names[i] = string(c)
	}
	return names
}
