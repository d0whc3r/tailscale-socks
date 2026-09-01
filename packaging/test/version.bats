#!/usr/bin/env bats
#
# packaging/next-version.sh: the bump the conventional commits since the last
# tag ask for. Every test builds its own throwaway repository, so nothing here
# reads the repository it runs in and nothing touches the network.

bats_require_minimum_version 1.5.0

NEXT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)/packaging/next-version.sh"

setup() {
  # `make test-sh` also runs from the pre-commit hook, and git exports GIT_DIR
  # and GIT_INDEX_FILE to a hook. Left set, every git call below — `git config`
  # included — would hit the real repository instead of the throwaway one.
  unset GIT_DIR GIT_INDEX_FILE GIT_WORK_TREE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR

  cd "$BATS_TEST_TMPDIR" || return 1
  git init -q -b main .
  git config user.email test@example.com
  git config user.name test
  git config commit.gpgsign false
  git commit -q --allow-empty -m 'feat: first'
}

# commit MESSAGE... — one empty commit per argument.
commit() {
  local msg
  for msg in "$@"; do git commit -q --allow-empty -m "$msg"; done
}

@test "no tag yet starts at v1.0.0" {
  run "$NEXT"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = 'tag=v1.0.0' ]
  [ "${lines[1]}" = 'push=true' ]
}

@test "feat bumps the minor and resets the patch" {
  git tag -a v1.2.3 -m v1.2.3
  commit 'feat: something'
  run "$NEXT"
  [ "${lines[0]}" = 'tag=v1.3.0' ]
  [ "${lines[1]}" = 'push=true' ]
}

@test "fix bumps the patch" {
  git tag -a v1.2.3 -m v1.2.3
  commit 'fix(dns): resolve on the tailnet side'
  run "$NEXT"
  [ "${lines[0]}" = 'tag=v1.2.4' ]
}

@test "perf bumps the patch" {
  git tag -a v1.2.3 -m v1.2.3
  commit 'perf: reuse the buffer'
  run "$NEXT"
  [ "${lines[0]}" = 'tag=v1.2.4' ]
}

@test "a bang in the subject bumps the major" {
  git tag -a v1.2.3 -m v1.2.3
  commit 'feat(api)!: drop the flag'
  run "$NEXT"
  [ "${lines[0]}" = 'tag=v2.0.0' ]
}

@test "a BREAKING CHANGE footer bumps the major" {
  git tag -a v1.2.3 -m v1.2.3
  git commit -q --allow-empty -m 'fix: rename the state directory' \
    -m 'BREAKING CHANGE: the old login is not read any more'
  run "$NEXT"
  [ "${lines[0]}" = 'tag=v2.0.0' ]
}

@test "the highest bump wins whatever the order" {
  git tag -a v1.2.3 -m v1.2.3
  commit 'fix: a' 'refactor!: b' 'feat: c'
  run "$NEXT"
  [ "${lines[0]}" = 'tag=v2.0.0' ]
}

@test "docs, chores and refactors release nothing" {
  git tag -a v1.2.3 -m v1.2.3
  commit 'docs: readme' 'chore: bump' 'ci: cache' 'refactor: split' 'test: cover'
  run "$NEXT"
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "a conventional prefix quoted in a body releases nothing" {
  git tag -a v1.2.3 -m v1.2.3
  git commit -q --allow-empty -m 'docs: document the bump rules' \
    -m 'feat: minor, fix: patch, and a subject ending in ! is major'
  run "$NEXT"
  [ -z "$output" ]
}

@test "a tag on HEAD is reported without a push" {
  git tag -a v1.2.3 -m v1.2.3
  run "$NEXT"
  [ "${lines[0]}" = 'tag=v1.2.3' ]
  [ "${lines[1]}" = 'push=false' ]
}

@test "a lightweight tag counts as the last one" {
  git tag v0.9.0
  commit 'feat: something'
  run "$NEXT"
  [ "${lines[0]}" = 'tag=v0.10.0' ]
}
