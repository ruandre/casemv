package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// validateSettings fails on values viper would silently coerce: GetBool reads
// anything unparsable as false, and unknown config keys are ignored.
func validateSettings(v *viper.Viper, cmd *cobra.Command) error {
	for _, key := range v.AllKeys() {
		flag := cmd.Flags().Lookup(key)
		if flag == nil {
			return fmt.Errorf("unknown config key %q", key)
		}
		if flag.Value.Type() != "bool" {
			continue
		}
		if err := checkBool(key, v.Get(key)); err != nil {
			return err
		}
	}
	return nil
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
