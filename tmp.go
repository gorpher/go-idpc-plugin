package plugin

import (
	"os"
	"strings"
)

// 插件环境变量名称
var PLUGIN_ENV_VAR = strings.ReplaceAll(strings.ToUpper(PLUGIN_PREFIX), "-", "_") + "_WORKDIR"

// PluginWorkDir returns path for mackerel-agent plugins' cache / tempfiles
func PluginWorkDir() string {
	dir := os.Getenv(PLUGIN_ENV_VAR)
	if dir == "" {
		dir = os.TempDir()
	}
	return dir
}
