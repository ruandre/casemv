// Command casemv renames files and directories to a naming convention.
package main

import (
	"os"

	"github.com/ruandre/casemv/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
