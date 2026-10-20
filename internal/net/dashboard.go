// SPDX-License-Identifier: GPL-2.0-or-later

package net

import "net/http"

// handleDashboard serves GET /, the static dashboard shell.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) *rejection {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
	return nil
}

// dashboardHTML is the whole dashboard, with no external assets.
//
//   - Network-sourced values are set with textContent or a server-generated
//     <img src>, never concatenated into innerHTML.
//   - It is a raw Go string, so the JS cannot contain a backtick.
const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Årkade</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body {
    margin: 0; padding: 24px;
    background: #111417; color: #e6e6e6;
    font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  }
  h1 { font-size: 18px; margin: 0 0 4px; letter-spacing: .02em; }
  .sub { color: #8a8f98; margin-bottom: 20px; font-size: 12px; }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 12px;
    margin-bottom: 20px;
  }
  .tile {
    background: #191d22; border: 1px solid #2a2f36; border-radius: 8px;
    padding: 12px 14px;
  }
  .tile .label { color: #8a8f98; font-size: 11px; text-transform: uppercase; letter-spacing: .05em; }
  .tile .value { font-size: 20px; margin-top: 4px; font-variant-numeric: tabular-nums; }
  .value.dim { color: #6c7280; font-size: 15px; }
  .state-active { color: #5fd97a; }
  .state-attract { color: #f2c94c; }
  .state-dark { color: #808792; }
  .on { color: #5fd97a; }
  .off { color: #6c7280; }
  h2 { font-size: 13px; text-transform: uppercase; letter-spacing: .05em; color: #8a8f98; margin: 24px 0 8px; }
  #logs {
    background: #0b0d0f; border: 1px solid #2a2f36; border-radius: 8px;
    padding: 10px 12px; height: 320px; overflow-y: auto;
    font: 12px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;
    white-space: pre-wrap;
  }
  .lvl-error { color: #f27979; }
  .lvl-warn { color: #f2c94c; }
  .lvl-info { color: #9db4c0; }
  #err { color: #f27979; font-size: 12px; margin-top: 8px; display: none; }

  /* --- access help + token --- */
  details.access {
    background: #191d22; border: 1px solid #2a2f36; border-radius: 8px;
    padding: 10px 14px; margin-bottom: 16px; font-size: 13px;
  }
  details.access summary { cursor: pointer; color: #9db4c0; }
  details.access p { color: #8a8f98; margin: 10px 0 6px; }
  pre.cmd {
    background: #0b0d0f; border: 1px solid #2a2f36; border-radius: 6px;
    padding: 8px 10px; margin: 0 0 8px; overflow-x: auto;
    font: 12px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace; color: #cfe3ee;
    user-select: all;
  }
  .tokenbar { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin-bottom: 16px; font-size: 12px; }
  .tokenbar input {
    background: #0b0d0f; border: 1px solid #2a2f36; border-radius: 6px; color: #e6e6e6;
    padding: 6px 8px; font: 12px ui-monospace, SFMono-Regular, Menlo, monospace; min-width: 260px;
  }
  #tokenState { color: #8a8f98; }
  #tokenState.ok { color: #5fd97a; }
  #tokenState.bad { color: #f2c94c; }

  /* --- game table --- */
  button {
    background: #232931; border: 1px solid #39414b; border-radius: 6px; color: #e6e6e6;
    padding: 5px 10px; font-size: 12px; cursor: pointer;
  }
  button:hover:not(:disabled) { background: #2c343e; }
  button:disabled { opacity: .38; cursor: not-allowed; }
  button.primary { border-color: #3d6b46; }
  table.games { width: 100%; border-collapse: collapse; font-size: 13px; }
  table.games th {
    text-align: left; color: #8a8f98; font-size: 11px; text-transform: uppercase;
    letter-spacing: .05em; font-weight: normal; padding: 4px 8px; border-bottom: 1px solid #2a2f36;
  }
  table.games td { padding: 8px; border-bottom: 1px solid #1d2228; vertical-align: middle; }
  table.games tr.running td { background: #16201a; }
  td.icon { width: 40px; }
  td.icon img {
    width: 32px; height: 32px; image-rendering: pixelated;
    background: #0b0d0f; border-radius: 4px;
  }
  .gname { font-weight: 600; }
  .gslug { color: #6c7280; font-size: 11px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  .badge {
    display: inline-block; font-size: 10px; letter-spacing: .05em; text-transform: uppercase;
    padding: 2px 6px; border-radius: 999px; border: 1px solid #39414b; color: #8a8f98;
  }
  .badge.custom { border-color: #3d6b46; color: #5fd97a; }
  .badge.showcase { border-color: #6b5a2a; color: #f2c94c; }
  /* Build-variant toggle. The whole column collapses on a cabinet with no
     variant store — which is every cabinet built from the public repo —
     so no game sprouts a control it has nothing to switch between. */
  table.games:not(.has-variants) .buildcol { display: none; }
  .variants { display: inline-flex; }
  .variants button {
    border-radius: 0; margin: 0 0 0 -1px; font-size: 11px; padding: 4px 8px;
    letter-spacing: .04em; text-transform: uppercase;
  }
  .variants button:first-child { border-radius: 6px 0 0 6px; margin-left: 0; }
  .variants button:last-child { border-radius: 0 6px 6px 0; }
  .variants button.active {
    background: #1b2a20; border-color: #3d6b46; color: #5fd97a; opacity: 1;
  }
  .variants .unknown { color: #f2c94c; font-size: 11px; margin-left: 6px; }
  .scorecell { font-variant-numeric: tabular-nums; color: #9db4c0; font-size: 12px; }
  .scorecell .none { color: #6c7280; }
  td.actions { text-align: right; white-space: nowrap; }
  tr.editor td { background: #0f1216; }
  .editor textarea {
    width: 100%; min-height: 130px; background: #0b0d0f; border: 1px solid #2a2f36;
    border-radius: 6px; color: #e6e6e6; padding: 8px;
    font: 12px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .editor .hint { color: #6c7280; font-size: 11px; margin: 6px 0; }
  .editor .warn { color: #f2c94c; font-size: 12px; margin: 6px 0; }
  .editor .row { display: flex; gap: 8px; align-items: center; margin-top: 6px; }
  .ssbar { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin-bottom: 16px; font-size: 12px; }
  .ssbar input { width: 100px; }
  .ssbar input:disabled { color: #6a737d; }
  .ssfix { color: #8b949e; }
  #toast {
    position: fixed; right: 20px; bottom: 20px; max-width: 460px;
    background: #191d22; border: 1px solid #39414b; border-radius: 8px;
    padding: 10px 14px; font-size: 13px; display: none; white-space: pre-wrap;
  }
  #toast.ok { border-color: #3d6b46; }
  #toast.bad { border-color: #7a3b3b; }
</style>
</head>
<body>
  <h1>Årkade</h1>
  <div class="sub" id="sub">connecting…</div>

  <details class="access">
    <summary>How to reach this page (and how to make the buttons work)</summary>
    <p>The cabinet serves this on loopback only, so it is reached through an SSH tunnel.
       On your laptop:</p>
    <pre class="cmd">ssh -L 8080:127.0.0.1:8080 $ARKADE_HOST</pre>
    <p>Then open <strong>http://localhost:8080</strong> — it must be <code>localhost</code>,
       not <code>cabinet.local</code>: any other host name is rejected (421) as a
       DNS-rebinding guard.</p>
    <p>The launch / attract / score-edit buttons need the shared secret from
       <code>ARKADE_HTTP_TOKEN</code>. To read the one the cabinet is running with:</p>
    <pre class="cmd">ssh $ARKADE_HOST 'systemctl show arkade -p Environment'</pre>
    <p>If it is not set yet, generate one and restart the service:</p>
    <pre class="cmd">ssh $ARKADE_HOST 'mkdir -p /etc/systemd/system/arkade.service.d &amp;&amp;
printf "[Service]\nEnvironment=ARKADE_HTTP_TOKEN=%s\n" "$(openssl rand -hex 24)" \
  &gt; /etc/systemd/system/arkade.service.d/token.conf &amp;&amp;
systemctl daemon-reload &amp;&amp; systemctl restart arkade &amp;&amp;
systemctl show arkade -p Environment'</pre>
    <p>Full notes: <code>docs/DASHBOARD.md</code> in the repo.</p>
  </details>

  <div class="tokenbar">
    <label for="token">Control token</label>
    <input id="token" type="password" autocomplete="off" spellcheck="false" placeholder="ARKADE_HTTP_TOKEN">
    <button id="tokenSave">Save</button>
    <button id="tokenClear">Forget</button>
    <span id="tokenState"></span>
  </div>

  <div class="grid" id="grid"></div>

  <h2>Games <button id="gamesReload" style="margin-left:8px">Reload</button></h2>
  <table class="games" id="gamesTable">
    <thead>
      <tr>
        <th></th><th>Game</th><th>Attract</th><th class="buildcol">Build</th><th>Top scores</th><th></th>
      </tr>
    </thead>
    <tbody id="games"><tr><td colspan="6" class="gslug">loading…</td></tr></tbody>
  </table>

  <h2>Screensavers</h2>
  <div class="ssbar">
    <label for="ssSelect">Screensaver</label>
    <select id="ssSelect"></select>
    <button id="ssShow">Show now</button>
    <label for="ssMode">Location</label>
    <select id="ssMode">
      <option value="auto">Automatic</option>
      <option value="manual">Manual</option>
    </select>
    <label for="ssLat">Lat</label>
    <input id="ssLat" type="text" inputmode="decimal" placeholder="37.77">
    <label for="ssLon">Lon</label>
    <input id="ssLon" type="text" inputmode="decimal" placeholder="-122.42">
    <button id="ssSave">Save</button>
    <button id="ssClear">Clear</button>
    <span id="ssFix" class="ssfix"></span>
  </div>

  <h2>Log tail</h2>
  <div id="logs"></div>
  <div id="err">connection lost — retrying…</div>
  <div id="toast"></div>

<script>
// The token lives in localStorage; anything that can read it can already call the API from this origin.
var TOKEN_KEY = 'arkade.http.token';
var controlEnabled = null; // null until /api/games says
function token() { return localStorage.getItem(TOKEN_KEY) || ''; }
function haveToken() { return token() !== ''; }

function paintTokenState() {
  var el = document.getElementById('tokenState');
  el.className = '';
  if (controlEnabled === false) {
    el.className = 'bad';
    el.textContent = 'the cabinet has no ARKADE_HTTP_TOKEN set — control routes are disabled (503)';
  } else if (haveToken()) {
    el.className = 'ok';
    el.textContent = 'saved in this browser';
  } else {
    el.textContent = 'needed for launch / attract / score edits';
  }
}

document.getElementById('tokenSave').onclick = function () {
  var v = document.getElementById('token').value.trim();
  if (v) { localStorage.setItem(TOKEN_KEY, v); toast('Token saved.', true); }
  document.getElementById('token').value = '';
  paintTokenState(); applyReadiness();
};
document.getElementById('tokenClear').onclick = function () {
  localStorage.removeItem(TOKEN_KEY);
  toast('Token forgotten.', true);
  paintTokenState(); applyReadiness();
};

var toastTimer = null;
function toast(msg, ok) {
  var t = document.getElementById('toast');
  t.textContent = msg;
  t.className = ok ? 'ok' : 'bad';
  t.style.display = 'block';
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(function () { t.style.display = 'none'; }, 6000);
}

// send issues a mutating request with the bearer token and shows the server's error text verbatim.
function send(method, path, body) {
  var opts = { method: method, headers: { 'Authorization': 'Bearer ' + token() } };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  return fetch(path, opts).then(function (r) {
    return r.text().then(function (text) {
      if (!r.ok) throw new Error(text.trim() || (r.status + ' ' + r.statusText));
      return text ? JSON.parse(text) : null;
    });
  });
}

function fmtMs(ms) {
  if (ms == null) return "—";
  const s = ms / 1000;
  if (s < 60) return s.toFixed(1) + "s";
  const m = Math.floor(s / 60), r = Math.floor(s % 60);
  return m + "m" + r + "s";
}
function el(tag, cls, text) {
  var e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  return e;
}
function tile(label, value, cls) {
  var t = el('div', 'tile');
  t.append(el('div', 'label', label), el('div', 'value' + (cls ? ' ' + cls : ''), value));
  return t;
}

var lastStatus = {};

function scoreLines(entries) {
  return entries.map(function (e) {
    return e.name ? (e.score + ' ' + e.name) : ('' + e.score);
  }).join('\n');
}

// variantCell renders the BUILD column: a segmented button per stored build, installed one lit.
// Games with fewer than two builds get an empty cell; renderGames hides the column if none have any.
function variantCell(g) {
  var td = el('td', 'buildcol');
  if (!g.variants || g.variants.length < 2) return td;

  var group = el('div', 'variants');
  g.variants.forEach(function (v) {
    var b = el('button', 'variant' + (v === g.activeVariant ? ' active' : ''), v);
    b.dataset.variant = v;
    b.dataset.installed = (v === g.activeVariant) ? '1' : '';
    b.onclick = function () {
      send('POST', '/api/variants/' + v + '/' + g.slug).then(function (res) {
        toast(g.slug + ' now runs the ' + res.variant + ' build ("' + res.name + '").\n' +
              'Anything already running keeps the OLD build — go back to the cabinet menu ' +
              'and launch it again (or press Launch here) to see the change.', true);
        loadGames();
      }).catch(function (e) { toast('Build swap failed: ' + e.message, false); });
    };
    group.appendChild(b);
  });
  td.appendChild(group);

  if (!g.activeVariant) {
    var q = el('span', 'unknown', '?');
    q.title = 'The installed bundle matches none of the stored builds — ' +
              'something else was copied into data/cartridges. Pick one to be sure.';
    td.appendChild(q);
  }
  return td;
}

function renderGames(data) {
  controlEnabled = data.controlEnabled;
  paintTokenState();
  var body = document.getElementById('games');
  body.textContent = '';
  data.games.forEach(function (g) {
    var tr = el('tr');
    tr.dataset.slug = g.slug;

    var tdIcon = el('td', 'icon');
    if (g.icon) {
      var img = document.createElement('img');
      img.src = g.icon;            // server-generated data: URI, see iconDataURI
      img.alt = '';
      tdIcon.appendChild(img);
    }
    tr.appendChild(tdIcon);

    var tdName = el('td');
    tdName.appendChild(el('div', 'gname', g.name));
    tdName.appendChild(el('div', 'gslug', g.slug + (g.version ? '  v' + g.version : '')));
    tr.appendChild(tdName);

    var tdAttract = el('td');
    var badge = el('span', 'badge ' + g.attract, g.attract);
    badge.title = g.attract === 'custom'
      ? 'The cartridge draws its own attract screen (cart_attract).'
      : g.attract === 'showcase'
        ? 'No cart_attract export: previewing shows the engine default (logo, trophy, leaderboard).'
        : 'Nothing to show: no cart_attract export and no recorded score.';
    tdAttract.appendChild(badge);
    tr.appendChild(tdAttract);

    tr.appendChild(variantCell(g));

    var tdScores = el('td', 'scorecell');
    if (!g.scoresEditable) {
      tdScores.appendChild(el('span', 'none', '—'));
    } else if (!g.scores || g.scores.length === 0) {
      tdScores.appendChild(el('span', 'none', 'no scores'));
    } else {
      var best = g.scores[0];
      tdScores.appendChild(el('span', null,
        best.score + (best.name ? '  ' + best.name : '') +
        (g.scores.length > 1 ? '   (+' + (g.scores.length - 1) + ')' : '')));
    }
    tr.appendChild(tdScores);

    var tdActions = el('td', 'actions');
    var launch = el('button', 'launch primary', 'Launch');
    launch.onclick = function () {
      send('POST', '/api/launch/' + g.slug).then(function () {
        toast('Launched ' + g.name + '.', true);
      }).catch(function (e) { toast('Launch failed: ' + e.message, false); });
    };
    tdActions.appendChild(launch);

    var attract = el('button', 'attract', 'Attract');
    attract.dataset.attract = g.attract;
    attract.style.marginLeft = '6px';
    attract.onclick = function () {
      send('POST', '/api/attract/' + g.slug).then(function (res) {
        toast('Showing ' + g.name + ' attract (' + res.attract + '). ' +
              'Any button at the cabinet wakes it back to what was running.', true);
      }).catch(function (e) { toast('Attract failed: ' + e.message, false); });
    };
    tdActions.appendChild(attract);

    if (g.scoresEditable) {
      var edit = el('button', 'edit', 'Scores');
      edit.style.marginLeft = '6px';
      edit.onclick = function () { toggleEditor(tr, g); };
      tdActions.appendChild(edit);
    }
    tr.appendChild(tdActions);
    body.appendChild(tr);
  });
  var anyVariants = data.games.some(function (g) { return g.variants && g.variants.length > 1; });
  document.getElementById('gamesTable').classList.toggle('has-variants', anyVariants);
  applyReadiness();
}

function toggleEditor(tr, g) {
  var next = tr.nextElementSibling;
  if (next && next.classList.contains('editor')) { next.remove(); return; }

  var row = el('tr', 'editor');
  var td = document.createElement('td');
  td.colSpan = 6;
  var box = el('div', 'editor');

  var ta = document.createElement('textarea');
  ta.value = scoreLines(g.scores || []);
  ta.spellcheck = false;
  box.appendChild(ta);

  box.appendChild(el('div', 'hint',
    'One entry per line: "<score> <name>", or just "<score>" for an anonymous entry. ' +
    'Up to 10 lines; names are A-Z / 0-9 / spaces, 12 characters max, and are uppercased. ' +
    'Saving REPLACES the whole table; an empty box clears it.'));
  if (lastStatus.slug === g.slug) {
    box.appendChild(el('div', 'warn',
      'Heads up: ' + g.name + ' is the app running right now, and it saves its own table ' +
      'when it exits — it may overwrite this. Edit it while something else is on screen.'));
  }

  var actions = el('div', 'row');
  var save = el('button', 'primary', 'Save table');
  save.onclick = function () {
    var entries = [];
    var bad = null;
    ta.value.split('\n').forEach(function (line) {
      line = line.trim();
      if (!line) return;
      var sp = line.indexOf(' ');
      var scoreText = sp === -1 ? line : line.slice(0, sp);
      var name = sp === -1 ? '' : line.slice(sp + 1).trim();
      if (!/^[0-9]+$/.test(scoreText)) { bad = bad || ('not a score: "' + line + '"'); return; }
      entries.push({ score: parseInt(scoreText, 10), name: name });
    });
    if (bad) { toast('Nothing sent — ' + bad, false); return; }
    send('PUT', '/api/scores/' + g.slug, { entries: entries }).then(function (table) {
      toast('Saved ' + table.entries.length + ' entries for ' + g.name + '.', true);
      loadGames();
    }).catch(function (e) { toast('Save failed: ' + e.message, false); });
  };
  actions.appendChild(save);

  var clear = el('button', null, 'Clear table');
  clear.onclick = function () {
    send('PUT', '/api/scores/' + g.slug, { entries: [] }).then(function () {
      toast('Cleared ' + g.name + '.', true);
      loadGames();
    }).catch(function (e) { toast('Clear failed: ' + e.message, false); });
  };
  actions.appendChild(clear);

  var close = el('button', null, 'Close');
  close.onclick = function () { row.remove(); };
  actions.appendChild(close);

  box.appendChild(actions);
  td.appendChild(box);
  row.appendChild(td);
  tr.parentNode.insertBefore(row, tr.nextSibling);
}

// applyReadiness re-derives every action button's enabled state from the latest /api/status poll.
// LAUNCH needs only a token; externalLaunchReady does not gate it.
function applyReadiness() {
  var st = lastStatus;
  var tok = haveToken() && controlEnabled !== false;
  var launchable = tok;
  var rows = document.querySelectorAll('#games tr');
  for (var i = 0; i < rows.length; i++) {
    rows[i].classList.toggle('running', rows[i].dataset.slug === st.slug);
  }
  var lb = document.querySelectorAll('button.launch');
  for (var j = 0; j < lb.length; j++) {
    lb[j].disabled = !launchable;
    lb[j].title = tok
      ? 'Switch the cabinet to this app now, whatever is on screen.'
      : 'Set the control token above first.';
  }
  var ab = document.querySelectorAll('button.attract');
  for (var k = 0; k < ab.length; k++) {
    var mode = ab[k].dataset.attract;
    var ok = tok && mode !== 'none' && st.standbyEnabled !== false;
    ab[k].disabled = !ok;
    ab[k].title = !tok ? 'Set the control token above first.'
      : mode === 'none' ? 'This game has no attract screen: no cart_attract export and no recorded score.'
      : st.standbyEnabled === false ? 'Standby is disabled on this cabinet (--attract-after 0).'
      : 'Show this attract screen on the cabinet now, from any state.';
  }
  var eb = document.querySelectorAll('button.edit');
  for (var n = 0; n < eb.length; n++) {
    eb[n].disabled = false; // reading is always allowed; only Save needs the token
  }
  // The installed build is lit and disabled; reinstalling it would only rewrite a file.
  var vb = document.querySelectorAll('button.variant');
  for (var p = 0; p < vb.length; p++) {
    var installed = vb[p].dataset.installed === '1';
    vb[p].disabled = !tok || installed;
    vb[p].title = !tok ? 'Set the control token above first.'
      : installed ? 'This build is the one installed right now.'
      : 'Install the ' + vb[p].dataset.variant + ' build of this game. Takes effect on the ' +
        'NEXT launch — anything already running keeps the build it started with.';
  }
  var mode = document.getElementById('ssMode');
  mode.disabled = !tok;
  mode.title = tok ? '' : 'Set the control token above first.';
  var manual = mode.value === 'manual';
  var ssb = document.querySelectorAll('#ssSave, #ssClear');
  for (var q = 0; q < ssb.length; q++) {
    ssb[q].disabled = !tok || !manual;
    ssb[q].title = !tok ? 'Set the control token above first.'
      : !manual ? 'This screensaver looks itself up; switch Location to Manual to type coordinates.'
      : '';
  }
  var show = document.getElementById('ssShow');
  show.disabled = !tok || st.standbyEnabled === false;
  show.title = !tok ? 'Set the control token above first.'
    : st.standbyEnabled === false ? 'Standby is disabled on this cabinet (--attract-after 0).'
    : 'Show this screensaver on the cabinet now, whatever its schedule says.';
}

function loadGames() {
  return fetch('/api/games').then(function (r) { return r.json(); })
    .then(renderGames)
    .catch(function (e) { toast('Could not load the game list: ' + e.message, false); });
}
document.getElementById('gamesReload').onclick = loadGames;

var screensavers = [];

function selectedScreensaver() {
  var slug = document.getElementById('ssSelect').value;
  var found = null;
  screensavers.forEach(function (s) { if (s.slug === slug) found = s; });
  return found;
}

function fillScreensaverFields() {
  var found = selectedScreensaver();
  document.getElementById('ssMode').value = (found && found.location) || 'auto';
  document.getElementById('ssLat').value = (found && found.lat != null) ? found.lat : '';
  document.getElementById('ssLon').value = (found && found.lon != null) ? found.lon : '';
  applyLocationMode();
}

function applyLocationMode() {
  var manual = document.getElementById('ssMode').value === 'manual';
  document.getElementById('ssLat').disabled = !manual;
  document.getElementById('ssLon').disabled = !manual;
  applyReadiness();

  var found = selectedScreensaver();
  var fix = found && found.fix;
  var el = document.getElementById('ssFix');
  if (manual) {
    el.textContent = '';
  } else if (fix) {
    el.textContent = 'resolved: ' + (fix.city || (fix.lat.toFixed(2) + ', ' + fix.lon.toFixed(2))) +
      ' (' + fix.lat.toFixed(2) + ', ' + fix.lon.toFixed(2) + ') via ' + (fix.source || 'unknown') +
      ' at ' + new Date(fix.at).toLocaleString();
  } else {
    el.textContent = 'not resolved yet — the screensaver looks itself up the next time it loads';
  }
}

function renderScreensavers(list) {
  screensavers = list;
  var sel = document.getElementById('ssSelect');
  var prev = sel.value;
  sel.textContent = '';
  screensavers.forEach(function (s) {
    var opt = document.createElement('option');
    opt.value = s.slug;
    opt.textContent = s.name + ' (' + s.slug + ')';
    sel.appendChild(opt);
  });
  if (prev) sel.value = prev;
  fillScreensaverFields();
  applyReadiness();
}

function loadScreensavers() {
  return fetch('/api/screensavers').then(function (r) { return r.json(); })
    .then(renderScreensavers)
    .catch(function (e) { toast('Could not load screensavers: ' + e.message, false); });
}
document.getElementById('ssSelect').onchange = fillScreensaverFields;

function saveScreensaverLocation(body, note) {
  var slug = document.getElementById('ssSelect').value;
  if (!slug) { toast('No screensaver to save.', false); return; }
  send('PUT', '/api/screensavers/' + slug, body).then(function (res) {
    toast(note(res.name), true);
    loadScreensavers();
  }).catch(function (e) { toast('Save failed: ' + e.message, false); });
}
document.getElementById('ssMode').onchange = function () {
  applyLocationMode();
  var mode = document.getElementById('ssMode').value;
  saveScreensaverLocation({ location: mode }, function (name) {
    return mode === 'auto'
      ? name + ' will find itself the next time it loads.'
      : name + ' will use the coordinates set here.';
  });
};
document.getElementById('ssSave').onclick = function () {
  var lat = parseFloat(document.getElementById('ssLat').value.trim());
  var lon = parseFloat(document.getElementById('ssLon').value.trim());
  if (isNaN(lat) || isNaN(lon)) {
    toast('Lat and lon must both be numbers (use Clear to remove the location).', false);
    return;
  }
  saveScreensaverLocation({ location: 'manual', lat: lat, lon: lon }, function (name) {
    return 'Saved location for ' + name + '.';
  });
};
document.getElementById('ssClear').onclick = function () {
  document.getElementById('ssLat').value = '';
  document.getElementById('ssLon').value = '';
  saveScreensaverLocation({ location: 'manual', lat: null, lon: null }, function (name) {
    return 'Cleared the location for ' + name + '; its weather row is off.';
  });
};
document.getElementById('ssShow').onclick = function () {
  var slug = document.getElementById('ssSelect').value;
  if (!slug) { toast('No screensaver to show.', false); return; }
  send('POST', '/api/screensavers/' + slug + '/show').then(function (res) {
    toast('Showing ' + res.name + ' on the cabinet, schedule ignored. ' +
          'Any button there wakes it back to what was running.', true);
  }).catch(function (e) { toast('Show failed: ' + e.message, false); });
};

async function refresh() {
  try {
    const [st, logs] = await Promise.all([
      fetch('/api/status').then(r => r.json()),
      fetch('/api/logs?n=200').then(r => r.json()),
    ]);
    lastStatus = st;
    document.getElementById('err').style.display = 'none';
    document.getElementById('sub').textContent =
      'version ' + st.version + ' · process uptime ' + fmtMs(st.processUptimeMs);

    const tiles = [
      tile('App', st.slug || '(none)'),
      tile('Suspended', st.suspendedSlug || '—'),
      tile('Standby', st.standbyState.toUpperCase(), 'state-' + st.standbyState),
      tile('Ticks', st.ticks.toLocaleString()),
      tile('Uptime', fmtMs(st.uptimeMs)),
      tile('App duration', fmtMs(st.durationMs)),
      tile('Dropped frames', st.droppedFrames.toLocaleString()),
      tile('Heap / Sys', st.heapMB.toFixed(1) + ' / ' + st.sysMB.toFixed(1) + ' MB'),
      tile('CPU temp', st.tempF != null ? st.tempF.toFixed(0) + '°F' : '—'),
      tile('Fan', st.fanOn ? 'ON' : 'OFF', st.fanOn ? 'on' : 'off'),
      tile('Motion', st.motionActive ? 'ACTIVE' : 'IDLE', st.motionActive ? 'on' : 'off'),
      tile('Motion seen', fmtMs(st.motionAgoMs)),
    ];
    document.getElementById('grid').replaceChildren(...tiles);
    applyReadiness();

    const box = document.getElementById('logs');
    const atBottom = box.scrollTop + box.clientHeight >= box.scrollHeight - 4;
    box.textContent = '';
    for (const e of logs) {
      const line = document.createElement('div');
      line.className = 'lvl-' + e.level;
      const t = new Date(e.time).toLocaleTimeString();
      line.textContent = t + '  [' + e.level + ']  ' + e.msg;
      box.appendChild(line);
    }
    if (atBottom) box.scrollTop = box.scrollHeight;
  } catch (e) {
    document.getElementById('err').style.display = 'block';
  }
}
paintTokenState();
refresh();
loadGames();
loadScreensavers();
setInterval(refresh, 2000);
</script>
</body>
</html>
`
