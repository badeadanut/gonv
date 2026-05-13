# gonv

A Node.js version manager for Windows, written in Go.

`gonv` lets you install multiple Node.js versions side by side, pin a
version to a directory tree, and enable `pnpm` / `yarn` per install — all
backed by a single SQLite database in `~/.gonv/`.

Unlike `nvm-windows`, switching versions doesn't shell out to a wrapper
script: each Node-related command (`node`, `npm`, `npx`, `pnpm`, `pnpx`,
`yarn`, `corepack`) is a tiny shim that looks up the configured version
for your current working directory and execs the real binary.

## Highlights

- **Per-directory pinning**: `gonv use 20.10.0` writes a mapping for the
  current directory; every subdirectory inherits it via longest-prefix
  match. No `.nvmrc` to manage and no PATH swapping per shell.
- **Aliases**: `gonv install 20.10.0 --as work` creates a second
  independent install of the same Node version with its own package
  managers. Useful when one project needs `pnpm@8` and another needs
  `pnpm@9`.
- **Partial version queries**: `gonv install v22` resolves to the latest
  `v22.x.x` release. `gonv enable pnpm@8` pulls the latest `8.x.x`. Same
  for yarn.
- **Direct binary downloads**: pnpm comes from GitHub releases
  (`pnpm-win32-x64.zip`), yarn from the npm registry tarball. No
  `corepack` round-trip.
- **Pure-Go SQLite**: no CGO toolchain required to build.
- **Single shim binary**: `gonv-shim.exe` is copied under each command
  name and dispatches based on its own filename.

## Installation

### Requirements

- Windows (x64)
- Go 1.22 or newer (only to build — there are no runtime dependencies)

### Build from source

```powershell
git clone git@github.com:badeadanut/gonv.git
cd gonv
.\build.ps1
```

This produces `.\bin\gonv.exe` and `.\bin\gonv-shim.exe`. Keep both in
the same directory; the shim binary needs to sit next to `gonv.exe` so
that `gonv install` can copy it under each command name.

### Put the shims on PATH

After the first `gonv install`, shims live at:

```
%USERPROFILE%\.gonv\shims
```

Add that directory to your **User** `PATH` (System Properties →
Environment Variables → User variables → `Path` → Edit → New).

If you currently have other Node version managers or installations on
`PATH` — `nvm-windows`, `C:\Program Files\nodejs`, etc. — remove them
first. They'll otherwise shadow the gonv shims because Windows searches
`PATH` left-to-right, first match wins.

Open a new terminal and confirm:

```powershell
where.exe node
# C:\Users\<you>\.gonv\shims\node.exe
```

## Quick start

```powershell
# See what's available upstream
gonv list-remote --lts

# Install Node — full version
gonv install 20.10.0

# Or a partial version: latest v22.x.x
gonv install v22

# Pin a version to the current project tree
cd C:\code\myproject
gonv use 20.10.0

# Inspect what's active here
gonv current
# v20.10.0

# Enable package managers for this install (drops binaries next to
# node.exe in the install directory)
gonv enable pnpm@8         # latest pnpm 8.x.x
gonv enable yarn           # latest yarn 1.x

# Verify it all works
node --version
npm --version
pnpm --version
yarn --version
```

## Working with aliases

Aliases let you keep multiple independent installs of the same Node
version:

```powershell
gonv install 20.10.0 --as work
gonv install 20.10.0 --as personal

cd C:\code\work-project
gonv use work
gonv enable pnpm@8

cd C:\code\personal-project
gonv use personal
gonv enable pnpm@9
```

Each install has its own `node_modules\.bin`, its own pnpm/yarn binary,
its own everything. They never collide.

## Commands

| Command | What it does |
| --- | --- |
| `gonv install <version> [--as <alias>]` | Download Node and register it. Accepts partial versions (`v22`, `20.10`). |
| `gonv uninstall <name>` | Remove an install by version or alias, including its directory and DB records. |
| `gonv use <name>` | Pin the current directory (and subdirectories) to an install. |
| `gonv enable <pm>[@version]` | Install `pnpm` or `yarn` into the active install. Partial versions accepted. |
| `gonv list` | List all installs. |
| `gonv list-remote [--lts]` | List versions available from nodejs.org. Alias: `ls-remote`. |
| `gonv current` | Print the install active in the current directory. |
| `gonv shims` | Re-copy the shim binaries to `~/.gonv/shims`. |

## Layout

```
%USERPROFILE%\.gonv\
  gonv.db                      # SQLite store
  shims\
    node.exe                   # copies of gonv-shim.exe, dispatched by name
    npm.exe
    npx.exe
    pnpm.exe
    pnpx.exe
    yarn.exe
    corepack.exe
  versions\node\
    v20.10.0\                  # default install for Node 20.10.0
      node.exe
      pnpm.exe                 # only if `gonv enable pnpm` was run here
      yarn.cmd
      yarn-pkg\
      ...
    work\                      # alias install — same node version, different pms
      node.exe
      pnpm.exe
      ...
```

Database schema (managed automatically with online migrations from any
earlier version):

```
installs(name, node_version, installed_at)
directory_versions(path, install_name)
enabled_pm(install_name, name, version)
```

## How the shim works

Every command in `~/.gonv/shims` is a copy of `gonv-shim.exe`. When you
run `node script.js`:

1. The shim reads its own filename to learn it should proxy `node`.
2. It opens the gonv database and finds the longest-prefix directory
   entry that covers the current working directory.
3. The corresponding install name maps to a directory under
   `~/.gonv/versions/node/<name>`.
4. The shim execs `<install_dir>\node.exe` with the original argv,
   forwarding stdin/stdout/stderr and propagating the exit code.

`.cmd` and `.bat` targets (e.g. `yarn.cmd`) are launched through
`cmd.exe /c` because `CreateProcess` cannot run them directly.

## Status

Early. The basics work but there is no test suite yet and the project
has been used by exactly one person on exactly one machine.
