package cli

import "runtime/debug"

// version is overridden at release time with -ldflags "-X ...cli.version=vX.Y.Z";
// otherwise it comes from the build info stamped by the module system.
var version = ""

func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}
