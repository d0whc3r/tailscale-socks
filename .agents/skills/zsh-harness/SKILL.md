---
name: zsh-harness
description: Use when changing contrib shell scripts or tests that mix Bash Bats helpers with zsh scripts. Covers the repository's bash-to-zsh seam and its verification rules.
---

# Zsh Harness

This repository treats Bash and zsh as two deliberate layers, not as
interchangeable shells.

- Bats and `contrib/test/helpers.bash` run in Bash. Keep Bash helper code in
  Bash syntax and quote every expansion passed between the layers.
- `contrib/*.zsh` runs in zsh. Do not copy Bash-only syntax into it.
- Bats cannot call a zsh function directly. Send the test snippet through
  `zsh_run OSTYPE` and a quoted heredoc so `contrib/test/harness.zsh`
  evaluates it in zsh byte for byte.
- `harness.zsh` runs with `zsh -f`; tests must not depend on the developer's
  `~/.zshrc`, aliases, options, or environment.
- Validate zsh syntax with `zsh -n`. Do not use ShellCheck on zsh files here:
  the project explicitly documents that ShellCheck rejects this dialect.
- Tests never start the real node or touch the network. Isolate `$HOME` and
  stub executables under `$PATH` as the existing helpers do.
- Finish with `make test-sh`; broader work still uses the repository gate
  `make check`.
