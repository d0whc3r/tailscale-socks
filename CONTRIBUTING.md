# Contributing

Small project: ~1300 lines of Go in three packages, no server, no UI. Read [docs/architecture.md](docs/architecture.md) before the first change — the whole thing fits in one sitting.

## Build

Go 1.27 or newer. Nothing else to install: `goimports`, `staticcheck` and `govulncheck` are pinned in the `tool` block of `go.mod` and run through `go tool`.

```sh
make build      # host binary -> ./tailscale-socks
make run        # build, then run it
make check      # the gate: Go lint/tests plus the zsh and POSIX shell checks
make cover      # per-function statement coverage; cover-html opens a browser
make vuln       # govulncheck (needs the network, so not part of check)
make outdated   # direct deps and pinned tools with a newer version (network too)
make fmt        # rewrite in place: gofmt + import grouping
make hooks      # point git at .githooks/: check what each commit touches and says
make release    # GoReleaser dry run: the three installers into dist/
make clean
```

`make build` takes `OS` and `ARCH`; either one set to `all` expands to the matching entries of `PLATFORMS`. A plain host build stays unstripped and lands in `./`; anything cross-compiled is stripped and lands in `dist/`.

```sh
make build OS=linux ARCH=arm64
make build OS=all ARCH=arm64      # every arm64 target
```

**`make check` is the gate.** Never install the tools separately, never `go run ...@latest`, never add a second linter config.

`make hooks` sets `core.hooksPath` to the tracked `.githooks/` directory — one command per clone, no hook manager to install. `.githooks/pre-commit` picks its checks from what is staged:

| staged | runs |
| --- | --- |
| `*.go`, `go.mod`, `go.sum` | `make fmt`, re-stage the rewritten files, `make lint test` |
| `*.zsh`, `*.bats`, `*.bash`, `*.sh`, `*.command`, `.githooks/*` | `make test-sh` |
| anything else | nothing |

Both groups staged means both run, which is `make check` plus the formatting pass. `git commit --no-verify` skips the hook entirely.

Only files staged in full are re-staged after `make fmt`. A partially staged file — `git add -p`, or edited again after `git add` — is named and left alone, because `git add` would take the whole working tree copy and commit the hunks you kept back.

`.githooks/commit-msg` checks the subject line against the conventional commit grammar, `type(scope)!: description`:

| rule | |
| --- | --- |
| type | one of `build chore ci docs feat fix perf refactor revert style test` |
| scope | optional, in parentheses |
| `!` | optional, before the colon |
| description | non-empty, after `: `, no trailing full stop |
| subject | 100 characters at most |

The subjects `git merge` and `git revert` write themselves are let through, and so is everything below the first line. This is not house style: `packaging/next-version.sh` reads these prefixes to pick the next tag and GoReleaser groups the release notes by them, so a subject the pattern misses ships unversioned and unmentioned. Same escape hatch, `git commit --no-verify`.

## Tests

**Tests never touch the network and never bring up a node.** `tsnet.Server.Up` needs a real tailnet and a login; a test that calls it is broken, not slow. Test the pure parts — the patterns are already in the repo:

- **Fake the seam.** `proxy.DialFunc` and `proxy.DNSBackend` are interfaces so the servers can be exercised end to end without a tailnet; `Node.dialTS` and `Node.lookupIP` are function fields so `DialContext` can be.
- **Build fixtures by hand.** `testStatus()` in `internal/tsnode` fakes an `ipnstate.Status`; `describe` is split from `Describe`, and `prefsFor` from `applyPrefs`, so each can be tested against it.
- Table-driven subtests, `t.Helper()` in helpers, `t.TempDir()` for anything on disk.

### The zsh helpers

`contrib/` has its own suite, in [bats](https://github.com/bats-core/bats-core) (1.5.0 or newer), along with `packaging/test`, run by `make test-sh` with a `zsh -n` syntax pass over the zsh and an `sh -n` pass over `contrib/install.sh` and `packaging/*.sh`. Same rule: no network, no node — the installer suite points `TSPROXY_BASE_URL` at a fixture directory over `file://`, so nothing it runs leaves the machine. `packaging/windows.nsi` and `packaging/path.ps1` have no syntax check of their own — `make release` compiles the one and the Windows installer exercises the other.

bats runs in bash and cannot call a zsh function directly, so `contrib/test/` has a seam. `contrib/test/harness.zsh` sources the contrib script under a chosen `$OSTYPE` and evals a zsh snippet piped in on stdin; the bats side wraps it in `zsh_run`. Write the snippet as a **quoted** heredoc, so it reaches zsh byte for byte:

```bash
@test "OSTYPE=msys selects the Task Scheduler backend" {
  run zsh_run msys <<'ZSH'
print -r -- "$TS_SOCKS_OS"
ZSH
  [ "$output" = "windows" ]
}
```

Because the service manager is a shell function in the test, every backend is exercised wherever the suite runs: the Windows task is verified from macOS and the launchd agent from Linux.

- **Stub with a function** for anything the code calls as a command — `launchctl`, `systemctl`, `powershell`, `cygpath`. A function shadows the real program.
- **Stub with a file** when the code tests `$commands` instead of calling — `_ts_svc_check` does, so use the `stub_exe` helper.
- `isolate` points `$HOME` at the test's temp directory. Call it in `setup()` before anything writes a plist or a unit.
- Assert on the **generated artefact**, not on the call: read back the plist, the unit, the `run.cmd`, the `Register-ScheduledTask` line.

Turn the task into something verifiable before writing code:

- "Add validation" → a table-driven test over the invalid inputs, then make it pass.
- "Fix the bug" → the reproducing test first. Watch it fail, then fix it.
- "Refactor X" → `make check` green before and after.

Anything that only proves itself on the wire gets a **manual** smoke check, run and reported, not a test:

```sh
./tailscale-socks --exit-node auto
curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
curl --proxy http://127.0.0.1:8080    http://peer.tailnet.ts.net/
dig @127.0.0.1 -p 5354 peer.tailnet.ts.net
./tailscale-socks status
```

"Looks right" is not verification. If a test failed, `staticcheck` complained, or a step was skipped, say so, with the output.

## Style

- `gofmt` through `make fmt` (`goimports -local github.com/d0whc3r/tailscale-socks`). Import groups: stdlib, third party, this module. Never hand-format imports.
- Package comment on every package, doc comment on every exported identifier, starting with its name.
- Comments say *why*, not what the code already says. The good ones here explain a non-obvious decision — why the SOCKS5 resolver returns a nil IP, why the state directory ignores the binary name, why the DNS default is 5354.
- Wrap errors with context and `%w`: `fmt.Errorf("joining tailnet: %w", err)`. Lowercase, no trailing punctuation, no "failed to". Several failures worth reporting → `errors.Join`.
- A condition a caller must branch on becomes a sentinel: `var ErrNoDNS = ...`, matched with `errors.Is`.
- Interfaces are declared by the consumer, not the producer. Accept a function type or a small interface; return concrete types.
- `context.Context` is the first parameter, plumbed through, never stored in a struct.
- Flat over nested: early `return`/`continue` instead of an `else` pyramid.
- Extract spec-derived values into named constants (`maxDNSMessage`, `hopByHopHeaders`), even with one use.
- Every `go func` needs an exit path. Server loops report their error on a buffered channel; nothing is fired and forgotten.
- Close what you open, with `defer`, on the line after you open it.
- No package-level mutable state, no `init()`, no `panic` outside `main`.
- Export nothing that nothing imports.

Prefer, in this order: something already in this repo → the standard library → what `tsnet` and `tailscale.com/*` already do → a dependency already in `go.mod`. A new direct dependency needs a reason stated up front.

`tsnet`, `ipn.Prefs`, `dns.Manager` and `go-socks5` are large and under-documented. Check the source under `$(go env GOMODCACHE)` instead of inventing a method that "should" exist.

## Invariants

Break one of these and the program is subtly wrong, not obviously broken.

- **Layering.** `cmd/` wires, `internal/tsnode` owns the tailnet, `internal/proxy` owns the wire protocols. `internal/proxy` must never import `internal/tsnode`. `cmd/` holds no protocol logic. One command per file in `cmd/` (`run.go`, `status.go`, `config.go`); the flag structs and the help text that documents them share `flags.go`; `main.go` only parses and wires. Split by what changes together, not by size.
- **A flag is a four-place contract.** Adding or changing one means the kong struct tag, `.env.example`, the README table and `configCmd.settings` — all four, same change. `TestFlagEnvVars` fails when the environment variable drifts; `TestConfigSettingsCoverEveryFlag` fails when a flag never reaches the dump. Variables are `TSPROXY_*`, except `TS_AUTHKEY` and `TS_CONTROL_URL`, which are Tailscale's own names: never rename or prefix them.
- **The state directory must not depend on the binary's name.** `tsnet`'s own default does, so renaming the binary would silently lose the login and register a second node. State file `0600`, directory `0700`.
- **The proxies have no authentication.** Defaults bind `127.0.0.1` and stay that way. Never change a default to `0.0.0.0`, never add a "listen everywhere" convenience — the failure mode is an open relay into someone's tailnet.
- **Credentials never reach a log line.** `TS_AUTHKEY` and the contents of `tailscaled.state` are secrets: not in logs, not in errors, not in `status` or `config` output.
- **Names resolve on the tailnet side.** SOCKS5 hands the host name to the dialer unresolved on purpose; "fixing" the resolver to return a real IP breaks MagicDNS and split DNS for every client.
- **Fail fast before joining.** Validate addresses and flag combinations before `tsnode.Start`, so a typo doesn't cost a tailnet login.
- **Every listener is optional.** An empty address disables it; all three empty is an error. Keep new listeners to the same shape.

## Changes

- Touch only what the change requires. Don't reformat, rename or "improve" adjacent code as a side effect. Match the existing style.
- Remove the imports and variables *your* change orphaned; leave pre-existing dead code alone and mention it instead.
- Fix the root cause: one guard in `Node.DialContext` beats a guard in each of the three servers.
- Never run `go mod tidy` or bump `tailscale.com` as a side effect of an unrelated change. Dependency moves are their own commit.
- Never commit `.env`, a state directory, `dist/` or the built binary.
- Commit messages and PR bodies: fewest words that carry the meaning.

## CI and releases

`.github/workflows/ci.yml` runs `make check` and `make build` on every push and pull request, publishes the coverage table to the run summary, and runs `make vuln` in a separate job (also weekly, so advisories published after the last commit are still caught).

**Releases tag themselves.** A push to `main` whose commits since the last tag ask for one gets tagged by the `tag` job, right after `check` goes green — so no tag ever points at a commit CI rejected. `packaging/next-version.sh` reads the bump out of the conventional commits:

| in the log since the last tag | next version |
|---|---|
| `type!:` in a subject, or a `BREAKING CHANGE:` footer | major |
| `feat:` | minor |
| `fix:`, `perf:` | patch |
| anything else — `docs`, `test`, `chore`, `ci`, `style`, `refactor` | no release |

The highest bump in the range wins. The first tag is `v1.0.0`, since there is no earlier one to bump. Only subjects are matched for the type prefix, so a commit body that quotes `feat:` does not cut a release; `packaging/test/version.bats` covers each rule.

The `release` job then runs [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`) on that tag: it builds the release files, writes `SHA256SUMS.txt` and creates the GitHub release with notes grouped by conventional commit type. The version in the binary is the tag itself, read back through `debug.ReadBuildInfo`, so there is nothing to bump in the tree.

It hangs off the `tag` job rather than off a tag push, because a tag pushed with `GITHUB_TOKEN` deliberately does not start a workflow. `ci.yml` therefore listens to `main` only — a tag pushed by hand starts nothing, and pushing one is not how a release is cut any more. When the script finds HEAD already tagged it reports `push=false`, which is what lets a failed `release` be re-run without a second tag.

Each tag produces one installer for Linux and one for Windows, and nothing else: no tar.gz, no zip. Both are x86-64. On top of that come the bare files `contrib/install.sh` fetches — a binary per platform, the two zsh backends and the configuration template — which is the only way to install on macOS and the default one on Linux. Every platform ends up with the same payload: executable, zsh service helper for that platform, the configuration template.

The `.deb` and the Windows setup ship `.env.example` renamed to `.env`, so the user copies it without a rename; the loose asset keeps the `.env.example` name, because `contrib/install.sh` is what turns it into `~/.tailscale/.env`. On Windows that lands next to the executable, which `dotEnvPaths` reads, so it is the live configuration from the first install on: the NSIS script writes it with `SetOverwrite off` and the uninstaller leaves it, or a reinstall would clobber a file holding `TS_AUTHKEY`.

| Artifact | Built by | Payload goes to |
|---|---|---|
| `.deb` | GoReleaser `nfpms` (pure Go) | `/usr/bin`, `/usr/share/tailscale-socks`, `/usr/share/doc` |
| loose assets | GoReleaser `lipo` + `release.extra_files` | `~/.local/bin` and `~/.local/share/tailscale-socks`, put there by `contrib/install.sh` |
| setup `.exe` | `packaging/windows-nsis.sh` → `packaging/windows.nsi` → `makensis` | `%LOCALAPPDATA%\Programs\tailscale-socks`, added to the user `PATH` |

The Windows setup is built by a GoReleaser hook rather than by a step after the release, so its output is inside `SHA256SUMS.txt` like the `.deb`. The `nsi` script gets one architecture at a time from the build post-hook, which no-ops on every target that is not Windows.

GoReleaser's own archive step is turned off with `--skip=archive`, in the workflow and in `make release` alike: an empty `archives:` block would only make it fall back to its tar.gz default. With no archives the bare binaries are not artifacts either, so `release.extra_files` publishes them straight out of `dist/` and a `name_template` gives each one the unversioned name `contrib/install.sh` asks for — `checksum.extra_files` repeats the same globs and names, or `SHA256SUMS.txt` would list files nobody can download.

**macOS installs over `curl` because nothing there is signed**, and a Developer ID costs $99/year. Anything downloaded through a browser carries `com.apple.quarantine`, which propagates from a disk image to its contents and from there to every copy made of them; Gatekeeper kills an unsigned quarantined binary with `Killed: 9` and no diagnostic. That is what a `.dmg` used to ship into, and it is why there is no installer format worth building here: `curl` sets no quarantine attribute, so the plain assets work where a package does not. `contrib/install.sh` also strips the attribute from the binary it installs, for the copy that arrived some other way. If the project ever gets a Developer ID, sign the binary and notarize with `xcrun notarytool`, which does need a Mac.

**Everything else cross-builds, so the release job runs on ubuntu.** `nfpm` is pure Go and `makensis` is an apt package; nothing in the matrix needs a macOS runner.

`make release` runs the same config locally as a snapshot; it needs `brew install goreleaser makensis`, while `make build OS=all ARCH=all` cross-builds the full matrix with the toolchain alone. Renovate opens weekly grouped PRs for direct Go dependencies, tool dependencies, and actions.

## Security and license

Found a vulnerability? Don't open a public issue or pull request — report it privately, as described in [SECURITY.md](SECURITY.md).

Contributions are licensed under the [MIT License](LICENSE), same as the rest of the project.
