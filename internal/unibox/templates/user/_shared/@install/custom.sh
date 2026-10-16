#!/bin/sh
# Your own installation script. A profile's Dockerfile runs it by naming it
# in its list, as custom, beside unibox's (base, agents/claude, tools/nvm).
# One named as a script of unibox's in .shared/@install - agents/claude.sh,
# say - runs instead of it. unibox wrote this file once and leaves it to you:
# rename it, or remove it.
set -eu
