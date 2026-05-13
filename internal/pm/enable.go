package pm

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"gonv/internal/config"
)

// Enable wires up a package manager for the given installed Node version
// using Corepack (which ships with Node ≥ 16.10).
//
//   - `corepack enable <name>` writes the shims into the Node bin dir.
//   - `corepack prepare <name>@<version> --activate` pins the version.
func Enable(nodeVersion, name, version string) error {
	nodeDir, err := config.NodeVersionDir(nodeVersion)
	if err != nil {
		return err
	}
	corepack := filepath.Join(nodeDir, "corepack.cmd")

	enable := exec.Command("cmd.exe", "/c", corepack, "enable", name)
	enable.Dir = nodeDir
	if out, err := enable.CombinedOutput(); err != nil {
		return fmt.Errorf("corepack enable %s: %w\n%s", name, err, string(out))
	}

	if version != "" {
		prepare := exec.Command("cmd.exe", "/c", corepack, "prepare",
			fmt.Sprintf("%s@%s", name, version), "--activate")
		prepare.Dir = nodeDir
		if out, err := prepare.CombinedOutput(); err != nil {
			return fmt.Errorf("corepack prepare %s@%s: %w\n%s", name, version, err, string(out))
		}
	}
	return nil
}
