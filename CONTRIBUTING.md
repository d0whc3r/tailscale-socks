# Contributing

Small project: ~1300 lines of Go in three packages, no server, no UI. Read
[docs/architecture.md](docs/architecture.md) before the first change — the whole
thing fits in one sitting.

## Build

Go 1.27 or newer. Nothing else to install: `goimports`, `staticcheck` and
`govulncheck` are pinned in the `tool` block of `go.mod` and run through
`go tool`.

```sh
make build      # host binary -> ./tailscale-socks
make run        # build, then run it
make check      # the gate: goimports -l + go vet + staticcheck + go test -race
make cover      # per-function statement coverage; cover-html opens a browser
make vuln       # govulncheck (needs the network, so not part of check)
make outdated   # direct deps and pinned tools with a newer version (network too)
make fmt        # rewrite in place: gofmt + import grouping
make hooks      # point git at .githooks/: check what each commit touches
make release    # GoReleaser dry run of the release matrix into dist/
make clean
```

`make build` takes `OS` and `ARCH`; either one set to `all` expands to the
matching entries of `PLATFORMS`. A plain host build stays unstripped and lands
in `./`; anything cross-compiled is stripped and lands in `dist/`.

```sh
make build OS=linux ARCH=arm64
make build OS=all ARCH=arm64      # every arm64 target
```

**`make check` is the gate.** Never install the tools separately, never
`go run ...@latest`, never add a second linter config.

`make hooks` sets `core.hooksPath` to the tracked `.githooks/` directory — one
command per clone, no hook manager to install. `.githooks/pre-commit` picks its
checks from what is staged:

| staged | runs |
| --- | --- |
| `*.go`, `go.mod`, `go.sum` | `make fmt`, re-stage the rewritten files, `make lint test` |
| `*.zsh`, `*.bats`, `*.bash` | `make test-sh` |
| anything else | nothing |

Both groups staged means both run, which is `make check` plus the formatting
pass. `git commit --no-verify` skips the hook entirely.

Only files staged in full are re-staged after `make fmt`. A partially staged
file — `git add -p`, or edited again after `git add` — is named and left alone,
because `git add` would take the whole working tree copy and commit the hunks
you kept back.

## Tests

**Tests never touch the network and never bring up a node.** `tsnet.Server.Up`
needs a real tailnet and a login; a test that calls it is broken, not slow.
Test the pure parts — the patterns are already in the repo:

- **Fake the seam.** `proxy.DialFunc` and `proxy.DNSBackend` are interfaces so
  the servers can be exercised end to end without a tailnet; `Node.dialTS` and
  `Node.lookupIP` are function fields so `DialContext` can be.
- **Build fixtures by hand.** `testStatus()` in `internal/tsnode` fakes an
  `ipnstate.Status`; `describe` is split from `Describe`, and `prefsFor` from
  `applyPrefs`, so each can be tested
  against it.
- Table-driven subtests, `t.Helper()` in helpers, `t.TempDir()` for anything on
  disk.

### The zsh helpers

`contrib/` has its own suite, in [bats](https://github.com/bats-core/bats-core)
(1.5.0 or newer), run by `make test-sh` along with a `zsh -n` syntax pass.
Same rule: no network, no node.

bats runs in bash and cannot call a zsh function directly, so `contrib/test/`
has a seam. `contrib/test/harness.zsh` sources the contrib script under a
chosen `$OSTYPE` and evals a zsh snippet piped in on stdin; the bats side wraps
it in `zsh_run`. Write the snippet as a **quoted** heredoc, so it reaches zsh
byte for byte:

```bash
@test "OSTYPE=msys selects the Task Scheduler backend" {
  run zsh_run msys <<'ZSH'
print -r -- "$TS_SOCKS_OS"
ZSH
  [ "$output" = "windows" ]
}
```

Because the service manager is a shell function in the test, every backend is
exercised wherever the suite runs: the Windows task is verified from macOS and
the launchd agent from Linux.

- **Stub with a function** for anything the code calls as a command —
  `launchctl`, `systemctl`, `powershell`, `cygpath`. A function shadows the
  real program.
- **Stub with a file** when the code tests `$commands` instead of calling —
  `_ts_svc_check` does, so use the `stub_exe` helper.
- `isolate` points `$HOME` at the test's temp directory. Call it in `setup()`
  before anything writes a plist or a unit.
- Assert on the **generated artefact**, not on the call: read back the plist,
  the unit, the `run.cmd`, the `Register-ScheduledTask` line.

Turn the task into something verifiable before writing code:

- "Add validation" → a table-driven test over the invalid inputs, then make it
  pass.
- "Fix the bug" → the reproducing test first. Watch it fail, then fix it.
- "Refactor X" → `make check` green before and after.

Anything that only proves itself on the wire gets a **manual** smoke check, run
and reported, not a test:

```sh
./tailscale-socks --exit-node auto
curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
curl --proxy http://127.0.0.1:8080    http://peer.tailnet.ts.net/
dig @127.0.0.1 -p 5354 peer.tailnet.ts.net
./tailscale-socks status
```

"Looks right" is not verification. If a test failed, `staticcheck` complained,
or a step was skipped, say so, with the output.

## Style

- `gofmt` through `make fmt` (`goimports -local github.com/d0whc3r/tailscale-socks`).
  Import groups: stdlib, third party, this module. Never hand-format imports.
- Package comment on every package, doc comment on every exported identifier,
  starting with its name.
- Comments say *why*, not what the code already says. The good ones here explain
  a non-obvious decision — why the SOCKS5 resolver returns a nil IP, why the
  state directory ignores the binary name, why the DNS default is 5354.
- Wrap errors with context and `%w`: `fmt.Errorf("joining tailnet: %w", err)`.
  Lowercase, no trailing punctuation, no "failed to". Several failures worth
  reporting → `errors.Join`.
- A condition a caller must branch on becomes a sentinel: `var ErrNoDNS = ...`,
  matched with `errors.Is`.
- Interfaces are declared by the consumer, not the producer. Accept a function
  type or a small interface; return concrete types.
- `context.Context` is the first parameter, plumbed through, never stored in a
  struct.
- Flat over nested: early `return`/`continue` instead of an `else` pyramid.
- Extract spec-derived values into named constants (`maxDNSMessage`,
  `hopByHopHeaders`), even with one use.
- Every `go func` needs an exit path. Server loops report their error on a
  buffered channel; nothing is fired and forgotten.
- Close what you open, with `defer`, on the line after you open it.
- No package-level mutable state, no `init()`, no `panic` outside `main`.
- Export nothing that nothing imports.

Prefer, in this order: something already in this repo → the standard library →
what `tsnet` and `tailscale.com/*` already do → a dependency already in
`go.mod`. A new direct dependency needs a reason stated up front.

`tsnet`, `ipn.Prefs`, `dns.Manager` and `go-socks5` are large and
under-documented. Check the source under `$(go env GOMODCACHE)` instead of
inventing a method that "should" exist.

## Invariants

Break one of these and the program is subtly wrong, not obviously broken.

- **Layering.** `cmd/` wires, `internal/tsnode` owns the tailnet,
  `internal/proxy` owns the wire protocols. `internal/proxy` must never import
  `internal/tsnode`. `cmd/` holds no protocol logic. One command per file in
  `cmd/` (`run.go`, `status.go`, `config.go`); the flag structs and the help
  text that documents them share `flags.go`; `main.go` only parses and wires.
  Split by what changes together, not by size.
- **A flag is a four-place contract.** Adding or changing one means the kong
  struct tag, `.env.example`, the README table and `configCmd.settings` — all
  four, same change. `TestFlagEnvVars` fails when the environment variable
  drifts; `TestConfigSettingsCoverEveryFlag` fails when a flag never reaches
  the dump. Variables are `TSPROXY_*`, except `TS_AUTHKEY` and `TS_CONTROL_URL`,
  which are Tailscale's own names: never rename or prefix them.
- **The state directory must not depend on the binary's name.** `tsnet`'s own
  default does, so renaming the binary would silently lose the login and
  register a second node. State file `0600`, directory `0700`.
- **The proxies have no authentication.** Defaults bind `127.0.0.1` and stay
  that way. Never change a default to `0.0.0.0`, never add a "listen
  everywhere" convenience — the failure mode is an open relay into someone's
  tailnet.
- **Credentials never reach a log line.** `TS_AUTHKEY` and the contents of
  `tailscaled.state` are secrets: not in logs, not in errors, not in `status` or
  `config` output.
- **Names resolve on the tailnet side.** SOCKS5 hands the host name to the
  dialer unresolved on purpose; "fixing" the resolver to return a real IP breaks
  MagicDNS and split DNS for every client.
- **Fail fast before joining.** Validate addresses and flag combinations before
  `tsnode.Start`, so a typo doesn't cost a tailnet login.
- **Every listener is optional.** An empty address disables it; all three empty
  is an error. Keep new listeners to the same shape.

## Changes

- Touch only what the change requires. Don't reformat, rename or "improve"
  adjacent code as a side effect. Match the existing style.
- Remove the imports and variables *your* change orphaned; leave pre-existing
  dead code alone and mention it instead.
- Fix the root cause: one guard in `Node.DialContext` beats a guard in each of
  the three servers.
- Never run `go mod tidy` or bump `tailscale.com` as a side effect of an
  unrelated change. Dependency moves are their own commit.
- Never commit `.env`, a state directory, `dist/` or the built binary.
- Commit messages and PR bodies: fewest words that carry the meaning.

## CI and releases

`.github/workflows/ci.yml` runs `make check` and `make build` on every push and
pull request, publishes the coverage table to the run summary, and runs
`make vuln` in a separate job (also weekly, so advisories published after the
last commit are still caught).

Pushing a `v*` tag runs [GoReleaser](https://goreleaser.com)
(`.goreleaser.yaml`): it builds the same matrix, writes `SHA256SUMS.txt` and
creates the GitHub release with the binaries attached and the notes grouped by
conventional commit type — `feat:` under Features, `fix:` under Fixes, and
`docs:`/`test:`/`chore:`/`ci:`/`style:`/`refactor:` dropped. A commit subject
without a type still ships, under Other. The version in the binary is the tag
itself, read back through `debug.ReadBuildInfo`, so there is nothing to bump in
the tree: pick the tag, push it, done.

`make release` runs the same config locally as a snapshot; it needs
`brew install goreleaser`, while `make build OS=all ARCH=all` cross-builds the
matrix with the toolchain alone. Dependabot opens weekly grouped PRs for direct
Go dependencies and actions.

## Security and license

Found a vulnerability? Don't open a public issue or pull request — report it
privately, as described in [SECURITY.md](SECURITY.md).

Contributions are licensed under the [MIT License](LICENSE), same as the rest
of the project.
