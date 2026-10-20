# Standby and screensavers

An idle cabinet steps down in stages, and a scheduled screensaver can take
over the middle stage at set hours. The state machine is
`internal/engine/standby.go`; the default screensaver is `apps/attract/`.

## Standby

Three states: **ACTIVE** (an app in use) → **ATTRACT** (screensaver) →
**DARK** (panel off).

- **ACTIVE → ATTRACT** after `--attract-after` (default `5m`) with no input.
  The running app is suspended, not closed, and the attract screensaver
  loads: a silent high-score showcase over a dim ambient animation, with a
  slow button-light chase.
- **ATTRACT → DARK** after `--standby-after` (default `5m`) in attract with
  no *presence*, meaning no input **and** no motion from the PIR/microwave
  sensor. Lights and LCDs go off, one black frame is painted, then painting
  stops. Motion alone keeps attract running indefinitely.
- **DARK → ATTRACT** on motion, so walking into the room brings the
  screensaver back.
- **ATTRACT or DARK → ACTIVE** on any button press. The suspended app resumes
  where it stopped, or the shell loads if nothing was suspended. The waking
  press itself is swallowed, so it never acts on the app that comes back.

**Hard dark.** Pressing BACK (`q`, esc or JOY B) while attracting, or choosing
SLEEP in the shell, goes dark deliberately. A hard dark ignores motion, so a
person moving about the room does not wake the screen they just put to sleep;
only a press does. After `--hard-standby-decay` (default `10m`) with no
presence it decays to an ordinary dark that motion wakes again.

Setting `--attract-after 0` disables standby entirely, `--standby-after 0`
lets attract run forever, and `--hard-standby-decay 0` disables hard dark. The
test rig (`internal/testkit`) defaults all three to `0`, so golden tests never
start attracting mid-test.

## Scheduled screensavers

ATTRACT does not have to be the attract reel. A native app registered as
`engine.TypeScreensaver` takes it over while its own schedule window is open,
and holds the panel up while it does: the ATTRACT → DARK timeout can't take
the screen away from a screensaver that is up because the clock says so.
Windows are re-evaluated once a second, so one opening on the minute swaps in
within a second of the boundary. When several overlap, the highest `priority`
wins and ties break by slug.

Green (JOY A) pressed while one is showing opens *that app's* settings screen;
any other button wakes to the shell as usual. Red on that settings screen
saves and drops straight back to the screensaver, running on the settings
just written and pinned there, whatever the window now says, until the next
button press.

Each screensaver keeps its window in its own `data/apps/<name>/settings.json`,
under the `"schedule"` key the engine reads (the rest of the file belongs to
the app):

```json
{
  "schedule": {
    "enabled": true,
    "from": "06:00",
    "to": "09:00",
    "days": [false, true, true, true, true, true, false],
    "priority": 10
  }
}
```

`from`/`to` are 24-hour local time, inclusive of `from` and exclusive of `to`;
a `to` earlier than `from` crosses midnight and is checked against the day the
window *started* on. `days` is Sunday-first. The engine re-reads the file at
most once a minute. A screensaver with no settings file yet runs on the
`DefaultSchedule` its manifest ships.

## Morning mode (`apps/morning`)

A bedside clock for the hours around sunrise, reading top to bottom as now
above and later below: the time in large amber digits that dissolve over two
seconds as the minute turns, the date under it, a dotted rule, and a weather
row from Open-Meteo (no API key). The sky and the current temperature sit as
one centred group, with the day's low and high on their own centre line
beneath, in a cool and a warm ink. Lamps stay out, both LCDs stay blank and
dark, and the panel drops to the app's own `brightness`, below the operator's
minimum if asked, which a screensaver is allowed to do.

`data/apps/morning/settings.json`, written with the defaults on first run:

| key | default | meaning |
|---|---|---|
| `schedule` | 06:00–09:00 daily, priority 10 | the window above |
| `brightness` | `15` | panel percent while the face is up (1–100) |
| `units` | `"F"` | `"F"` or `"C"`, the unit both the fetch and the face use |
| `location` | `"auto"` | `"auto"` to look itself up, `"manual"` to use `lat`/`lon` |
| `lat` / `lon` | absent | the manual forecast location; absent in manual mode draws the clock with no weather row |

**Finding itself.** On `"auto"` the app asks
`https://arkade.build/api/whereami` where its public IP is and caches the
answer in `data/apps/morning/location.json` for 24 hours, so a cabinet has a
weather row with nothing configured. The lookup runs on a background
goroutine at load; until it lands, and forever if it never does, the clock
draws alone and the failure is logged once. A settings file that carries
`lat`/`lon` but no `location` key reads as `"manual"`, so a location someone
chose is never overridden.

The face shows no place name: every line is centred, and the widest leaves
eight pixels a side, so a truncated city would be worse than none. The
[dashboard](DASHBOARD.md) shows what the cabinet resolved.

**Before dawn.** The fetch also brings back the day's sun times. A clear or
partly-clouded sky draws a crescent instead of a sun while the sun is down
(the other five icons, cloud, rain, snow, fog and lightning, look the same
after dark). And the lower line becomes a rotation: twenty seconds of the
low/high, a two-second dissolve, twenty seconds of the coming sunrise, and
back. Once the sun is up the forecast keeps the line to itself. A cached
reading with no sun times reads as daytime.

**Settings screen.** Green opens a joystick-driven editor for everything but
the manual coordinates: up/down picks a row (ENABLED, FROM, TO, DAYS, BRIGHT,
UNITS, LOCATION), left/right changes it (times in 15-minute steps, brightness
in 5s, LOCATION between AUTO and MANUAL; on DAYS left/right walks the week and
green toggles the day under the cursor), and red saves and exits. Latitude
and longitude are numbers a joystick should not have to enter, so the
[dashboard](DASHBOARD.md) writes them instead.

**Refreshing.** The last good reading is cached in
`data/apps/morning/weather.json` and re-fetched every 10 minutes while the app
is loaded (and once on load if the cache is already older than that), always
on a background goroutine so the frame loop never waits on the network. Unless
there is no reading on screen at all, a refetch goes out only if a button or
the motion sensor reported somebody within the last 10 minutes, so a face left
up in an empty bedroom stops asking. A failed fetch keeps the cached numbers
on screen; past three hours old they carry a small `OLD` marker, and a
location that has never returned a reading shows dashes. No location at all
draws no weather row.

## Evening mode (`apps/evening`)

In the evening, if someone is about and the room is loud (a television,
voices), the cabinet shows a dimmed equalizer of what it can hear instead of
the attract reel, and holds it while the motion sensor keeps seeing someone:
sixteen columns driven by eight microphone bands, a small clock in the
corner, lamps out and LCDs blank.

**The columns.** Each band is the summed power of a whole slice of the
spectrum; the bands tile 60 Hz to 5.3 kHz between the geometric means of their
centres, so a voice or a knock, which spreads its energy rather than sitting
on one frequency, lifts the columns it belongs under. A column's height is
`10·log10(band / that band's quiet floor) / 30 dB`, clamped, so thirty
decibels over the floor fills it. The floor is per band: it follows the band
down immediately and drifts up one e-fold per ten minutes, which cancels the
microphone's own colouration, with a 1e-7 minimum so an empty band stays down
instead of amplifying hiss. Columns rise immediately and fall over 600 ms, and
a frame carries the band's peak over the window it covers, so a knock on the
cabinet is not smoothed away. `apps/evening/bars_test.go` holds the columns to
the shape of speech over room noise.

**The gate**, re-evaluated once a second while the cabinet is attracting:

- **In** when the window is open **and** motion arrived within the presence
  hold **and** the room has been loud for at least 60% of the last 20 s.
- **Stays** while the window is open and motion keeps arriving, whatever the
  room is doing, so a quiet scene or an ad break does not bounce the panel.
- **Out** when the presence hold expires with no motion. (The noise tracker
  has its own release, loud for under 20% of the last 3 minutes, which only
  matters if something else keeps the presence clock alive.)

"Loud" is measured against the room's own quiet: a fast level (a ~2 s average)
against a slow floor that follows the level down immediately and drifts up one
e-fold per ten minutes, so an evening of television never becomes the new
quiet. It counts as loud above `sensitivity` × that floor and above 0.0012
full-scale absolute: low 2.2×, medium 1.6×, high 1.3×. Against a recording of a
room with a TV on (`apps/evening/testdata/tv-rms.csv`), medium triggers in
about twenty seconds.

`data/apps/evening/settings.json`, all editable from the settings screen
(green on the showing screensaver): `schedule` (default enabled, 19:00–23:30,
every day, priority 20), `brightness` (panel dim percent, default 40, 5–100 in
5s), `sensitivity` (`low`/`medium`/`high`, default medium) and
`presenceHoldMinutes` (default 5, 1–30). On that screen up/down picks a row,
left/right changes the value, green toggles the highlighted day on the DAYS
row, and red saves and leaves.
