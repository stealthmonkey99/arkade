// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge

func SetFreeSpaceFunc(c *RemoteControl, f func(path string) (uint64, error)) { c.freeSpace = f }

var StatfsFree = statfsFree
