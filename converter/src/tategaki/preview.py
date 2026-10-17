"""Static HTML preview of a .jbk (plan §5.7): pages with glyph overlays."""

import json
import zipfile
from pathlib import Path

TEMPLATE = r'''<!doctype html>
<html lang="ja"><head><meta charset="utf-8">
<title>Preview: __TITLE__</title>
<style>
  body { margin: 0; font: 14px system-ui, sans-serif; display: flex; height: 100vh; background: #ddd; }
  #stage { flex: 1; display: flex; align-items: center; justify-content: center; overflow: auto; }
  #wrap { position: relative; box-shadow: 0 0 8px #0006; background: #fff; }
  #wrap img, #wrap canvas { position: absolute; left: 0; top: 0; width: 100%; height: 100%; }
  #side { width: 380px; background: #fafafa; border-left: 1px solid #bbb; padding: 12px; overflow: auto; box-sizing: border-box; }
  #side h2 { font-size: 15px; margin: 12px 0 4px; }
  .ctx { font-size: 20px; line-height: 1.6; word-break: break-all; }
  .ctx mark { background: #fc6; } .ctx .sent { background: #cdf; }
  button { font-size: 14px; padding: 4px 10px; }
  code { background: #eee; padding: 0 3px; }
</style></head>
<body>
<div id="stage"><div id="wrap"><img id="pg"><canvas id="ov"></canvas></div></div>
<div id="side">
  <div><button id="next">◀ next</button> <button id="prev">prev ▶</button>
    page <input id="num" type="number" style="width:5em"> / <span id="total"></span></div>
  <div><label><input type="checkbox" id="boxes" checked> glyph boxes</label></div>
  <div id="pageinfo"></div>
  <h2>Selection</h2><div id="sel">Click a glyph.</div>
  <h2>Book</h2><div id="book"></div>
</div>
<script>
const DATA = __DATA__;
const pages = [];  // global page -> {chapter, entry}
DATA.chapters.forEach((ch, ci) => DATA.layouts[ci].pages.forEach(p => pages[p.page] = {ci, p}));
const W = DATA.manifest.profile.width, H = DATA.manifest.profile.height;
const wrap = document.getElementById('wrap'), ov = document.getElementById('ov');
let cur = 0, scale = 1, selected = null;
function fit() {
  const s = document.getElementById('stage');
  scale = Math.min((s.clientWidth - 20) / W, (s.clientHeight - 20) / H, 1);
  wrap.style.width = W * scale + 'px'; wrap.style.height = H * scale + 'px';
  ov.width = W; ov.height = H; draw();
}
function cps(s) { return Array.from(s); }
const texts = DATA.texts.map(cps);
function draw() {
  const ctx = ov.getContext('2d'); ctx.clearRect(0, 0, W, H);
  const pg = pages[cur]; if (!pg) return;
  const m = DATA.manifest.profile;
  if (document.getElementById('boxes').checked) {
    ctx.strokeStyle = 'rgba(0,120,255,.55)'; ctx.lineWidth = 1;
    for (const [o, x, y, w, h] of pg.p.glyphs) ctx.strokeRect(x + .5, y + .5, w, h);
  }
  for (const im of pg.p.images || []) { ctx.strokeStyle = 'rgba(0,160,0,.8)'; ctx.lineWidth = 3; ctx.strokeRect(...im.box); }
  if (selected && selected.page === cur) {
    ctx.fillStyle = 'rgba(255,140,0,.35)';
    for (const g of pg.p.glyphs) if (g[0] >= selected.s0 && g[0] < selected.s1) ctx.fillRect(g[1], g[2], g[3], g[4]);
    ctx.fillStyle = 'rgba(255,0,0,.5)'; const g = selected.g; ctx.fillRect(g[1], g[2], g[3], g[4]);
  }
}
function show(n) {
  cur = Math.max(0, Math.min(pages.length - 1, n));
  document.getElementById('pg').src = 'pages/' + String(cur).padStart(5, '0') + '.png';
  document.getElementById('num').value = cur;
  const pg = pages[cur], ch = DATA.manifest.chapters[pg.ci];
  document.getElementById('pageinfo').innerHTML =
    `chapter ${pg.ci} <code>${ch.source || ''}</code>, ${pg.p.glyphs.length} glyphs` +
    (pg.p.glyphs.length ? `, offsets ${pg.p.glyphs[0][0]}–${pg.p.glyphs[pg.p.glyphs.length - 1][0]}` : '');
  location.hash = cur; draw();
}
function esc(s) { return s.replace(/[&<>]/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;'}[c])); }
ov.addEventListener('click', ev => {
  const r = ov.getBoundingClientRect();
  const x = (ev.clientX - r.left) / scale, y = (ev.clientY - r.top) / scale;
  const pg = pages[cur]; let best = null, bd = 1e9;
  for (const g of pg.p.glyphs) {
    const dx = Math.max(g[1] - x, 0, x - g[1] - g[3]), dy = Math.max(g[2] - y, 0, y - g[2] - g[4]);
    const d = dx * dx + dy * dy; if (d < bd) { bd = d; best = g; }
  }
  if (!best || bd > 400) { selected = null; draw(); return; }
  const t = texts[pg.ci], off = best[0], L = DATA.layouts[pg.ci];
  const sent = L.sentences.find(([a, b]) => a <= off && off < b) || [off, off + 1];
  const para = L.paragraphs.find(([a, b]) => a <= off && off < b) || [off, off + 1];
  selected = {page: cur, g: best, s0: sent[0], s1: sent[1]};
  let html = '';
  for (let i = para[0]; i < para[1]; i++) {
    let c = esc(t[i]);
    if (i === off) c = `<mark>${c}</mark>`;
    else if (i >= sent[0] && i < sent[1]) c = `<span class="sent">${c}</span>`;
    html += c;
  }
  document.getElementById('sel').innerHTML =
    `<div>offset <code>${off}</code> char <code>${esc(t[off])}</code> U+${t[off].codePointAt(0).toString(16).toUpperCase()}` +
    ` box <code>${best.slice(1).join(',')}</code></div>` +
    `<div>sentence <code>[${sent}]</code> paragraph <code>[${para}]</code></div><div class="ctx">${html}</div>`;
  draw();
});
document.getElementById('next').onclick = () => show(cur + 1);
document.getElementById('prev').onclick = () => show(cur - 1);
document.getElementById('num').onchange = e => show(+e.target.value);
document.getElementById('boxes').onchange = draw;
document.addEventListener('keydown', e => {
  if (e.target.tagName === 'INPUT' && e.target.type === 'number') return;
  if (e.key === 'ArrowLeft' || e.key === ' ') show(cur + 1);
  if (e.key === 'ArrowRight' || e.key === 'Backspace') show(cur - 1);
});
const mf = DATA.manifest;
document.getElementById('total').textContent = pages.length - 1;
document.getElementById('book').innerHTML = `<b>${esc(mf.title)}</b> ${esc(mf.author || '')}<br>` +
  `render <code>${esc(JSON.stringify(mf.render))}</code><br>TOC:<br>` +
  mf.toc.map(t => `<a href="#" onclick="show(${t.page});return false">${'&nbsp;'.repeat(2 * (t.level - 1))}${esc(t.title)} → ${t.page}</a>`).join('<br>');
window.addEventListener('resize', fit);
fit(); show(+(location.hash.slice(1)) || 0);
</script></body></html>
'''


def build_preview(jbk: Path, out_dir: Path) -> Path:
    out_dir.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(jbk) as z:
        manifest = json.loads(z.read('manifest.json'))
        layouts = [json.loads(z.read(c['layout'])) for c in manifest['chapters']]
        texts = [z.read(c['text']).decode('utf-8') for c in manifest['chapters']]
        for name in z.namelist():
            if name.startswith(('pages/', 'images/')) or name == 'cover.png':
                target = out_dir / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(z.read(name))
    data = json.dumps({'manifest': manifest, 'layouts': layouts, 'texts': texts,
                       'chapters': manifest['chapters']}, ensure_ascii=False).replace('</', '<\\/')
    html = TEMPLATE.replace('__TITLE__', manifest.get('title', '')).replace('__DATA__', data)
    index = out_dir / 'index.html'
    index.write_text(html, encoding='utf-8')
    return index
