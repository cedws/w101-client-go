package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go-dml-codegen/codegen"
	"os"
)

var (
	output  string
	debug   bool
	rootCmd = &cobra.Command{
		Short: "Generate Go code from a DML service definition",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var f *os.File
			var err error

			w := os.Stdout

			if output != "" {
				f, err = os.Create(output)
				if err != nil {
					log.Fatal(err)
				}
				defer f.Close()
				w = f
			}

			err = codegen.Generate(w, args[0])
			if err != nil {
				log.Fatal(err)
			}
		},
	}
)

func init() {
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "file to write output to")
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
