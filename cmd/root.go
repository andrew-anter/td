/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:           "todo",
	Short:         "todo application to help me work on my goals.",
	Long:          "With no arguments, prints pending todos (what you should be working on).",
	RunE:          rootRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func rootRun(cmd *cobra.Command, args []string) error {
	return runList(false, false)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initConfig() {
	home, err := homedir.Dir()
	if err != nil {
		log.Fatalf("failed to determine home directory: %v", err)
	}

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".todo")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("todo")

	// Bind --datafile so viper respects the CLI flag value above config/default.
	if err := viper.BindPFlag("datafile", rootCmd.PersistentFlags().Lookup("datafile")); err != nil {
		log.Printf("could not bind datafile flag: %v", err)
	}

	// Set a default in Viper so it can be written to the config file later
	defaultDataPath := filepath.Join(home, ".tasks.json")
	viper.SetDefault("datafile", defaultDataPath)

	if err := viper.ReadInConfig(); err != nil {
		// If config file not found, create a default one
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			createDefaultConfig(home)
		} else {
			log.Printf("Error reading config: %v", err)
		}
	}

	ensureDataFileExists()
}

func init() {
	cobra.OnInitialize(initConfig)

	home, err := homedir.Dir()
	if err != nil {
		log.Fatalf("failed to determine home directory: %v", err)
	}
	defaultDataPath := filepath.Join(home, ".tasks.json")

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.todo.yaml)")
	rootCmd.PersistentFlags().String("datafile", defaultDataPath, "data file to store tasks.")
}

func createDefaultConfig(home string) {
	configPath := filepath.Join(home, ".todo.yaml")
	// Only create if it truly doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := viper.SafeWriteConfigAs(configPath); err != nil {
			log.Printf("Could not create default config: %v", err)
		} else {
			fmt.Printf("Created default config file: %s\n", configPath)
		}
	}
}

func ensureDataFileExists() {
	path := viper.GetString("datafile")
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("[]"), 0644); err != nil {
			log.Fatalf("Failed to create data file %s: %v", path, err)
		}
		fmt.Printf("Created data file: %s\n", path)
	}
}
