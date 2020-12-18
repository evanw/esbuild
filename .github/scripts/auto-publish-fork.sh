#!/usr/bin/env bash
set -euxo pipefail
shopt -s inherit_errexit

echo '//registry.npmjs.org/:_authToken=${NPM_AUTOMATION_TOKEN}' > .npmrc
npm whoami

pr_number="$1"

# Auto-merge latest upstream changes
# git remote add evanw https://github.com/evanw/esbuild
# git fetch evanw
# git merge evanw/master

# Branch descriptions
# evanw/master: stable upstream
# origin/master: hacks to publish this fork, changes package names
# ab/worker_threads: changes to support worker_threads

# Fetch full checkout
git remote add cspotcode https://github.com/cspotcode/esbuild || true
git fetch cspotcode

git config user.name "Committer"
git config user.email "committer@example.com"

# Merge in the pull request
git fetch cspotcode refs/pull/$pr_number/head:PR_HEAD
git merge PR_HEAD

# Add unique datestamp suffix to version number
echo "$(cat version.txt)-$(node -p 'new Date().toISOString().replace(/:|\./g, "-")')" > version.txt

# Test without token
NPM_AUTOMATION_TOKEN="" make test-prepublish
# publish with token
make publish-all
