#!/usr/bin/env bats
#
# .githooks/commit-msg: the subject lines a commit is allowed to carry. The
# hook takes the message file as its only argument, so every test writes one
# and runs the hook against it — no repository, no commit, no network.

bats_require_minimum_version 1.5.0

HOOK="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)/.githooks/commit-msg"

# msg LINE... — write the message file, one argument per line, and run the hook.
msg() {
  printf '%s\n' "$@" >"$BATS_TEST_TMPDIR/msg"
  run "$HOOK" "$BATS_TEST_TMPDIR/msg"
}

@test "accepts every allowed type" {
  for type in build chore ci docs feat fix perf refactor revert style test; do
    msg "$type: something"
    [ "$status" -eq 0 ] || { echo "$type rejected: $output"; return 1; }
  done
}

@test "accepts a scope and a breaking marker" {
  msg 'feat(proxy)!: drop the socks5 fallback'
  [ "$status" -eq 0 ]
}

@test "rejects an unknown type" {
  msg 'feet: a typo'
  [ "$status" -eq 1 ]
  [[ "$output" == *'not a conventional commit'* ]]
}

@test "rejects a missing type" {
  msg 'just some words'
  [ "$status" -eq 1 ]
}

@test "rejects an empty description" {
  msg 'fix: '
  [ "$status" -eq 1 ]
}

@test "rejects a missing space after the colon" {
  msg 'fix:nospace'
  [ "$status" -eq 1 ]
}

@test "rejects a trailing full stop" {
  msg 'fix: no period here.'
  [ "$status" -eq 1 ]
  [[ "$output" == *'full stop'* ]]
}

@test "rejects a subject over 100 characters" {
  msg "fix: $(printf 'x%.0s' $(seq 96))"
  [ "$status" -eq 1 ]
  [[ "$output" == *'over the 100'* ]]

  msg "fix: $(printf 'x%.0s' $(seq 95))"
  [ "$status" -eq 0 ]
}

@test "lets git's own merge and revert subjects through" {
  msg "Merge branch 'main' into topic"
  [ "$status" -eq 0 ]

  msg 'Revert "feat: something"'
  [ "$status" -eq 0 ]
}

@test "reads past comments and stops at the first real line" {
  msg '# please enter the commit message' '' 'fix: after the comment'
  [ "$status" -eq 0 ]
}

@test "ignores the diff git commit -v appends" {
  msg 'fix: real subject' '' '# ------------------------ >8 ------------------------' \
    'diff --git a/x b/x' '+bad subject.'
  [ "$status" -eq 0 ]
}

@test "rejects an empty message" {
  msg '' ''
  [ "$status" -eq 1 ]
}
