# The cabinet dashboard

The cabinet runs a small HTTP service (`internal/net`) beside the game loop:
live status, the log tail, and a **game table** — every registered game with
its icon, a button to launch it, a button to show its attract screen right
now, an editor for its saved high scores, and (where the cabinet holds more
than one build of a game) a switch between them.

It is **loopback-only** by default, so getting to it is one SSH tunnel.

---

## Getting there

With `$ARKADE_HOST` set to your cabinet's SSH host:

```sh
ssh -N -L 8080:127.0.0.1:8080 "$ARKADE_HOST"
```

Leave that open (Ctrl-C tears it down; `-L 9090:127.0.0.1:8080` moves the
local end), and browse to:

```
http://localhost:8080
```

Two things that bite every time:

- It **must** be `localhost` (or `127.0.0.1`). `http://<cabinet>.local:8080`
  gets a **421 Misdirected Request** — the server only answers to `localhost`
  and literal IPs, which is what stops a hostile web page from DNS-rebinding
  its way onto the cabinet.
- The tunnel is not optional. The listener is bound to `127.0.0.1:8080` on
  purpose, because the read routes have no authentication.
  Don't "fix" access by exposing the port.

## Making the buttons work: the control token

Reading the dashboard needs nothing. **Launching, previewing attract, and
editing scores need a shared secret**, the same one that gates remote
cartridge installs: the `ARKADE_HTTP_TOKEN` environment variable. With no
token set, those routes fail closed — every request gets a `503`, whatever
header it sends.

Read the one the cabinet is running with:

```sh
ssh "$ARKADE_HOST" 'systemctl show arkade -p Environment'
```

If it isn't set yet, generate one, drop it into a systemd override, and
restart (this prints the new token at the end):

```sh
ssh "$ARKADE_HOST" 'mkdir -p /etc/systemd/system/arkade.service.d &&
printf "[Service]\nEnvironment=ARKADE_HTTP_TOKEN=%s\n" "$(openssl rand -hex 24)" \
  > /etc/systemd/system/arkade.service.d/token.conf &&
systemctl daemon-reload && systemctl restart arkade &&
systemctl show arkade -p Environment'
```

A drop-in rather than an edit to `scripts/arkade.service` keeps the secret
off the repo and out of any deploy that overwrites the unit file.

The token must be at least 32 characters and not trivially repetitive; a
weaker one is refused at startup with an error in the journal and the log
ring, and the control routes stay disabled (`503`) until it is replaced. A
source that sends 10 wrong tokens within a minute is locked out of every
token-gated route for a minute (`429` with `Retry-After`).

Paste the token into the **Control token** box at the top of the dashboard
and press Save. It is kept in that browser's `localStorage`, so it survives
reloads; **Forget** clears it. (The page is loopback-only — anything that can
read that value can already reach the API from the same origin.)

---

## What the table does

Each row is one registered app, in the shell menu's own order.

### Launch

Switches the cabinet to that app.

**It works from any state.** Launch tears down whatever is running or parked
behind the screensaver and wakes the cabinet, even from a dark panel. The
button is greyed only when no control token is saved.

### Attract

Shows **that one game's attract screen immediately**, instead of waiting for
the screensaver rotation to reach it. Like Launch this works from any state
— mid-game, mid-screensaver, or from a dark panel — but it needs standby
enabled (`--attract-after` non-zero); with standby disabled the route answers
`409`.

Whatever was running is *suspended*, not killed: any button press at the
cabinet wakes it straight back to where it was, exactly like a normal
screensaver takeover. The preview is one-shot; the next attract that starts
on its own rotates through everything as usual.

The badge in the ATTRACT column says what a preview would actually show:

| Badge | Meaning |
|---|---|
| `CUSTOM` | The cartridge exports `cart_attract` and draws its own attract screen. |
| `SHOWCASE` | No `cart_attract`, but the game has a recorded score, so the engine's default slot (logo, trophy, leaderboard crawl) is shown. |
| `NONE` | Nothing to show — no `cart_attract` export *and* no recorded score. The button is disabled. |

`cart_attract` is an optional export (`docs/CARTRIDGE.md`), so `NONE`
is a normal state for a game, not a fault. Seeding a score with the editor
below turns a `NONE` game into `SHOWCASE`.

### Build: alternate builds

**Only appears when the cabinet actually holds two builds of a game.** On a
cabinet with none the whole column is hidden and no game grows a control it
has nothing to switch between.

Two builds of one cartridge can share a slug. The registry is slug-keyed and
`data/cartridges/` holds one file per slug, so only one of them can be
registered at a time; this column chooses which.

Pressing a build's button copies that stored build over
`data/cartridges/<slug>.rcart` (atomically: staged beside it and renamed, so
the watcher never reads a half-written bundle) and re-registers it. The lit
button is the build installed right now, and it is disabled: re-installing
what is already there would do nothing.

> **It takes effect on the NEXT launch.** Swapping the file re-registers the
> game, and re-registering only changes what a *new* launch instantiates; a
> session already running keeps the build it started with, exactly as with any
> cartridge deploy. If the game is on screen, back out to the menu and launch
> it again (or press **Launch** in the same row).

A `?` beside the buttons means the installed bundle matches **none** of the
stored builds, because something else was copied into `data/cartridges/`.
Press either button to get back to a known state.

**Scores are shared between the builds**: the slug is what score tables,
favourites and MRU are keyed by. The swap cannot lose them: the stored bundle
is re-parsed before anything is written and refused unless its manifest slug
matches the file it is about to become. Nothing in this path reads or writes
`data/cart/<slug>/` at all.

### Scores

Opens an editor on that game's saved top-ten table: one entry per line,
`<score> <name>`, or just `<score>` for an anonymous entry. **Save table**
replaces the whole table; **Clear table** empties it (handy for checking a
game's first-run screen).

The server validates before anything touches the disk, and refuses:

- a slug that isn't a currently-registered **game** (the write can only ever
  land on that game's own `scores` key under `data/` — the request cannot
  name a storage label or a path),
- a body it doesn't fully understand: an unknown/misspelled field, or a
  missing `entries` key (clearing has to be the explicit `{"entries":[]}`, so
  a typo can never read as "erase this table"),
- more than 10 entries, a negative score, or a score above 2147483647,
- a name longer than 12 characters or outside the cabinet's own alphabet
  (A–Z, 0–9, space) — anything else would reach the LCDs and the attract
  ticker as garbage.

Names are uppercased and trimmed, and entries are re-sorted descending, so
what lands on disk always matches what the games themselves would write. The
response echoes the normalized table.

One caveat the editor warns about: **editing the table of the game that is
running right now** can be overwritten when that game exits and saves its
own. Edit it while something else is on screen.

---

## Screensavers

Scheduled screensavers (`engine.TypeScreensaver` — morning mode is the first)
keep their schedule in their own settings file and edit it at the cabinet:
on the screen the green button opens while one is showing, or from
SETTINGS → SCREENSAVERS, which opens the same editor over the same file.
Either save takes effect at the next pick, without a restart.

Latitude and longitude are the one exception. A joystick is a poor way to
enter numbers, so they are set from here instead. They are optional and unset
by default.

### Show now

Puts the selected screensaver on the cabinet **immediately, whatever its
schedule says** — how you look at morning mode tonight instead of waiting for
sunrise. It works from any state, mid-game or from a dark panel, and the pick
is *pinned*: the once-a-second re-evaluation that normally swaps screensavers
at their window boundaries leaves it alone.

Whatever was running is *suspended*, not killed. Any button press at the
cabinet wakes it straight back to where it was, exactly like a normal
screensaver takeover — and that wake also drops the pin, so the next attract
that starts on its own follows the schedule again. So does a BACK press (hard
dark), and so does the screensaver exiting by itself.

The button needs standby to be enabled (`--attract-after` non-zero); on a
cabinet without it there is no attract state to enter, and the route says so
with a 409.

### Location

A screensaver is either **automatic** — it asks `arkade.build/api/whereami`
where its public IP is and caches the answer for a day — or **manual**, using
the two coordinates set here. Automatic is the default, so a fresh cabinet
has a weather row without anyone touching this form; the mode dropdown is the
switch, and it writes as soon as you change it.

In automatic mode the bar shows the fix the screensaver last resolved (city,
coordinates, source and when) read-only, and the lat/lon fields are greyed
out. Nothing here forces a fresh lookup: the screensaver resolves on load and
trusts the cached fix for 24 hours, so "Show now" after a day is the way to
make it look again.

In manual mode the fields are editable and **Save** writes them. Everything
else in `data/<slug>/settings.json` — the `schedule` key the engine reads,
and the app's own preferences — is left exactly as it was; the write patches
keys rather than replacing the file.

**Clear** empties both coordinates *and* stays on manual, which is how you
turn the weather row off and keep it off. Leaving it automatic would just
resolve a location again on the next load.

The four bodies the write route takes:

```
{"location":"auto"}                           find yourself; stored coordinates untouched
{"location":"manual","lat":37.77,"lon":-122.42}  use these
{"location":"manual","lat":null,"lon":null}   manual with nothing set: no weather row
{"lat":37.77,"lon":-122.42}                   the original shape, which means manual
```

A settings file written before the mode existed reads as **manual** when it
already carries `lat`/`lon`, and automatic otherwise — an upgrade never moves
a cabinet whose location someone chose.

The server refuses a slug that isn't a currently-registered screensaver, a
mode that isn't `auto` or `manual`, a body carrying neither key, half a
location (one of the two, or one null against one number), and a latitude
outside ±90 or a longitude outside ±180.

```
GET  /api/screensavers                 → [{"slug":"apps/morning","name":"Morning","location":"auto",
                                            "fix":{"city":"San Francisco","lat":37.77,"lon":-122.42,
                                                   "source":"cloudflare-ip","at":"<RFC 3339 time>"}}]
PUT  /api/screensavers/{slug...}       ← {"location":"manual","lat":37.77,"lon":-122.42}
POST /api/screensavers/{slug...}/show  → {"slug":"apps/morning","name":"Morning"}
```

`lat`/`lon` in the listing are the *stored* manual coordinates; `fix` is the
last location the screensaver resolved for itself, absent until it manages
one.

---

## Known behaviour

- **The server listens on loopback only.** You reach it through an SSH
  tunnel, so the control token never crosses the
  LAN in the clear. There is no TLS. If you ever bind it to a LAN address
  with `--http`, put a TLS-terminating proxy in front of it first: anyone who
  can watch that traffic can capture the token and install code.
- **The config app's Wi-Fi QR code contains your network passphrase.** It
  deliberately encodes `WIFI_SSID` and `WIFI_PASS` from `.env`, and anyone
  who can see the panel, or a photo of it, can read it. Point it at a guest
  network, not your main one.
- **A USB stick is mounted and its cartridge launched automatically.** That
  is the point of the cartridge slot. A cartridge runs inside the wasm
  sandbox with no file or network access beyond its own storage. To switch
  it off on a cabinet left unattended in public, drop `/media/*` from
  `--cartridges` in `scripts/arkade.service`.
- **Installs need free space, not a quota.** There is no limit on how many
  cartridges you can install. An install is refused (`507`) when the bundle
  plus 32 MiB of headroom would not fit on the filesystem holding
  `data/cartridges`, and a warning is logged when an install leaves less
  than 256 MiB free.
- **Install errors are generic.** A rejected upload returns `invalid
  cartridge bundle`; the full reason is in the log ring.

---

## Routes

| Route | Auth | Notes |
|---|---|---|
| `GET /` | none | The dashboard page |
| `GET /api/status` | none | Engine snapshot, uptime, version, temp, motion |
| `GET /api/logs?n=` | none | Last *n* ring entries |
| `GET /api/scores` | none | Every game's raw stored scores |
| `GET /api/scores/{app...}` | none | One game's raw stored scores |
| `GET /api/games` | none | The game table: metadata, inline icons, attract capability, parsed score tables |
| `GET /api/screensavers` | none | Every registered screensaver: its location mode, stored coordinates and last resolved fix |
| `PUT /api/scores/{app...}` | **bearer** | Replace one game's score table |
| `POST /api/cartridges` | **bearer** | Install an uploaded `.rcart` bundle |
| `POST /api/launch/{slug...}` | **bearer** | Switch the running app |
| `POST /api/attract/{slug...}` | **bearer** | Show one app's attract screen now |
| `POST /api/variants/{variant}/{slug...}` | **bearer** | Install a different stored build of one game |
| `PUT`/`POST /api/screensavers/{slug...}` | **bearer** | Set one screensaver's location mode, and the coordinates its manual mode uses |
| `POST /api/screensavers/{slug...}/show` | **bearer** | Show one screensaver now, ignoring its schedule |
| `POST /api/preview/frame` | **bearer** | Show a frame (or short animation) on the LED matrix |
| `POST /api/preview/sound` | **bearer** | Play an uploaded WAV through the speakers |
| `POST /api/preview/lcd` | **bearer** | Put lines + custom glyphs on the character LCDs |
| `DELETE /api/preview` | **bearer** | End the current preview now |
| `POST /api/flight/dump` | **bearer** | Write the running cartridge's input flight recording to disk |
| `GET /api/periph/stream` | **bearer** | Server-Sent Events observation stream: input, lights, motion and, unless `?mic=0`, microphone events. An input record names its `seat` (`"P1"`, `"P2"`) and zero-based attach `slot`, each `null` for a source without one |
| `POST /api/periph/lights` | **bearer** | Play a bounded button-lamp choreography |
| `DELETE /api/periph/lights` | **bearer** | End the choreography now (succeeds with nothing playing) |
| `GET /api/mic/status` | none | Whether the cabinet has a microphone, and the capture pipeline's state |
| `GET /api/mic/stream` | **bearer** | The microphone frame stream as a chunked binary body |

The `/api/preview` family is the on-hardware asset preview: push a sprite, a
sound or a 5×8 glyph at the real peripherals for a few seconds and look at
it. It overlays whatever is running and always times out back on its own.

Every mutating route and both streams are behind the token. A request
body over a route's cap is answered `413` with `request body too large (max N
bytes)`, and the rejection is logged.

Every mutating route is audited to the log ring, accepted or rejected.
Repeated token and Host rejections of the same kind are folded into one line
every 30 seconds, with a count, so a stream of unauthenticated requests cannot
push older entries out of the ring.

The reads are deliberately left unauthenticated, as they always were: the
loopback listener is the real boundary, and `GET /api/games` publishes
nothing that `GET /api/scores` and `GET /api/logs` didn't already.

### From the command line

```sh
# with the tunnel open, from your laptop
TOKEN=$(ssh "$ARKADE_HOST" 'systemctl show arkade -p Environment' | sed 's/.*ARKADE_HTTP_TOKEN=//')

curl -s localhost:8080/api/games | jq '.games[] | {slug, attract}'
curl -s -X POST -H "Authorization: Bearer $TOKEN" localhost:8080/api/attract/cart/snake
curl -s -X PUT  -H "Authorization: Bearer $TOKEN" \
  -d '{"entries":[{"score":9001,"name":"ADA"}]}' localhost:8080/api/scores/cart/snake

# which games have two builds, and which one is live
curl -s localhost:8080/api/games \
  | jq '.games[] | select(.variants) | {slug, variants, activeVariant}'

# which screensavers are installed, and where each one thinks it is
curl -s localhost:8080/api/screensavers
# tell morning mode exactly where it is
curl -s -X PUT -H "Authorization: Bearer $TOKEN" \
  -d '{"location":"manual","lat":37.77,"lon":-122.42}' \
  localhost:8080/api/screensavers/apps/morning
# hand it back to the IP lookup
curl -s -X PUT -H "Authorization: Bearer $TOKEN" \
  -d '{"location":"auto"}' localhost:8080/api/screensavers/apps/morning
# no location at all, and no weather row
curl -s -X PUT -H "Authorization: Bearer $TOKEN" \
  -d '{"location":"manual","lat":null,"lon":null}' \
  localhost:8080/api/screensavers/apps/morning
# what the automatic lookup last resolved
curl -s localhost:8080/api/screensavers | jq '.[] | {slug, location, fix}'
# see morning mode right now, whatever the clock says
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  localhost:8080/api/screensavers/apps/morning/show
```

---

## Alternate builds on the cabinet

The toggle only chooses between bundles that are already on the Pi. They live
in a store beside the live library:

```
/path/to/arkade/data/cartridges/<slug>.rcart            the LIVE one: the engine scans this
/path/to/arkade/data/variants/<variant>/<slug>.rcart    the stored builds
```

`data/variants/` is **not** scanned by the engine, so a stored build sits
inert until the dashboard installs it. Nothing else in `data/` is touched; in
particular not `data/cart/<slug>/`, which is save data.

Ship the stored builds with rsync, with `<variant>` directories laid out as
above:

```sh
rsync -a --delete stage/variants/ "$ARKADE_HOST":/path/to/arkade/data/variants/
```

No service restart is needed, for the copy or for the swap. The variant store
is read on demand by `GET /api/games`, and installing a build goes through the
same hot-mount registration path a cartridge deploy does.

A cabinet without a `data/variants/` directory has the whole feature
disabled: no column, no route that succeeds. That is the default.

---

## Deploying a change to any of this

The dashboard is **host code**, not a cartridge — a hot cartridge swap will
not pick it up. It needs the full cabinet deploy: rsync the tree, `make
build-pi` on the Pi, `systemctl restart arkade`.
