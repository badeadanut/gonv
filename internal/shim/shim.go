package shim

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"gonv/internal/config"
	gonvdb "gonv/internal/db"
	"gonv/internal/node"
)

// Run is the entry point for every shim binary. The shim discovers which
// command was requested from its own filename (node.exe → "node"), looks
// up the install mapped to the current working directory, and execs the
// matching binary from that install's directory.
func Run() int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gonv-shim: cannot determine executable:", err)
		return 1
	}
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(exe), ".exe"))

	db, err := gonvdb.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gonv-shim: cannot open db:", err)
		return 1
	}
	defer db.Close()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gonv-shim: cannot get cwd:", err)
		return 1
	}
	installName, err := node.ResolveForCWD(db, cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gonv-shim:", err)
		return 1
	}

	nodeDir, err := config.InstallDir(installName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gonv-shim:", err)
		return 1
	}

	target, prefixArgs, err := resolveTarget(nodeDir, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gonv-shim: %v\n", err)
		return 1
	}

	args := append(prefixArgs, os.Args[1:]...)
	cmd := exec.Command(target, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
				return ws.ExitStatus()
			}
			return 1
		}
		fmt.Fprintln(os.Stderr, "gonv-shim: failed to run:", err)
		return 1
	}
	return 0
}

// resolveTarget locates the requested binary inside the install directory.
// .exe files run directly; .cmd / .bat must be run through cmd.exe because
// CreateProcess (used by Go's os/exec) cannot launch them on its own.
func resolveTarget(nodeDir, name string) (string, []string, error) {
	if p := filepath.Join(nodeDir, name+".exe"); fileExists(p) {
		return p, nil, nil
	}
	for _, ext := range []string{".cmd", ".bat"} {
		if p := filepath.Join(nodeDir, name+ext); fileExists(p) {
			comspec := os.Getenv("ComSpec")
			if comspec == "" {
				comspec = "cmd.exe"
			}
			return comspec, []string{"/c", p}, nil
		}
	}
	return "", nil, fmt.Errorf("binary %q not found in %s (is the package manager enabled? try `gonv enable %s`)", name, nodeDir, name)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
