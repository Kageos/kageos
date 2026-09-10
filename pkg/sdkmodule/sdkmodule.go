package sdkmodule

import "strings"

const (
	ModulePath = "github.com/kageos/kageos-sdk"
	// Version is a legacy fallback for cache lookups. App builds sync the SDK
	// through LatestVersionQuery unless a local replace is present.
	Version             = "v0.3.4"
	LatestVersionQuery  = "latest"
	LocalReplaceVersion = "v0.0.0"
	AgentAppPrefix      = ModulePath + "/agent-app"
)

func AgentAppImport(packagePath string) string {
	packagePath = strings.Trim(packagePath, "/")
	if packagePath == "" {
		return AgentAppPrefix
	}
	return AgentAppPrefix + "/" + packagePath
}
