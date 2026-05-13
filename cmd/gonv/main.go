package main

import (
	"database/sql"
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
		Use:          "gonv",
		Short:        "gonv — Node.js version manager for Windows",
		SilenceUsage: true,
	}
	root.AddCommand(
		installCmd(),
		uninstallCmd(),
		useCmd(),
		enableCmd(),
		listCmd(),
		listRemoteCmd(),
		currentCmd(),
		shimsCmd(),
	)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func installCmd() *cobra.Command {
	var alias string
	cmd := &cobra.Command{
		Use:   "install <node_version> [--as <alias>]",
		Short: "Download and install a Node.js version, optionally under an alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			version := config.NormalizeVersion(args[0])
			name := alias
			if name == "" {
				name = version
			}
			if err := config.ValidateInstallName(name); err != nil {
				return err
			}

			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()

			exists, err := gonvdb.IsInstallRegistered(db, name)
			if err != nil {
				return err
			}
			if exists {
				return fmt.Errorf("install %q already exists — pick a different --as name or run `gonv uninstall %s` first", name, name)
			}

			target, err := config.InstallDir(name)
			if err != nil {
				return err
			}
			if err := node.Install(version, target); err != nil {
				return err
			}
			if err := gonvdb.RegisterInstall(db, name, version); err != nil {
				return err
			}
			if err := installShims(); err != nil {
				return fmt.Errorf("install shims: %w", err)
			}
			shimsDir, _ := config.ShimsPath()
			fmt.Printf("Installed Node %s as %q at %s\n", version, name, target)
			fmt.Printf("Shims at %s — make sure that directory is on PATH.\n", shimsDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&alias, "as", "", "alias name for this install (defaults to the version)")
	return cmd
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Remove an install (by version or alias) and its files",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()

			resolved, err := resolveInstallName(db, name)
			if err != nil {
				return err
			}
			target, err := config.InstallDir(resolved)
			if err != nil {
				return err
			}
			if err := gonvdb.DeleteInstall(db, resolved); err != nil {
				return err
			}
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("remove %s: %w", target, err)
			}
			fmt.Printf("Removed install %q and %s\n", resolved, target)
			return nil
		},
	}
}

func useCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Use this install for the current directory and its subdirectories",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			name, err := resolveInstallName(db, args[0])
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			abs, err := filepath.Abs(cwd)
			if err != nil {
				return err
			}
			if err := gonvdb.SetDirectoryInstall(db, abs, name); err != nil {
				return err
			}
			fmt.Printf("Using %q for %s (and subdirectories).\n", name, abs)
			return nil
		},
	}
}

func enableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>[@version]",
		Short: "Enable a package manager (pnpm, yarn) for the active install",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			pmName, pmVersion := splitPM(args[0])
			if pmName == "" {
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
			installName, err := node.ResolveForCWD(db, cwd)
			if err != nil {
				return err
			}
			if err := pm.Enable(installName, pmName, pmVersion); err != nil {
				return err
			}
			if err := gonvdb.EnablePM(db, installName, pmName, pmVersion); err != nil {
				return err
			}
			shown := pmVersion
			if shown == "" {
				shown = "latest"
			}
			fmt.Printf("Enabled %s@%s for install %q.\n", pmName, shown, installName)
			return nil
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installs",
		RunE: func(_ *cobra.Command, _ []string) error {
			db, err := gonvdb.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			installs, err := gonvdb.ListInstalls(db)
			if err != nil {
				return err
			}
			if len(installs) == 0 {
				fmt.Println("(no installs)")
				return nil
			}
			for _, i := range installs {
				if i.Name == i.NodeVersion {
					fmt.Println(i.Name)
				} else {
					fmt.Printf("%-20s  (node %s)\n", i.Name, i.NodeVersion)
				}
			}
			return nil
		},
	}
}

func currentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Print the install configured for the current directory",
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
			name, err := node.ResolveForCWD(db, cwd)
			if err != nil {
				return err
			}
			nv, _, err := gonvdb.GetInstall(db, name)
			if err != nil {
				return err
			}
			if name == nv {
				fmt.Println(name)
			} else {
				fmt.Printf("%s (node %s)\n", name, nv)
			}
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

func listRemoteCmd() *cobra.Command {
	var ltsOnly bool
	cmd := &cobra.Command{
		Use:     "list-remote",
		Aliases: []string{"ls-remote"},
		Short:   "List Node.js versions available for download from nodejs.org",
		RunE: func(_ *cobra.Command, _ []string) error {
			releases, err := node.FetchRemoteReleases()
			if err != nil {
				return err
			}
			for _, r := range releases {
				if ltsOnly && r.LTS == "" {
					continue
				}
				if r.LTS != "" {
					fmt.Printf("%-14s %s  (LTS: %s)\n", r.Version, r.Date, r.LTS)
				} else {
					fmt.Printf("%-14s %s\n", r.Version, r.Date)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&ltsOnly, "lts", false, "show only LTS releases")
	return cmd
}

// resolveInstallName accepts either an alias or a version, with or
// without a leading 'v', and returns the canonical install name as
// registered in the database.
func resolveInstallName(db *sql.DB, raw string) (string, error) {
	if exists, err := gonvdb.IsInstallRegistered(db, raw); err != nil {
		return "", err
	} else if exists {
		return raw, nil
	}
	normalized := config.NormalizeVersion(raw)
	if normalized != raw {
		if exists, err := gonvdb.IsInstallRegistered(db, normalized); err != nil {
			return "", err
		} else if exists {
			return normalized, nil
		}
	}
	return "", fmt.Errorf("no install named %q", raw)
}

func splitPM(arg string) (string, string) {
	if i := strings.IndexByte(arg, '@'); i >= 0 {
		return arg[:i], arg[i+1:]
	}
	return arg, ""
}

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
