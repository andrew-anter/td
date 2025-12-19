/*
Copyright © 2025 Andrew Anter <andrew.anter@gmail.com>
*/
package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var dataFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "todo",
	Short: "todo application to help me work on my goals.",
	Long:  ``,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	viper.SetConfigName(".todo")
	viper.AddConfigPath("$HOME")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("todo")

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using Config file:", viper.ConfigFileUsed())
	}
}

// func initConfig() {
// 	home, err := os.UserHomeDir()
// 	if err != nil {
// 		fmt.Println(err)
// 		os.Exit(1)
// 	}
//
// 	viper.AddConfigPath(home)
// 	viper.SetConfigName(".todo")
// 	viper.SetConfigType("yaml")
//
// 	viper.AutomaticEnv()
// 	viper.SetEnvPrefix("todo")
//
// 	if err := viper.ReadInConfig(); err == nil {
// 		fmt.Println("Using Config file:", viper.ConfigFileUsed())
// 	} else {
// 		fmt.Printf("Error reading config file: %s\n", err)
// 	}
// }

func init() {
	cobra.OnInitialize(initConfig)
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	home, err := homedir.Dir()
	if err != nil {
		log.Println("Unable to detect home directory. Please set data file using --datafile.")
		return
	}

	rootCmd.PersistentFlags().StringVar(&dataFile, "datafile", home+string(os.PathSeparator)+".tasks.json", "data file to store tasks.")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tasks.yaml).")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
