#!/bin/sh
# Double-clickable entry point on the mounted disk image. Finder runs a
# .command in Terminal and opens a .sh in a text editor, so this is the only
# reason the file exists: everything it does is in install.sh.

cd "$(dirname "$0")" || exit 1
exec ./install.sh
