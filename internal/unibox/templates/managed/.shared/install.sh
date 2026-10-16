#!/bin/sh
# Managed by unibox: init, check and build rewrite .shared to unibox's own.
# Your own scripts go in _shared/@install; one of the same name wins there.
#
# Runs the named installation scripts in order, each from _shared/@install,
# the user's, or else .shared/@install, unibox's: agents/codex runs
# agents/codex.sh. Names come as arguments, or one per line on stdin - which
# lets a Dockerfile write them as a vertical list in a heredoc. Blank lines
# and #-comments pass. An executable script runs by its own shebang (nvm and
# sdkman need bash).
set -eu
context="$(cd "$(dirname "$0")/.." && pwd)"
run() {
    name="${1%.sh}"
    for dir in _shared/@install .shared/@install; do
        script="$context/$dir/$name.sh"
        if [ -f "$script" ]; then
            echo "installing $name from $dir"
            if [ -x "$script" ]; then
                "$script"
            else
                sh "$script"
            fi
            return
        fi
    done
    echo "no installation script $name in _shared/@install or .shared/@install" >&2
    exit 1
}
if [ "$#" -gt 0 ]; then
    for name in "$@"; do run "$name"; done
else
    while read -r name; do
        case "$name" in '' | \#*) continue ;; esac
        run "$name"
    done
fi
