package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// AppConfig stores entire app config
type AppConfig struct {
	Retries int      `mapstructure:"retries"`
	Sleep   string   `mapstructure:"sleep"`
	Regex   []string `mapstructure:"regex"`
	Min     string   `mapstructure:"min"`
	Max     string   `mapstructure:"max"`
}

// NewCommand return xlr8s sub commands
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retry",
		Short: "Retries failed commands",
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.AutomaticEnv()
			// When using environment variables, replace . with _ and - with _
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

			f := cmd.Flags()
			// When using configuration files, replace - with _
			normalizeFunc := f.GetNormalizeFunc()
			f.SetNormalizeFunc(func(fs *pflag.FlagSet, name string) pflag.NormalizedName {
				result := normalizeFunc(fs, name)
				name = strings.ReplaceAll(string(result), "-", "_")
				return pflag.NormalizedName(name)
			})
			_ = viper.BindPFlags(f)
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			viper.SetConfigType("yaml")
			configFilePath := viper.GetString("config")
			viper.SetConfigFile(configFilePath)

			if configFilePath != "" {
				if err := viper.ReadInConfig(); err != nil {
					return fmt.Errorf("error reading config file, %v", err)
				}
			}

			var conf AppConfig
			if err := viper.Unmarshal(&conf); err != nil {
				return fmt.Errorf("unable to decode into struct, %v", err)
			}

			if len(args) == 0 {
				fmt.Println("Please provide a command to execute.")
				os.Exit(1)
			}

			command := ""
			finalArgs := []string{}
			if len(args) == 1 {
				command = args[0]
				checkSpaceExists := strings.Split(command, " ")
				if len(checkSpaceExists) > 1 {
					command = checkSpaceExists[0]
					finalArgs = checkSpaceExists[1:]
				}
			}
			if len(args) > 1 {
				command = args[0]
				finalArgs = args[1:]
			}

			fmt.Println("Executing command in retriable CLI...")
			return executeCommand(command, finalArgs, conf)
		},
	}

	// Server Config
	cmd.Flags().IntP("retries", "r", 9, "")
	cmd.Flags().StringP("sleep", "s", "5s", "")
	cmd.Flags().StringSliceP("regex", "", []string{}, "")
	cmd.Flags().StringP("min", "", "10s", "")
	cmd.Flags().StringP("max", "", "10s", "")
	cmd.Flags().StringP("config", "", "", "Path to config file")

	return cmd
}

func main() {
	if err := NewCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func executeCommand(command string, args []string, configOption AppConfig) error {
	sleepDuration, err2 := time.ParseDuration(configOption.Sleep)
	if err2 != nil {
		return err2
	}

	var err error
	for i := 0; i < configOption.Retries; i++ {
		err = runCommand(command, configOption.Regex, args...)
		if err == nil {
			break
		}

		fmt.Printf("Encountered an error eligible for retrying. Attempt %d/%d failed. Sleeping %s before retrying.\n", i+1, configOption.Retries, configOption.Sleep)
		time.Sleep(sleepDuration)
	}

	if err != nil {
		fmt.Println("Command execution failed after retries.")
		os.Exit(1)
	}

	return nil
}

func runCommand(name string, matchPatterns []string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ExitCode() == 1 {
			matched := false
			for _, matchPattern := range matchPatterns {
				matched, _ = regexp.MatchString(matchPattern, stdout.String())
				if matched {
					fmt.Println(stdout.String()) // print the output for debugging
					return err
				}
			}
		}
	}

	// Print output if not matched
	fmt.Println(stdout.String())
	return nil
}
