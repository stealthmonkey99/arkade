# rpi-rgb-led-matrix (vendored header)

- Upstream: <https://github.com/hzeller/rpi-rgb-led-matrix>
- Commit: `4e326c1b34bed36847711e5c1a421aecb879b9bb`, the last upstream
  change to the file
- Vendored: `include/led-matrix-c.h`, unmodified. It is the only header
  `internal/render/ledpanel/ledpanel.go` includes, and it includes only the C
  standard library.
- Licence: GPL-2.0-or-later. `COPYING` here is upstream's licence text,
  verbatim. The file's own boilerplate names only version 2; upstream's
  README settles the "or later":

  > The LED-matrix library is (c) Henner Zeller <h.zeller@acm.org>,
  > licensed with GNU General Public License Version 2.0 (or any later
  > version) […] Yes, you are also allowed to use this code under the GNU
  > General Public License Version 3.0 which allows linking with Apache
  > 2.0 code and LGPL-3.0 code.

The library itself is not vendored. `scripts/pi-setup.sh` clones and
builds it on the Pi, and the built `lib/` is gitignored.
