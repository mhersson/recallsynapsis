#!/bin/sh
# Fails on what must never reach git between a base and a head (see the make
# help). Usage: scan-commits.sh [base [head]]; base defaults to origin/main,
# head to HEAD. Word boundaries are spelled out: \b is a GNU extension to POSIX ERE.
set -u
export LC_ALL=C

base=${1:-origin/main}
head=${2:-HEAD}
b='(^|[^A-Za-z0-9])'
# Matched without case: prose writes "Task 3" and "task 3" alike.
loose="${b}(task|step|milestone)s? [0-9]"
# Session links never reach git, whatever a contributor's tools add.
session='claude-session|claude\.ai/code/session_'
# Nor do attribution lines; the optional prefix allows a leading emoji.
attribution='^co-authored-by:|^[^A-Za-z0-9]*generated with'
dash=$(printf '\342\200\224')

# Prove each pattern matches before an empty result is trusted.
for s in 'in Task 3' 'see step 2' 'steps 5 to 8' 'see milestone 2'; do
	printf '%s\n' "$s" | grep -Eiq "$loose" || { echo "scan-commits: grep cannot match the task, step or milestone pattern on '$s'"; exit 1; }
done
for s in 'Claude-Session: x' 'https://claude.ai/code/session_x'; do
	printf '%s\n' "$s" | grep -Eiq "$session" || { echo "scan-commits: grep cannot match the session pattern on '$s'"; exit 1; }
done
for s in 'Co-authored-by: x <x@y>' 'Generated with [Claude Code](https://claude.com/claude-code)' "$(printf '\360\237\244\226') Generated with [Claude Code](https://claude.com/claude-code)"; do
	printf '%s\n' "$s" | grep -Eiq "$attribution" || { echo "scan-commits: grep cannot match the attribution pattern on '$s'"; exit 1; }
done
! printf '%s\n' '- Mocks generated with mockery' | grep -Eiq "$attribution" || { echo "scan-commits: the attribution pattern matches a mockery line"; exit 1; }
printf 'a\342\200\224b\n' | grep -qF "$dash" || { echo "scan-commits: grep cannot match an em-dash"; exit 1; }

for r in "$base" "$head"; do
	git rev-parse --verify --quiet "$r^{commit}" >/dev/null || { echo "scan-commits: no commit $r"; exit 1; }
done

# Plain diffs whatever the user's config, so the added-line filter holds.
gdiff() { git diff --no-color --no-ext-diff --src-prefix=a/ --dst-prefix=b/ "$base...$head" -- "$@"; }
added() { grep '^+' | grep -Ev '^\+\+\+ (b/|/dev/null)'; }

msgs=$(git log --no-merges --format='%h %s%n%b' "$base..$head") || exit 1
# This script holds its own canaries, so the pattern checks leave it out.
codediff=$(gdiff ':(exclude)scripts/scan-commits.sh') || exit 1
mddiff=$(gdiff '*.md') || exit 1
alldiff=$(gdiff) || exit 1
objects=$(git rev-list --objects "$base..$head") || exit 1
sizes=$(printf '%s\n' "$objects" | git cat-file --batch-check='%(objecttype) %(objectsize) %(rest)') || exit 1

fail=0
report() {
	echo "scan-commits: $1:"
	printf '%s\n' "$2"
	fail=1
}

hits=$(printf '%s\n' "$msgs" | grep -Ein "$loose") && report "task, step or milestone numbers in commit messages" "$hits"
hits=$(printf '%s\n' "$msgs" | grep -Ein "$session") && report "session links in commit messages" "$hits"
hits=$(printf '%s\n' "$msgs" | grep -Ein "$attribution") && report "attribution lines in commit messages" "$hits"
hits=$(printf '%s\n' "$msgs" | grep -nF "$dash") && report "em-dashes in commit messages" "$hits"
hits=$(printf '%s\n' "$codediff" | added | grep -E '//|#' | grep -Ei "$loose") && report "task, step or milestone numbers in added comments" "$hits"
hits=$(printf '%s\n' "$mddiff" | added | grep -Ei "$loose") && report "task, step or milestone numbers in added Markdown lines" "$hits"
hits=$(printf '%s\n' "$codediff" | added | grep -Ei "$session") && report "session links in added lines" "$hits"
hits=$(printf '%s\n' "$alldiff" | added | grep -nF "$dash") && report "em-dashes in added lines" "$hits"
hits=$(printf '%s\n' "$sizes" | awk '$1 == "blob" && $2 > 500000' | grep .) && report "blobs over 500 KB" "$hits"

[ "$fail" = 0 ] && echo "scan-commits: $(git rev-list --count "$base..$head") commits clean"
exit "$fail"
