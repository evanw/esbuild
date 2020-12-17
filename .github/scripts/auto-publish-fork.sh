#!/usr/bin/env bash
set -euxo pipefail
shopt -s inherit_errexit

# Auto-merge latest upstream changes
# git remote add evanw https://github.com/evanw/esbuild
# git fetch evanw
# git merge evanw/master

# Branch descriptions
# evanw/master: stable upstream
# origin/master: hacks to publish this fork, changes package names
# ab/worker_threads: changes to support worker_threads

# Auto-merge master (still need to manually merge upstream from evanw)
git remote add cspotcode https://github.com/cspotcode/esbuild || true
git fetch cspotcode
git merge cspotcode/master

# Add unique datestamp suffix to version number
echo "$(cat version.txt)-$(node -p 'new Date().toISOString().replace(/:|\./g, "-")')" > version.txt
# TODO this will fail
make publish-all
