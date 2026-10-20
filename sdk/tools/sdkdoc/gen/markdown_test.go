// SPDX-License-Identifier: GPL-2.0-or-later

package gen

import (
	"go/doc/comment"
	"testing"
)

// A toolchain whose go/doc knows nothing of struct fields must still see
// [Host.Channels] as a link, or the pages differ by the Go that made them.
func TestMemberLinksResolveWithoutToolchainHelp(t *testing.T) {
	none := func(recv, name string) bool { return false }
	syms := map[string]string{"Host": "p.md#type-host", "Host.Channels": "p.md#type-host"}
	p := &comment.Parser{LookupSym: memberLookup(none, syms)}

	d := p.Parse("InputBind reads [Host.Channels] but not [Host.Missing] or [Channels].")
	var links []string
	for _, x := range d.Content[0].(*comment.Paragraph).Text {
		if l, ok := x.(*comment.DocLink); ok {
			links = append(links, l.Recv+"."+l.Name)
		}
	}
	if len(links) != 1 || links[0] != "Host.Channels" {
		t.Fatalf("doc links %q, want only Host.Channels", links)
	}
}
