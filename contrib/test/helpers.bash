# Shared setup for the .bats files in this directory. Load it with `load
# helpers` at the top of each one.

# `run --separate-stderr` and the other run flags need this; without it bats
# only warns and keeps the old behaviour.
bats_require_minimum_version 1.5.0

TS_TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TS_MAIN="$(cd "$TS_TEST_DIR/.." && pwd)/tailscale-socks.zsh"
TS_HARNESS="$TS_TEST_DIR/harness.zsh"

# zsh_run OSTYPE — source the contrib script under OSTYPE, then run the zsh
# snippet on stdin. Always feed it a quoted heredoc, so the snippet reaches
# zsh byte for byte with nothing in bash touching it first.
zsh_run() {
  TS_TEST_OSTYPE="$1" TS_TEST_MAIN="$TS_MAIN" zsh -f "$TS_HARNESS"
}

# isolate points $HOME at this test's temp directory. Every backend derives
# its paths from $HOME at source time, so this is what keeps a test from
# writing into the developer's real LaunchAgents or systemd directory.
isolate() { export HOME="$BATS_TEST_TMPDIR"; }

# stub_bin puts a fake tailscale-socks on $PATH: _ts_bin resolves it and
# _ts_cfg runs it. `config SETTING` answers from $TS_STUB_<setting>, so a test
# exports TS_STUB_socks5=127.0.0.1:1080 and leaves TS_STUB_dns unset for the
# disabled case. Nothing here may run the real binary — it would join a tailnet.
stub_bin() {
  local dir="$BATS_TEST_TMPDIR/bin"
  mkdir -p "$dir"
  cat > "$dir/tailscale-socks" <<'STUB'
#!/bin/sh
eval "printf '%s\n' \"\${TS_STUB_$2}\""
STUB
  chmod +x "$dir/tailscale-socks"
  export PATH="$dir:$PATH"
}

# stub_exe NAME BODY puts an executable on $PATH. Needed where a shell
# function will not do: _ts_svc_check tests $commands, which only a real file
# in $PATH populates.
stub_exe() {
  local dir="$BATS_TEST_TMPDIR/bin"
  mkdir -p "$dir"
  printf '#!/bin/sh\n%s\n' "$2" > "$dir/$1"
  chmod +x "$dir/$1"
  export PATH="$dir:$PATH"
}

# stub_help_bin puts a fake tailscale-socks on $PATH that answers the two
# things the completion reads: `--help`, with Kong-shaped output, and
# `config`, with the resolved settings.
stub_help_bin() {
  local dir="$BATS_TEST_TMPDIR/bin"
  mkdir -p "$dir"
  cat > "$dir/tailscale-socks" <<'STUB'
#!/bin/sh
case $* in
  *--help)
    printf 'Usage: tailscale-socks\n\n  --not-a-flag  only in the description\n\n'
    printf 'Flags:\n  -h, --help    Show help.\n'
    printf '  -s, --socks5="127.0.0.1:1080"    SOCKS5 listen address.\n'
    printf '  -r, --[no-]accept-routes    Accept subnet routes.\n'
    ;;
  config)
    printf "TSPROXY_SOCKS5='127.0.0.1:1080'\nTSPROXY_EXIT_NODE_ALLOW_LAN='false'\n"
    ;;
esac
STUB
  chmod +x "$dir/tailscale-socks"
  export PATH="$dir:$PATH"
}
