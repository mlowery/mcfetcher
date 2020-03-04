package root

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mlowery/mcfetcher/cmd/fetch"
)

var cfgFile string

var cmd = &cobra.Command{
	Use:   "mcfetcher",
	Short: "Fetch, filter, and sanitize objects across Kubernetes clusters.",
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
}

func Execute() {
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	cmd.SilenceUsage = true
	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
	cmd.PersistentFlags().StringP("work-dir", "d", ".", "working directory")
	viper.BindPFlag("work-dir", cmd.PersistentFlags().Lookup("work-dir"))
	cmd.AddCommand(fetch.Cmd)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.SetEnvPrefix("mcfetcher")
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			log.Fatalf("failed to read config file: %v", err)
		}
	}
}
