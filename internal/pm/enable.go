package pm

import (
	"fmt"
	"strings"

	"gonv/internal/config"
)

// Enable downloads a package manager's binaries and installs them into the
// directory of the given Node version. The gonv shims pick them up
// automatically because they live alongside node.exe.
func Enable(nodeVersion, name, version string) error {
	nodeDir, err := config.NodeVersionDir(nodeVersion)
	if err != nil {
		return err
	}
	switch strings.ToLower(name) {
	case "pnpm":
		return installPnpm(nodeDir, version)
	case "yarn":
		return installYarn(nodeDir, version)
	default:
		return fmt.Errorf("unsupported package manager %q (supported: pnpm, yarn)", name)
	}
}
