package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// validateSettings fails on values viper would silently coerce: GetBool reads
// anything unparsable as false, GetString and GetStringSlice read a mistyped
// value as empty, and unknown config keys are ignored. Keys are checked in
// order so the same config always reports the same error.
func validateSettings(v *viper.Viper, cmd *cobra.Command) error {
	keys := v.AllKeys()
	slices.Sort(keys)
	for _, key := range keys {
		flag := cmd.Flags().Lookup(key)
		if flag == nil {
			return fmt.Errorf("unknown config key %q", key)
		}
		if err := checkSource(v, key); err != nil {
			return err
		}
		if err := checkType(key, flag.Value.Type(), v.Get(key)); err != nil {
			return err
		}
	}
	return nil
}

// checkSource rejects keys only the command line may set. A stored text
// default would turn every run into a stdin filter. Like viper, it ignores an
// empty environment variable.
func checkSource(v *viper.Viper, key string) error {
	switch {
	case key == configKey && v.InConfig(key):
		return errors.New(`config key "config" is not allowed: set --config or CASEMV_CONFIG instead`)
	case key == textKey && (v.InConfig(key) || os.Getenv(envPrefix+"_TEXT") != ""):
		return errors.New(`"text" cannot be set in the config file or CASEMV_TEXT: pass --text`)
	}
	return nil
}

func checkType(key, flagType string, value any) error {
	switch flagType {
	case "bool":
		return checkBool(key, value)
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("invalid value %v for %s: want a string", value, key)
		}
		return nil
	case "stringArray":
		return checkStringList(key, value)
	default:
		return fmt.Errorf("no validation for %s, a %s flag", key, flagType)
	}
}

func checkBool(key string, value any) error {
	if _, ok := value.(bool); ok {
		return nil
	}
	text := fmt.Sprint(value)
	if _, err := strconv.ParseBool(text); err != nil {
		return fmt.Errorf("invalid boolean %q for %s: want true or false", text, key)
	}
	return nil
}

// checkStringList accepts a string too: an environment variable is one, and
// excludePatterns splits it.
func checkStringList(key string, value any) error {
	switch list := value.(type) {
	case string, []string:
		return nil
	case []any:
		isString := func(item any) bool { _, ok := item.(string); return ok }
		if !slices.ContainsFunc(list, func(item any) bool { return !isString(item) }) {
			return nil
		}
	}
	return fmt.Errorf("invalid value %v for %s: want a list of strings", value, key)
}
