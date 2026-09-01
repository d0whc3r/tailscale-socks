#!/usr/bin/env zsh
#
# The seam between bats and zsh. bats runs in bash and cannot call a zsh
# function directly, so every test body arrives here as a zsh snippet on
# stdin: this sources the contrib script under a chosen $OSTYPE and evals the
# snippet in the same shell, where the functions and their state are reachable.
#
# Started with `zsh -f`: a test must never see the developer's ~/.zshrc.

# LOCALAPPDATA exists only on Windows, and the windows backend branches on it.
# Clear it so a stray value cannot decide the branch, and let a test set it
# on purpose.
unset LOCALAPPDATA
[[ -n $TS_TEST_LOCALAPPDATA ]] && LOCALAPPDATA=$TS_TEST_LOCALAPPDATA

OSTYPE=${TS_TEST_OSTYPE:?}
source ${TS_TEST_MAIN:?} || exit $?

eval "$(cat)"
