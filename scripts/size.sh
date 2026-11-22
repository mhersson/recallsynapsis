#!/bin/sh
# Prints the number of non-test Go lines. It never fails on a count.
set -u

lines=$(git ls-files -z --cached --others --exclude-standard -- '*.go' ':(exclude)*_test.go' | xargs -0 cat /dev/null | wc -l | tr -d ' ')
echo "size: $lines non-test Go lines"
