# Security Policy

## Reporting a vulnerability

Please report vulnerabilities **privately** — not in a public issue.

- **Preferred**: GitHub's private vulnerability reporting — the
  "Report a vulnerability" button under this repo's **Security** tab.
- **Or email**: `security@arkade.build`.

Include what you can: affected component, repro steps, impact as you
understand it. A seed + input log or a minimal cartridge that triggers
the issue is gold.

**What to expect**: an acknowledgment within about a week. This is a
one-person hobby project — fixes land on best effort, not an SLA.
Please allow a reasonable window for a fix before disclosing publicly;
coordinated disclosure with credit is happily given.

## Scope

- **Engine / SDK** (the `arkade` repo): the wasm cartridge sandbox is the
  interesting boundary — a cartridge escaping wazero or the host ABI is
  exactly the kind of report we want.
- **Web** (`arkade.build`): the player, the Cloudflare Worker, and
  (later) the leaderboard — especially anything that would let a forged
  replay or input log claim a score.
- **Cabinet** (Pi build): the LAN dashboard and remote-install server
  (`internal/net`) assume a trusted home LAN *by design* — reports that
  break that assumption in surprising ways (say, reachable from the
  WAN, or a malicious rcart pivoting to the Pi) are still in scope.

Out of scope: denial of service against a hobbyist's living-room
Raspberry Pi, and issues in upstream dependencies (report those
upstream — a heads-up here is still appreciated).

## No bounty

There is no bug bounty and no payment for reports — just genuine
thanks, credit in the fix, and the warm glow of protecting a 64×64 LED
arcade cabinet. It's a good glow.
