// SPDX-License-Identifier: GPL-2.0-or-later

package docs

import (
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/tools/sdkdoc/gen"
)

// TestSDKDocCommentsConform is the guard behind the SDK's comment rule:
// every exported symbol is documented, each doc begins with its symbol's
// name, and every package has a package doc (AUTHORING.md, "Doc comments").
func TestSDKDocCommentsConform(t *testing.T) {
	problems, err := gen.Lint("..")
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Errorf("%d doc comment(s) break the SDK conventions:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// TestAPIReferenceIsCurrent fails when api/*.md or abi.md's generated
// tables no longer match the sources: the same check as `sdkdoc -check`.
func TestAPIReferenceIsCurrent(t *testing.T) {
	if err := gen.Check(".."); err != nil {
		t.Error(err)
	}
}
