package cmd

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "explain-cloudformation-changeset",
	Short:   "Explain a CloudFormation changeset",
	Long:    `explain-cloudformation-changeset provides tools to make reviewing a CloudFormation changeset easier`,
	Aliases: []string{graphCmd.Name()},
}

var cacheDir string
var region string
var stackName string
var changeSetName string

func checkRootAlias(a string, b []string) {
	for _, v := range b {
		if a == v {
			return
		}
	}
	os.Args = append([]string{os.Args[0], rootCmd.Aliases[0]}, os.Args[1:]...)
}

func nonRootSubCmds() (l []string) {
	for _, c := range rootCmd.Commands() {
		isAlias := false
		for _, a := range append(c.Aliases, c.Name()) {
			if a == rootCmd.Aliases[0] {
				isAlias = true
				break
			}
		}
		if !isAlias {
			l = append(l, c.Name())
			l = append(l, c.Aliases...)
		}
	}
	return
}

func Execute() {
	// Work out a default command if none is given
	// See https://github.com/spf13/cobra/issues/725#issuecomment-411807394
	var firstArg string
	if len(os.Args) > 1 {
		firstArg = os.Args[1]
	}
	checkRootAlias(firstArg, nonRootSubCmds())
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func getEnvOrDefault(defaultValue string, names ...string) string {
	for _, name := range names {
		val, present := os.LookupEnv(name)
		if present {
			return val
		}
	}

	return defaultValue
}

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("cannot determine current working directory (%q)", err.Error())
	}

	rootCmd.PersistentFlags().StringVar(&cacheDir, "cache-dir", cwd, "Directory for caching changeset descriptions")
	rootCmd.PersistentFlags().StringVar(&region, "region", getEnvOrDefault("us-east-1", "AWS_REGION", "AWS_DEFAULT_REGION"), "AWS region")
	rootCmd.PersistentFlags().StringVar(&stackName, "stack-name", "", "Root stack name (required when change set is not given as ARN)")
	rootCmd.PersistentFlags().StringVar(&changeSetName, "change-set-name", "", "Root change set name")
	rootCmd.MarkPersistentFlagRequired("change-set-name")
}
