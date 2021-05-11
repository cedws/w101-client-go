package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go-dml-codegen/codegen"
)

var (
	debug   bool
	rootCmd = &cobra.Command{
		Short: "Generate Go code from a DML service definition",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			err := codegen.Generate(args[0])
			if err != nil {
				log.Fatal(err)
			}
		},
	}
)

func init() {
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug logging")
}

func Execute() {
	if debug {
		log.SetLevel(log.DebugLevel)
	}

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
