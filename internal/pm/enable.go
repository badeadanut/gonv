package pm

import (
	"fmt"
	"strings"

	"gonv/internal/config"
)

// Enable downloads a package manager's binaries and installs them into
// the install directory identified by installName.
func Enable(installName, name, version string) error {
	nodeDir, err := config.InstallDir(installName)
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
