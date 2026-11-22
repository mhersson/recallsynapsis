#!/bin/sh
# Fails on an em-dash in any tracked or untracked, unignored text file.
# A pipe hides git's exit status, so the listing is proved first: it must hold
# this script, else git failed or the scan runs from outside the repository root.
# Byte patterns under LC_ALL=C because macOS grep has no -P.
set -u
export LC_ALL=C

dash=$(printf '\342\200\224')
printf '\342\200\224\n' | grep -qF "$dash" || { echo "scan: grep cannot match an em-dash"; exit 1; }

git ls-files --cached --others --exclude-standard | grep -qxF scripts/scan.sh || { echo "scan: git cannot list this repository; run from its root"; exit 1; }

bad=$(git ls-files -z --cached --others --exclude-standard | xargs -0 grep -IlF "$dash" /dev/null) || :
if [ -n "$bad" ]; then
	echo "scan: em-dashes (use hyphens) in:"
	printf '%s\n' "$bad"
	exit 1
fi
