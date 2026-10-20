#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
# Usage: binary-check.sh [tree]
# Lists tracked files under tree that are native executables or libraries
# (ELF, Mach-O, PE); exits 1 if any. A stray `go build` output is the usual
# culprit, and .gitignore cannot name every one in advance.
set -eu
cd "${1:-.}"
found=$(git ls-files --eol -- . | grep '^i/-text' | cut -f 2- | while IFS= read -r f; do
	[ -f "$f" ] || continue
	case $(head -c 4 "$f" | od -An -tx1 | tr -d ' \n') in
	(7f454c46 | cffaedfe | cefaedfe | feedface | feedfacf | cafebabe | 4d5a*) echo "$f" ;;
	esac
done)
[ -z "$found" ] && exit 0
echo "tracked native binaries (git rm --cached them and add an ignore pattern):"
echo "$found"
exit 1
