package main

// viewerPage renders the match live from the frames relayed by the viewer.
// The server does not broadcast projectiles, they are animated from the
// shooting flag of each player. Hits and deaths use the health sent by the server.
const viewerPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>triebwerk load test</title>
<style>
  :root {
    --bg: #0d1117; --panel: #161b22; --line: #30363d; --text: #e6edf3; --muted: #8b949e;
    --good: #3fb950; --warn: #d29922; --bad: #f85149; --accent: #58a6ff;
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; height: 100%; background: var(--bg); color: var(--text);
    font: 13px/1.4 ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif; }
  body { display: flex; }
  #arena { flex: 1; position: relative; min-width: 0; }
  canvas { position: absolute; inset: 0; width: 100%; height: 100%; display: block; }
  aside { width: 390px; flex-shrink: 0; border-left: 1px solid var(--line); background: var(--panel);
    display: flex; flex-direction: column; overflow: hidden; }
  header { padding: 14px 16px; border-bottom: 1px solid var(--line); display: flex; align-items: center; gap: 8px; }
  header h1 { font-size: 14px; margin: 0; font-weight: 600; flex: 1; }
  .conn { font-size: 12px; color: var(--muted); display: flex; align-items: center; gap: 6px; }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--bad); }
  .dot.on { background: var(--good); }
  .stats { display: grid; grid-template-columns: 1fr 1fr 1fr; border-bottom: 1px solid var(--line); }
  .stat { padding: 10px 16px; border-right: 1px solid var(--line); border-bottom: 1px solid var(--line); }
  .stat:nth-child(3n) { border-right: 0; }
  .stat:nth-last-child(-n+3) { border-bottom: 0; }
  .stat .label { color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: .04em; }
  .stat .value { font-size: 18px; font-weight: 600; font-variant-numeric: tabular-nums; }
  .good { color: var(--good); } .warn { color: var(--warn); } .bad { color: var(--bad); }
  h2 { font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: var(--muted);
    margin: 0; padding: 12px 16px 6px; font-weight: 600; }
  #players { overflow-y: auto; flex: 1; padding: 0 8px; }
  .player { display: grid; grid-template-columns: 12px 1fr auto 44px 30px; align-items: center; gap: 8px;
    padding: 5px 8px; border-radius: 6px; }
  .player.dead { opacity: .45; }
  .swatch { width: 12px; height: 12px; border-radius: 3px; }
  .name { font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .name small { color: var(--muted); }
  .keys { display: flex; gap: 3px; font-size: 11px; }
  .key { width: 18px; height: 16px; display: grid; place-items: center; border-radius: 3px;
    background: #21262d; color: #484f58; }
  .key.on { background: var(--accent); color: #0d1117; }
  .key.shoot.on { background: var(--warn); }
  .hp { height: 6px; background: #21262d; border-radius: 3px; overflow: hidden; }
  .hp div { height: 100%; background: var(--good); }
  .deaths { color: var(--muted); font-size: 12px; text-align: right; font-variant-numeric: tabular-nums; }
  #log { height: 170px; overflow-y: auto; border-top: 1px solid var(--line); padding: 4px 16px 10px;
    font: 12px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace; color: var(--muted); }
  #log .t { color: #484f58; margin-right: 6px; }
  footer { padding: 8px 16px; border-top: 1px solid var(--line); color: var(--muted); font-size: 11px; }
  @media (max-width: 760px) {
    html, body { height: auto; }
    body { display: block; }
    #arena { height: 60vh; }
    aside { width: 100%; border-left: 0; border-top: 1px solid var(--line); }
    #players { overflow: visible; }
  }
</style>
</head>
<body>
<div id="arena"><canvas id="canvas"></canvas></div>
<aside>
  <header>
    <h1>triebwerk load test</h1>
    <span class="conn"><span id="dot" class="dot"></span><span id="conn">connecting</span></span>
  </header>
  <div class="stats">
    <div class="stat"><div class="label">Game</div><div class="value" id="s-game">–</div></div>
    <div class="stat"><div class="label">Time left</div><div class="value" id="s-time">–</div></div>
    <div class="stat"><div class="label">Clients</div><div class="value" id="s-clients">–</div></div>
    <div class="stat"><div class="label">Updates/s</div><div class="value" id="s-ups">–</div></div>
    <div class="stat"><div class="label">Max gap</div><div class="value" id="s-gap">–</div></div>
    <div class="stat"><div class="label">Lost conns</div><div class="value" id="s-lost">–</div></div>
  </div>
  <h2>Players</h2>
  <div id="players"></div>
  <h2 style="border-top:1px solid var(--line)">Events</h2>
  <div id="log"></div>
  <footer>Projectiles are animated from shot events, the server does not send their positions. Hits and deaths come from server health values.</footer>
</aside>
<script>
(function () {
  var GAME_LENGTH = 300000, PROJECTILE_SPEED = 100, HALF_WIDTH = 2.5, HALF_DEPTH = 3.5;
  var KEY_GLYPHS = ['▲', '▼', '◀', '▶', '↻', '↺', '●'];
  var KEY_TITLES = ['forward', 'backward', 'left', 'right', 'turret right', 'turret left', 'shoot'];

  var canvas = document.getElementById('canvas');
  var ctx = canvas.getContext('2d');
  var map = null, view = { scale: 1, ox: 0, oy: 0 };
  var players = {}, bots = {}, projectiles = [], effects = [];
  var running = false, gameTime = 0, frameAt = 0, lastStats = null, connected = false;

  function hue(id) { return (id * 137.508) % 360; }
  function color(id, l) { return 'hsl(' + hue(id).toFixed(0) + ',70%,' + (l || 58) + '%)'; }

  // world to screen, y points up in the world
  function sx(x) { return view.ox + (x - map.min.x) * view.scale; }
  function sy(y) { return view.oy + (map.max.y - y) * view.scale; }

  function resize() {
    var dpr = window.devicePixelRatio || 1;
    var w = canvas.clientWidth, h = canvas.clientHeight;
    canvas.width = Math.round(w * dpr); canvas.height = Math.round(h * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    if (!map) return;
    var pad = 36, b = map.inner, mw = b.max.x - b.min.x, mh = b.max.y - b.min.y;
    view.scale = Math.min((w - 2 * pad) / mw, (h - 2 * pad) / mh);
    view.ox = (w - mw * view.scale) / 2 - (b.min.x - map.min.x) * view.scale;
    view.oy = (h - mh * view.scale) / 2 - (map.max.y - b.max.y) * view.scale;
  }
  window.addEventListener('resize', resize);

  function hull(p) {
    var c = Math.cos(p.rot), s = Math.sin(p.rot);
    return [[-HALF_WIDTH, HALF_DEPTH], [HALF_WIDTH, HALF_DEPTH], [HALF_WIDTH, -HALF_DEPTH], [-HALF_WIDTH, -HALF_DEPTH]]
      .map(function (v) { return { x: p.x + v[0] * c - v[1] * s, y: p.y + v[0] * s + v[1] * c }; });
  }

  function inPolygon(x, y, pts) {
    var inside = false;
    for (var i = 0, j = pts.length - 1; i < pts.length; j = i++) {
      if (((pts[i].y > y) !== (pts[j].y > y)) &&
          (x < (pts[j].x - pts[i].x) * (y - pts[i].y) / (pts[j].y - pts[i].y) + pts[i].x)) inside = !inside;
    }
    return inside;
  }

  function clock(ms) {
    var s = Math.max(0, Math.round(ms / 1000));
    return Math.floor(s / 60) + ':' + ('0' + (s % 60)).slice(-2);
  }

  function logEvent(text, cls) {
    var el = document.getElementById('log');
    var line = document.createElement('div');
    var t = document.createElement('span');
    t.className = 't';
    t.textContent = running ? clock(GAME_LENGTH - gameTime) : '-:--';
    line.appendChild(t);
    line.appendChild(document.createTextNode(text));
    if (cls) line.className = cls;
    el.prepend(line);
    while (el.childNodes.length > 60) el.removeChild(el.lastChild);
  }

  function label(id) { return bots[id] ? '#' + id + ' ' + bots[id].name : '#' + id; }

  function onFrame(f) {
    var now = performance.now();
    gameTime = f.t; frameAt = now;
    var seen = {};
    f.players.forEach(function (p) {
      seen[p.id] = true;
      var prev = players[p.id];
      if (!prev) {
        prev = players[p.id] = { deaths: 0, lastShot: -1e9, flash: -1e9 };
        for (var k in p) prev[k] = p[k];
        return;
      }
      if (p.health < prev.health && p.health > 0) prev.flash = now;
      if (prev.health > 0 && p.health === 0) {
        prev.deaths++;
        effects.push({ kind: 'explosion', x: p.x, y: p.y, born: now, id: p.id });
        logEvent(label(p.id) + ' destroyed', 'bad');
      }
      if (prev.health === 0 && p.health > 0) logEvent(label(p.id) + ' respawned');
      if (p.shoot && p.health > 0 && now - prev.lastShot > 900) {
        prev.lastShot = now;
        var dx = p.tx - p.x, dy = p.ty - p.y, len = Math.hypot(dx, dy) || 1;
        projectiles.push({ x: p.tx, y: p.ty, dx: dx / len, dy: dy / len, owner: p.id, born: now, trail: [] });
      }
      for (var key in p) prev[key] = p[key];
    });
    Object.keys(players).forEach(function (id) {
      if (!seen[id]) { logEvent(label(id) + ' left the game'); delete players[id]; }
    });
  }

  function onEvent(e) {
    running = e.event === 'start';
    if (running) {
      projectiles = [];
      Object.keys(players).forEach(function (id) { players[id].deaths = 0; });
      logEvent('game started', 'good');
    } else {
      logEvent('game ended, next game in 10s', 'warn');
    }
  }

  function onStats(s) {
    lastStats = s;
    running = s.running;
    bots = {};
    (s.bots || []).forEach(function (b) { if (b.id) bots[b.id] = b; });
    set('s-game', running ? 'Running' : 'Paused', running ? 'good' : 'warn');
    set('s-clients', s.connected + '/' + s.clients, s.connected === s.clients ? 'good' : 'warn');
    set('s-ups', running ? s.updatesPerSec : '–', !running ? '' : s.updatesPerSec >= 28 ? 'good' : s.updatesPerSec >= 20 ? 'warn' : 'bad');
    set('s-gap', running ? s.maxGap + 'ms' : '–', !running ? '' : s.maxGap < 60 ? 'good' : s.maxGap < 200 ? 'warn' : 'bad');
    set('s-lost', s.lostConns, s.lostConns === 0 ? 'good' : 'bad');
    renderPlayers();
  }

  function set(id, text, cls) {
    var el = document.getElementById(id);
    el.textContent = text;
    el.className = 'value ' + (cls || '');
  }

  function renderPlayers() {
    var el = document.getElementById('players');
    var ids = Object.keys(players).map(Number).sort(function (a, b) { return a - b; });
    el.textContent = '';
    ids.forEach(function (id) {
      var p = players[id], b = bots[id];
      var row = document.createElement('div');
      row.className = 'player' + (p.health === 0 ? ' dead' : '');
      var sw = document.createElement('div'); sw.className = 'swatch'; sw.style.background = color(id);
      var name = document.createElement('div'); name.className = 'name';
      name.textContent = '#' + id + ' ';
      var small = document.createElement('small'); small.textContent = b ? b.name : '';
      name.appendChild(small);
      var hp = document.createElement('div'); hp.className = 'hp';
      var bar = document.createElement('div'); bar.style.width = p.health + '%';
      bar.style.background = p.health > 50 ? 'var(--good)' : p.health > 25 ? 'var(--warn)' : 'var(--bad)';
      hp.appendChild(bar);
      var deaths = document.createElement('div'); deaths.className = 'deaths';
      deaths.textContent = '☠' + p.deaths; deaths.title = 'deaths this game';
      var keys = document.createElement('div'); keys.className = 'keys';
      KEY_GLYPHS.forEach(function (g, i) {
        var k = document.createElement('span');
        k.className = 'key' + (i === 6 ? ' shoot' : '') + (b && b.keys[i] ? ' on' : '');
        k.textContent = g; k.title = KEY_TITLES[i];
        keys.appendChild(k);
      });
      row.appendChild(sw); row.appendChild(name); row.appendChild(keys); row.appendChild(hp); row.appendChild(deaths);
      el.appendChild(row);
    });
  }

  function stepProjectiles(dt, now) {
    if (!map) return;
    projectiles = projectiles.filter(function (b) {
      b.trail.push({ x: b.x, y: b.y });
      if (b.trail.length > 6) b.trail.shift();
      b.x += b.dx * PROJECTILE_SPEED * dt;
      b.y += b.dy * PROJECTILE_SPEED * dt;
      if (now - b.born > 5000) return false;
      if (b.x < map.min.x || b.x > map.max.x || b.y < map.min.y || b.y > map.max.y) return false;
      for (var i = 0; i < map.colliders.length; i++) {
        var c = map.colliders[i];
        if (c.projectile && inPolygon(b.x, b.y, c.points)) {
          effects.push({ kind: 'impact', x: b.x, y: b.y, born: now });
          return false;
        }
      }
      for (var id in players) {
        var p = players[id];
        if (+id === b.owner || p.health === 0) continue;
        if (inPolygon(b.x, b.y, hull(p))) {
          effects.push({ kind: 'impact', x: b.x, y: b.y, born: now, hit: true });
          return false;
        }
      }
      return true;
    });
  }

  function poly(pts) {
    ctx.beginPath();
    pts.forEach(function (pt, i) { i ? ctx.lineTo(sx(pt.x), sy(pt.y)) : ctx.moveTo(sx(pt.x), sy(pt.y)); });
    ctx.closePath();
  }

  function drawMap() {
    var w = canvas.clientWidth, h = canvas.clientHeight;
    ctx.fillStyle = '#0d1117';
    ctx.fillRect(0, 0, w, h);
    ctx.fillStyle = '#131920';
    ctx.fillRect(sx(map.min.x), sy(map.max.y), (map.max.x - map.min.x) * view.scale, (map.max.y - map.min.y) * view.scale);
    ctx.strokeStyle = 'rgba(255,255,255,0.035)';
    ctx.lineWidth = 1;
    for (var gx = Math.ceil(map.min.x / 25) * 25; gx <= map.max.x; gx += 25) {
      ctx.beginPath(); ctx.moveTo(sx(gx), sy(map.min.y)); ctx.lineTo(sx(gx), sy(map.max.y)); ctx.stroke();
    }
    for (var gy = Math.ceil(map.min.y / 25) * 25; gy <= map.max.y; gy += 25) {
      ctx.beginPath(); ctx.moveTo(sx(map.min.x), sy(gy)); ctx.lineTo(sx(map.max.x), sy(gy)); ctx.stroke();
    }
    map.colliders.forEach(function (c) {
      poly(c.points);
      if (c.projectile) {
        ctx.fillStyle = '#2d333b'; ctx.fill();
        ctx.strokeStyle = '#444c56'; ctx.setLineDash([]); ctx.stroke();
      } else { // shots fly across low walls
        ctx.fillStyle = 'rgba(110,118,129,0.12)'; ctx.fill();
        ctx.strokeStyle = 'rgba(110,118,129,0.5)'; ctx.setLineDash([4, 3]); ctx.stroke();
        ctx.setLineDash([]);
      }
    });
    map.spawns.forEach(function (s) {
      ctx.beginPath();
      ctx.arc(sx(s.x), sy(s.y), 4 * view.scale, 0, Math.PI * 2);
      ctx.strokeStyle = 'rgba(88,166,255,0.25)'; ctx.lineWidth = 1; ctx.stroke();
    });
  }

  function drawTank(id, p, now) {
    var alive = p.health > 0;
    poly(hull(p));
    ctx.fillStyle = alive ? color(id) : 'rgba(110,118,129,0.35)';
    ctx.fill();
    ctx.lineWidth = Math.max(1, 0.4 * view.scale);
    ctx.strokeStyle = alive ? color(id, 30) : 'rgba(110,118,129,0.6)';
    ctx.stroke();
    var px = sx(p.x), py = sy(p.y);
    if (!alive) {
      var r = 2.5 * view.scale;
      ctx.strokeStyle = 'rgba(248,81,73,0.7)'; ctx.lineWidth = 2;
      ctx.beginPath(); ctx.moveTo(px - r, py - r); ctx.lineTo(px + r, py + r);
      ctx.moveTo(px + r, py - r); ctx.lineTo(px - r, py + r); ctx.stroke();
      return;
    }
    // barrel through the turret point
    var dx = p.tx - p.x, dy = p.ty - p.y, len = Math.hypot(dx, dy) || 1;
    ctx.strokeStyle = color(id, 25); ctx.lineWidth = Math.max(2, 0.9 * view.scale); ctx.lineCap = 'round';
    ctx.beginPath(); ctx.moveTo(px, py); ctx.lineTo(sx(p.x + dx / len * 5), sy(p.y + dy / len * 5)); ctx.stroke();
    ctx.beginPath(); ctx.arc(px, py, 1.6 * view.scale, 0, Math.PI * 2);
    ctx.fillStyle = color(id, 38); ctx.fill();
    // hit flash
    var since = now - p.flash;
    if (since < 300) {
      poly(hull(p));
      ctx.strokeStyle = 'rgba(255,255,255,' + (1 - since / 300) + ')'; ctx.lineWidth = 3; ctx.stroke();
    }
    // health and id
    var bw = 9 * view.scale, bx = px - bw / 2, by = py - 6 * view.scale;
    ctx.fillStyle = 'rgba(0,0,0,0.55)'; ctx.fillRect(bx, by, bw, 3);
    ctx.fillStyle = p.health > 50 ? '#3fb950' : p.health > 25 ? '#d29922' : '#f85149';
    ctx.fillRect(bx, by, bw * p.health / 100, 3);
    ctx.fillStyle = '#e6edf3'; ctx.font = '600 11px ui-sans-serif, system-ui, sans-serif'; ctx.textAlign = 'center';
    ctx.fillText('#' + id, px, by - 4);
  }

  function drawProjectiles() {
    projectiles.forEach(function (b) {
      if (b.trail.length) {
        ctx.beginPath(); ctx.moveTo(sx(b.trail[0].x), sy(b.trail[0].y));
        b.trail.forEach(function (t) { ctx.lineTo(sx(t.x), sy(t.y)); });
        ctx.lineTo(sx(b.x), sy(b.y));
        ctx.strokeStyle = 'rgba(255,214,102,0.35)'; ctx.lineWidth = 2; ctx.stroke();
      }
      ctx.beginPath(); ctx.arc(sx(b.x), sy(b.y), 2.5, 0, Math.PI * 2);
      ctx.fillStyle = '#ffd666'; ctx.fill();
    });
  }

  function drawEffects(now) {
    effects = effects.filter(function (e) {
      var life = e.kind === 'explosion' ? 700 : 250, age = (now - e.born) / life;
      if (age >= 1) return false;
      ctx.beginPath();
      var r = (e.kind === 'explosion' ? 3 + 9 * age : 1 + 3 * age) * view.scale;
      ctx.arc(sx(e.x), sy(e.y), r, 0, Math.PI * 2);
      ctx.strokeStyle = e.kind === 'explosion' ? 'rgba(255,140,50,' + (1 - age) + ')' :
        e.hit ? 'rgba(255,255,255,' + (1 - age) + ')' : 'rgba(255,214,102,' + (1 - age) + ')';
      ctx.lineWidth = e.kind === 'explosion' ? 3 : 1.5;
      ctx.stroke();
      return true;
    });
  }

  function drawOverlay(now) {
    ctx.textAlign = 'left';
    ctx.font = '600 13px ui-sans-serif, system-ui, sans-serif';
    ctx.fillStyle = '#8b949e';
    if (running) {
      var t = gameTime + (now - frameAt);
      ctx.fillText(clock(GAME_LENGTH - t) + ' left', 16, 24);
      document.getElementById('s-time').textContent = clock(GAME_LENGTH - t);
    } else {
      document.getElementById('s-time').textContent = '–';
    }
    var message = !connected ? 'Not connected to the load test, retrying' :
      !running && Object.keys(players).length ? 'Game over, next game starts shortly' :
      !running ? 'Waiting for the game to start' : '';
    if (message) {
      var w = canvas.clientWidth, h = canvas.clientHeight;
      ctx.fillStyle = 'rgba(13,17,23,0.72)'; ctx.fillRect(0, h / 2 - 28, w, 56);
      ctx.fillStyle = '#e6edf3'; ctx.textAlign = 'center'; ctx.font = '600 15px ui-sans-serif, system-ui, sans-serif';
      ctx.fillText(message, w / 2, h / 2 + 5);
    }
  }

  var last = performance.now();
  function frame(now) {
    var dt = Math.min(0.1, (now - last) / 1000);
    last = now;
    if (map) {
      if (running) stepProjectiles(dt, now);
      drawMap();
      Object.keys(players).forEach(function (id) { if (players[id].health === 0) drawTank(+id, players[id], now); });
      Object.keys(players).forEach(function (id) { if (players[id].health > 0) drawTank(+id, players[id], now); });
      drawProjectiles();
      drawEffects(now);
      drawOverlay(now);
    }
    requestAnimationFrame(frame);
  }

  function connect() {
    var ws = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws');
    ws.onopen = function () {
      connected = true;
      document.getElementById('dot').className = 'dot on';
      document.getElementById('conn').textContent = 'live';
    };
    ws.onmessage = function (m) {
      var msg = JSON.parse(m.data);
      if (msg.type === 'frame') onFrame(msg);
      else if (msg.type === 'event') onEvent(msg);
      else if (msg.type === 'stats') onStats(msg);
    };
    ws.onclose = function () {
      connected = false;
      document.getElementById('dot').className = 'dot';
      document.getElementById('conn').textContent = 'reconnecting';
      setTimeout(connect, 1000);
    };
  }

  fetch('map.json').then(function (r) { return r.json(); }).then(function (m) {
    m.inner = { min: { x: m.min.x + 45, y: m.min.y + 45 }, max: { x: m.max.x - 45, y: m.max.y - 45 } };
    map = m;
    resize();
    connect();
    requestAnimationFrame(frame);
  });
})();
</script>
</body>
</html>
`
