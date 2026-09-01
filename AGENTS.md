# AGENTS.md

Behavioral rules for coding agents working on **tailscale-socks**: a userspace Tailscale node (`tsnet`) exposing a tailnet through local SOCKS5, HTTP and DNS front doors. Go, ~1300 lines, no server, no UI.

Project facts — flags, defaults, layout, how to run it — live in the README and `.env.example`. Read them before your first change.

## 1. Think Before Coding

**Read first. Don't assume. Don't hide confusion.**

- Read the code the change touches — every caller, the real flow — before editing. The whole codebase fits in one sitting; there is no excuse for guessing.
- `tsnet`, `ipn.Prefs`, `dns.Manager` and `go-socks5` are large, under-documented APIs. Check the vendored source under `$(go env GOMODCACHE)` instead of inventing a method that "should" exist.
- State your assumptions explicitly. Uncertain → ask.
- Multiple valid interpretations → present them, don't pick silently.
- A simpler approach exists → say so. Push back when warranted.
- Unclear or contradictory → stop, name what's confusing, ask. Don't guess and continue.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond the ask.
- No abstraction with one caller — inline it.
- No config for a value that never changes — hardcode it.
- No error handling for impossible states.
- 200 lines that could be 50 → rewrite it.

Reuse before writing, in this order: something already in this repo → the standard library (`net`, `net/http`, `net/netip`, `context`) → what `tsnet` and `tailscale.com/*` already do → a dependency that is already in `go.mod`. A new direct dependency needs a reason stated up front.

`net/netip` over `net.IP`. `net/http` over a proxy library. `dnsmessage` over a hand-rolled parser.

The test: would a senior Go engineer call this overcomplicated? Then simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

- Don't reformat, rename or "improve" adjacent code, comments or tests.
- Don't refactor working code as a side effect. Match existing style, even if you'd do it differently.
- Unrelated dead code → mention it, don't delete it.
- Remove the imports and variables YOUR change orphaned; leave pre-existing dead code alone.
- Never run `go mod tidy` or bump `tailscale.com` as a side effect of an unrelated change. Dependency moves are their own commit.

Fix the root cause, not the symptom: one guard in `Node.DialContext` beats a guard in each of the three servers — and patching only the reported path leaves the sibling callers broken.

The test: every changed line traces directly to the request.

## 4. Verify Before Done

**Define the check first. Not done until it passes.**

```sh
make check     # lint + go test -race ./... + test-sh
make test-sh   # zsh -n over contrib/ + the bats suites in contrib/test and packaging/test
make build     # host binary; OS=all ARCH=all for the full matrix
make cover     # per-function statement coverage; cover-html opens a browser
make vuln      # govulncheck; needs the network, so it is not part of check
make outdated  # direct deps and pinned tools with a newer version; network too
```

`make check` is the gate, and it now covers the shell too: `make test-sh` needs `zsh` and `bats` >= 1.5.0 (`brew install bats-core`). There is no linter for zsh — shellcheck rejects the dialect with SC1071 — so `zsh -n` is the syntax check.

`goimports`, `staticcheck` and `govulncheck` are pinned in the `tool` block of `go.mod` and run through `go tool` — never install them separately, never `go run ...@latest`, never add a second linter config.

Turn the task into something verifiable:

- "Add validation" → table-driven test over the invalid inputs, then make it pass.
- "Fix the bug" → reproducing test first. Watch it fail, then fix, then watch it pass.
- "Refactor X" → `make check` green before and after.

**Tests never touch the network and never bring up a node.** `tsnet.Server.Up` needs a real tailnet and a login; a test that calls it is broken, not slow. Test the pure parts instead — the pattern is already in the repo:

- fake the seam: `proxy.DialFunc` and `proxy.DNSBackend` are interfaces so the servers can be tested without a tailnet, and `Node.dialTS`/`Node.lookupIP` are function fields so `DialContext` can be.
- split the pure part out of the one that needs a live node: `describe` from `Describe`, `prefsFor` from `applyPrefs`. That is where the logic worth testing lives.
- build fixtures by hand: `testStatus()` in `internal/tsnode` fakes an `ipnstate.Status`.
- table-driven subtests, `t.Helper()` in helpers, `t.TempDir()` for anything on disk.

Anything that only proves itself on the wire gets a **manual** smoke check, run and reported, not a test:

```sh
./tailscale-socks --exit-node auto
curl --socks5-hostname 127.0.0.1:1080 http://peer.tailnet.ts.net/
curl --proxy http://127.0.0.1:8080    http://peer.tailnet.ts.net/
dig @127.0.0.1 -p 5354 peer.tailnet.ts.net
./tailscale-socks status
```

Multi-step work → state the plan up front:

```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
```

"Looks right" is not verification. Run the check and report the real result. Tests failed, `staticcheck` complained, or a step was skipped → say so, with the output.

## 5. Git & PR

**Commits, pushes and PR replies are visible to the team. Each needs an explicit request.**

- Never commit unless asked — wait, even with changes staged.
- Never push unless asked.
- Never add a co-author.
- Never reply to PR comments unless asked. Draft it, let the human post.
- Never commit `.env`, a state directory, `dist/` or the built binary.

## 6. Go Style

**Readable at a glance. Flat, named, spaced. Idiomatic before clever.**

- `gofmt` via `make fmt` (`goimports -local github.com/d0whc3r/tailscale-socks`). Import groups: stdlib, third party, this module. Never hand-format imports.
- Package comment on every package, doc comment on every exported identifier, starting with its name. Existing packages all do this; keep it that way.
- Comments say *why*, not *what the code already says*. The good ones in this repo explain a non-obvious decision — why the resolver returns a nil IP, why the state dir ignores the binary name, why the DNS default is 5354.
- Wrap errors with context and `%w`: `fmt.Errorf("joining tailnet: %w", err)`. Lowercase, no trailing punctuation, no "failed to". Several failures worth reporting → `errors.Join`.
- A condition a caller must branch on becomes a sentinel: `var ErrNoDNS = errors.New(...)`, matched with `errors.Is`.
- Interfaces are declared by the consumer, not the producer — `proxy.DNSBackend` lives in `proxy`, and `*tsnode.Node` satisfies it without knowing. Accept a function type or a small interface; return concrete types.
- `context.Context` is the first parameter, plumbed through, never stored in a struct.
- Flat over nested. Early `return` / `continue` instead of an `else` pyramid.
- Extract spec-derived values into named constants — `maxDNSMessage`, `hopByHopHeaders` — even with one use. Self-explanatory one-offs stay inline.
- Every `go func` needs an exit path. Server loops report their error on a buffered channel; nothing is fired and forgotten.
- Close what you open, with `defer`, on the line after you open it.
- No package-level mutable state. No `init()`. No `panic` outside `main`.
- Export nothing that nothing imports. Widening an unexported symbol is a design change — ask first.
- Let the reader breathe: blank line between logical blocks, one short comment for a block that needs it.

## 7. Project Invariants

**Break one of these and the program is subtly wrong, not obviously broken.**

- **Layering.** `cmd/` wires, `internal/tsnode` owns the tailnet, `internal/proxy` owns the wire protocols. `internal/proxy` must never import `internal/tsnode` — that separation is what makes the servers testable without a tailnet. `cmd/` holds no protocol logic. One command per file in `cmd/` (`run.go`, `status.go`, `config.go`); the flag structs and the help text that documents them share `flags.go`; `main.go` only parses and wires. Split by what changes together, not by size.
- **Flags are a four-place contract.** Adding or changing a flag means the kong struct tag, `.env.example`, the README table and `configCmd.settings` — all four, same change. `TestFlagEnvVars` fails when the environment variable drifts; `TestConfigSettingsCoverEveryFlag` fails when a flag never reaches the dump. Env vars are `TSPROXY_*` via `kong.DefaultEnvars`, except `TS_AUTHKEY` and `TS_CONTROL_URL`, which are Tailscale's own names: never rename them, never prefix them.
- **The state directory must not depend on the binary's name.** `tsnet`'s own default does, so renaming the binary would silently lose the login and register a second node. `DefaultStateDir` keys on the hostname only. State file stays `0600`, its directory `0700`.
- **The proxies have no authentication.** Defaults bind `127.0.0.1` and stay that way. Never change a default to `0.0.0.0`, never add a "listen everywhere" convenience, never widen a bind without an explicit request — the failure mode is an open relay into someone's tailnet.
- **Credentials never reach a log line.** `TS_AUTHKEY` and the contents of `tailscaled.state` are secrets: don't print them, don't echo them into an error, don't add them to `status` or `config` output — `config` is made to be piped and eval'd.
- **Names resolve on the tailnet side.** SOCKS5 hands the host name to the dialer unresolved on purpose; "fixing" the resolver to return a real IP breaks MagicDNS and split DNS for every client.
- **Fail fast before joining.** Validate addresses and flag combinations before `tsnode.Start`, so a typo doesn't cost a tailnet login.
- **Every listener is optional.** An empty address disables it; all three empty is an error. Keep new listeners to the same shape.

## 8. Words

**Fewest words that carry the meaning.**

- Comments, commit messages, PR bodies, replies to prompts: cut every word that isn't load-bearing. Less is more.
- No superlatives, no praise, no "you're absolutely right". Cold, factual.
- User-facing output is one lowercase line per fact, aligned like `Describe` does it. No banners, no emoji, no color.

---

**Working if:** smaller diffs, `make check` green on the first try more often, clarifying questions before implementation instead of corrections after.
