#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
# Usage: license-check.sh [tree]
# Lists tracked source files under tree that lack the SPDX line in their
# first two lines; exits 1 if any.
#
# Out of scope: third_party/, vendor/, node_modules/, dist/, build/ and
# testdata/ directories.
set -eu
cd "${1:-.}"
missing=$(git ls-files '*.go' '*.ts' '*.js' '*.mjs' '*.sh' '*.c' '*.h' '*.py' |
	grep -v -E '(^|/)(third_party|vendor|node_modules|dist|build|testdata)/' |
	while IFS= read -r f; do
		head -n 2 "$f" | grep -q 'SPDX-License-Identifier: GPL-2.0-or-later' || echo "$f"
	done)
[ -z "$missing" ] && exit 0
echo "missing SPDX-License-Identifier: GPL-2.0-or-later:"
echo "$missing"
exit 1
