package configs

import "embed"

// DefaultConfigs embeds the default configuration files into the binary.
// This allows running without external config files.
//
//go:embed master.yaml agent.yaml
var DefaultConfigs embed.FS
