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
var dataFile string

var rootCmd = &cobra.Command{
	Use:   "todo",
	Short: "todo application to help me work on my goals.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	home, _ := homedir.Dir()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".todo")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("todo")

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

	// Priority: 1. Explicit Flag, 2. Config File, 3. Default
	if dataFile == "" || dataFile == defaultDataPath {
		dataFile = viper.GetString("datafile")
	}

	ensureDataFileExists()
}

func init() {
	cobra.OnInitialize(initConfig)

	home, _ := homedir.Dir()
	defaultDataPath := filepath.Join(home, ".tasks.json")

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.todo.yaml)")
	rootCmd.PersistentFlags().StringVar(&dataFile, "datafile", defaultDataPath, "data file to store tasks.")
}

func createDefaultConfig(home string) {
	configPath := filepath.Join(home, ".todo.yaml")
	// Only create if it truly doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		viper.Set("datafile", filepath.Join(home, ".tasks.json"))
		if err := viper.SafeWriteConfigAs(configPath); err != nil {
			log.Printf("Could not create default config: %v", err)
		} else {
			fmt.Printf("Created default config file: %s\n", configPath)
		}
	}
}

func ensureDataFileExists() {
	dir := filepath.Dir(dataFile)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		if err := os.WriteFile(dataFile, []byte("[]"), 0644); err != nil {
			log.Fatalf("Failed to create data file %s: %v", dataFile, err)
		}
		fmt.Printf("Created data file: %s\n", dataFile)
	}
}
