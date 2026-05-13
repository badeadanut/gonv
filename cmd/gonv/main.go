package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"gonv/internal/config"
	gonvdb "gonv/internal/db"
	"gonv/internal/node"
	"gonv/internal/pm"
)

// shimNames are the executable names materialized in ~/.gonv/shims.
// Each one is a copy of gonv-shim.exe; the shim dispatches by basename.
var shimNames = []string{"node", "npm", "npx", "pnpm", "pnpx", "yarn", "corepack"}

func main() {
	root := &cobra.Command{
		Use:           "gonv",
		Short:         "gonv — Node.js version manager for Windows",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(installCmd(), useCmd(), enableCmd(), listCmd(), currentCmd(), shimsCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <node_version>",
		Short: "Download and install a Node.js version",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			version := config.NormalizeVersion(args[0])
			dir, err := node.Install(version)
			if err != nil {
				return err
			}
			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := gonvdb.MarkNodeInstalled(db, version); err != nil {
				return err
			}
			if err := installShims(); err != nil {
				return fmt.Errorf("install shims: %w", err)
			}
			shimsDir, _ := config.ShimsPath()
			fmt.Printf("Installed Node %s at %s\n", version, dir)
			fmt.Printf("Shims written to %s — add that directory to your PATH.\n", shimsDir)
			return nil
		},
	}
}

func useCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <node_version>",
		Short: "Use this Node.js version for the current directory and its subdirectories",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			version := config.NormalizeVersion(args[0])
			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			installed, err := gonvdb.IsNodeInstalled(db, version)
			if err != nil {
				return err
			}
			if !installed {
				return fmt.Errorf("Node %s is not installed — run `gonv install %s` first", version, version)
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			abs, err := filepath.Abs(cwd)
			if err != nil {
				return err
			}
			if err := gonvdb.SetDirectoryVersion(db, abs, version); err != nil {
				return err
			}
			fmt.Printf("Using Node %s for %s (and its subdirectories).\n", version, abs)
			return nil
		},
	}
}

func enableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>[@version]",
		Short: "Enable a package manager (pnpm, yarn) for the active Node version",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name, version := splitPM(args[0])
			if name == "" {
				return fmt.Errorf("package manager name is required")
			}
			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			nodeVersion, err := node.ResolveForCWD(db, cwd)
			if err != nil {
				return err
			}
			if err := pm.Enable(nodeVersion, name, version); err != nil {
				return err
			}
			if err := gonvdb.EnablePM(db, nodeVersion, name, version); err != nil {
				return err
			}
			shown := version
			if shown == "" {
				shown = "latest"
			}
			fmt.Printf("Enabled %s@%s for Node %s.\n", name, shown, nodeVersion)
			return nil
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed Node versions",
		RunE: func(_ *cobra.Command, _ []string) error {
			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			versions, err := gonvdb.ListInstalledNode(db)
			if err != nil {
				return err
			}
			if len(versions) == 0 {
				fmt.Println("(no versions installed)")
				return nil
			}
			for _, v := range versions {
				fmt.Println(v)
			}
			return nil
		},
	}
}

func currentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Print the Node version configured for the current directory",
		RunE: func(_ *cobra.Command, _ []string) error {
			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			v, err := node.ResolveForCWD(db, cwd)
			if err != nil {
				return err
			}
			fmt.Println(v)
			return nil
		},
	}
}

func shimsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shims",
		Short: "(Re)install the shim binaries into ~/.gonv/shims",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := installShims(); err != nil {
				return err
			}
			shimsDir, _ := config.ShimsPath()
			fmt.Printf("Shims installed at %s\n", shimsDir)
			return nil
		},
	}
}

func splitPM(arg string) (string, string) {
	if i := strings.IndexByte(arg, '@'); i >= 0 {
		return arg[:i], arg[i+1:]
	}
	return arg, ""
}

// installShims copies gonv-shim.exe (next to gonv.exe) into the shims
// directory under each name in shimNames.
func installShims() error {
	shimsDir, err := config.ShimsPath()
	if err != nil {
		return err
	}
	if _, err := config.EnsureRoot(); err != nil {
		return err
	}
	selfExe, err := os.Executable()
	if err != nil {
		return err
	}
	src := filepath.Join(filepath.Dir(selfExe), "gonv-shim.exe")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("gonv-shim.exe not found next to gonv.exe (looked at %s)", src)
	}
	for _, n := range shimNames {
		dst := filepath.Join(shimsDir, n+".exe")
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", dst, err)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
