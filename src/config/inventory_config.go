package config

import (
	"flag"

	log "github.com/sirupsen/logrus"
)

// Inventory command configuration
type InventoryConfig struct {
	DryRunConfig
	LoggingConfig
	GPUConfigFile string
	OutputMaxSize int
	DiskMinSize   int64
}

func ProcessInventoryConfigArgs() *InventoryConfig {
	ret := &InventoryConfig{}

	RegisterLoggingArgs(&ret.LoggingConfig)

	err := RegisterDryRunArgs(&ret.DryRunConfig)
	if err != nil {
		log.Fatalf("Failed to register dry run arguments: %v", err)
	}

	flag.StringVar(&ret.GPUConfigFile, "gpu-config-file", "", "Configuration file for GPU discovery")
	flag.IntVar(&ret.OutputMaxSize, "output-max-size", 0, "Best-effort maximum size of the inventory JSON output in bytes (0 disables the limit). Truncation metadata may cause the final payload to exceed this value.")
	flag.Int64Var(&ret.DiskMinSize, "disk-min-size", 0, "Minimum size of a disk in bytes to be included in the inventory. Disks smaller than this value will be excluded. (0 disables disk size filtering)")
	h := flag.Bool("help", false, "Help message")
	flag.Parse()

	if h != nil && *h {
		printHelpAndExit()
	}

	if ret.OutputMaxSize < 0 {
		log.Fatalf("Invalid --output-max-size: must be >= 0, got %d", ret.OutputMaxSize)
	}

	if ret.DiskMinSize < 0 {
		log.Fatalf("Invalid --disk-min-size: must be >= 0, got %d", ret.DiskMinSize)
	}

	return ret
}
