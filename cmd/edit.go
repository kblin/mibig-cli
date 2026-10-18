package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	mibig "go.kblin.dev/go-mibig"

	"go.kblin.dev/mibig-cli/internal/editor"
)

var editCmd = &cobra.Command{
	Use:   "edit FILE",
	Short: "Edit an entry interactively",
	Long: `edit opens a MIBiG entry in an interactive editor.

The entry is shown as a tree. Select a value to see its details and
validation issues, press enter to edit it, a to add list elements or
optional sections, d to delete, and e to edit any part as JSON in your
editor. The entry is validated after every change; press n to jump to the
next issue and v to check against a different quality level. Press ? for
all keys.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entry, err := mibig.ReadFile(args[0])
		if err != nil {
			return err
		}
		quality := mibig.Quality(viper.GetString("edit.validate-at"))
		if quality != "" && !quality.Valid() {
			return fmt.Errorf("invalid quality level %q for --validate-at, expected questionable, low, medium or high", quality)
		}
		return editor.Run(entry, editor.Options{
			Path:       args[0],
			Editor:     viper.GetString("editor"),
			ValidateAt: quality,
		})
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
	editCmd.Flags().String("editor", "", "command to edit JSON with (default $VISUAL, $EDITOR or vi)")
	editCmd.Flags().String("validate-at", "", "quality level to validate at (default: the entry's own)")
	_ = viper.BindPFlag("editor", editCmd.Flags().Lookup("editor"))
	_ = viper.BindPFlag("edit.validate-at", editCmd.Flags().Lookup("validate-at"))
}
