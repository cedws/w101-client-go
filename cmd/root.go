package cmd

import (
	"os"

	"github.com/cedws/w101-client-go/codegen"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	output string
	debug  bool
)

var rootCmd = &cobra.Command{
	Short: "Generate Go code from a DML service definition",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if debug {
			log.SetLevel(log.DebugLevel)
		}

		var file *os.File
		var err error

		w := os.Stdout

		if output != "" {
			file, err = os.Create(output)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			w = file
		}

		protocol, err := codegen.ReadProtocol(args[0])
		if err != nil {
			log.Fatal(err)
		}

		packageName, ok := os.LookupEnv("GOPACKAGE")
		if !ok {
			log.Fatal("GOPACKAGE not set")
		}

		if err := codegen.Generate(w, packageName, protocol); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "file to write output to")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug logging")
}

func Execute() {
	log.SetFormatter(&log.TextFormatter{
		DisableQuote: true,
	})

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
