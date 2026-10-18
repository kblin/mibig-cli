// Command mibig works with MIBiG entries.
package main

import (
	"os"

	"go.kblin.dev/mibig-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
