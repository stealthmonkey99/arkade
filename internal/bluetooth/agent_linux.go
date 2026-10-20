// SPDX-License-Identifier: GPL-2.0-or-later

//go:build linux

package bluetooth

import (
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

// agent is the pairing agent BlueZ requires. It registers as
// NoInputNoOutput so BlueZ picks Just Works, accepts confirmations, and
// rejects PIN and passkey requests.
type agent struct{}

const agentIntrospect = `
<node>
 <interface name="org.bluez.Agent1">
  <method name="Release"/>
  <method name="RequestPinCode"><arg type="o" direction="in"/><arg type="s" direction="out"/></method>
  <method name="DisplayPinCode"><arg type="o" direction="in"/><arg type="s" direction="in"/></method>
  <method name="RequestPasskey"><arg type="o" direction="in"/><arg type="u" direction="out"/></method>
  <method name="DisplayPasskey"><arg type="o" direction="in"/><arg type="u" direction="in"/><arg type="q" direction="in"/></method>
  <method name="RequestConfirmation"><arg type="o" direction="in"/><arg type="u" direction="in"/></method>
  <method name="RequestAuthorization"><arg type="o" direction="in"/></method>
  <method name="AuthorizeService"><arg type="o" direction="in"/><arg type="s" direction="in"/></method>
  <method name="Cancel"/>
 </interface>` + introspect.IntrospectDataString + `</node>`

var rejected = dbus.NewError("org.bluez.Error.Rejected", nil)

func (agent) Release() *dbus.Error { return nil }

func (agent) RequestPinCode(dbus.ObjectPath) (string, *dbus.Error) { return "", rejected }

func (agent) DisplayPinCode(dbus.ObjectPath, string) *dbus.Error { return rejected }

func (agent) RequestPasskey(dbus.ObjectPath) (uint32, *dbus.Error) { return 0, rejected }

func (agent) DisplayPasskey(dbus.ObjectPath, uint32, uint16) *dbus.Error { return nil }

func (agent) RequestConfirmation(dbus.ObjectPath, uint32) *dbus.Error { return nil }

func (agent) RequestAuthorization(dbus.ObjectPath) *dbus.Error { return nil }

func (agent) AuthorizeService(dbus.ObjectPath, string) *dbus.Error { return nil }

func (agent) Cancel() *dbus.Error { return nil }

// registerAgent exports the agent and makes it the default, which covers
// pads that reconnect on their own.
func (b *bluez) registerAgent() error {
	a := agent{}
	if err := b.conn.Export(a, agentPath, agentIface); err != nil {
		return err
	}
	if err := b.conn.Export(introspect.Introspectable(agentIntrospect), agentPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return err
	}
	mgr := b.conn.Object(bluezService, agentMgrPath)
	if err := callBounded(mgr, agentMgrIface+".RegisterAgent", agentPath, "NoInputNoOutput").Err; err != nil {
		// AlreadyExists is left by a previous connection that never
		// unregistered, and must not be fatal.
		if !strings.Contains(err.Error(), "AlreadyExists") {
			return err
		}
	}
	return callBounded(mgr, agentMgrIface+".RequestDefaultAgent", agentPath).Err
}
