// This module is responsible for loading the config file
package config

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/re-cinq/aether/pkg/log"
	"github.com/spf13/viper"
)

var (
	// Stores the ApplicationConfig
	config *ApplicationConfig

	// Locks in case the file changed and it's been reloaded
	lock sync.Mutex

	// The last time the config was updated/reloaded
	UpdatedAt time.Time
)

func InitConfig(ctx context.Context) {
	logger := log.FromContext(ctx)

	viper.SetEnvPrefix("CARBON")        // sets an environment variable prefix CARBON_
	viper.SetConfigName(getEnvConfig()) // name of config file (without extension)
	viper.SetConfigType("yaml")         // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath(".")            // optionally look for config in the working directory
	viper.AddConfigPath("conf")         // path to look for the config file in the "./conf" path

	// Set defaults
	viper.SetDefault("api.metricsPath", "/metrics")
	viper.SetDefault("logLevel", "info")
	viper.SetDefault("cache.store", "bigcache")
	viper.SetDefault("cache.expire", "3600m")

	// Find and read the config file
	err := viper.ReadInConfig()

	// Handle errors reading the config file
	if err != nil {
		logger.Error("could not parse/read config file", "error", err)
		os.Exit(255)
	}

	// parse the application config
	config = parseApplicationConfig(ctx)

	// Setup a call back on the file changes
	viper.OnConfigChange(func(e fsnotify.Event) {
		// Log the fact that the config file was changed
		logger.Info("config file changed", "file", fmt.Sprintf("%s.yaml", getEnvConfig()), "event", e.Name)

		// Lock the reading of the file untile we parsed it
		lock.Lock()

		// Parse the config file
		config = parseApplicationConfig(ctx)

		// Unlock so that the file can be accessed again
		lock.Unlock()

		// Log the fact that the new config file was reloaded
		logger.Info("config file reloaded", "file", fmt.Sprintf("%s.yaml", getEnvConfig()))
	})

	// Setup a watch in case the config file is changed
	viper.WatchConfig()
}

// AppConfig returns the app config
func AppConfig() *ApplicationConfig {
	// Make sure we lock, because there could be a write happening
	lock.Lock()

	// Defer the unlocking
	defer lock.Unlock()

	// return the config file
	return config
}

// ParseApplicationConfig reads the config file into a struct
func parseApplicationConfig(ctx context.Context) *ApplicationConfig {
	logger := log.FromContext(ctx)

	var config ApplicationConfig

	// Override the config file via the environment variables
	// for _, k := range viper.AllKeys() {
	// 	v := viper.GetString(k)
	// 	viper.Set(k, os.ExpandEnv(v))
	// }

	// Parse the config file
	if err := viper.Unmarshal(&config); err != nil {
		logger.Error("Error parsing config file", "error", err)
		os.Exit(1)
	}

	// Update the last modified time
	UpdatedAt = time.Now()

	// return the config
	return &config
}

// Defines the name of the config file
func getEnvConfig() string {
	// First check if the name is defined in an environment variable
	environment := os.Getenv(DefaultEnvironmentConfigName)

	// If it's not defined then load the default config name
	if environment == "" {
		// fall back to the default config file name which is `local`
		environment = DefaultEnvironment
	}

	return environment
}
