// Injected into each spine item by layout.py. Defines window.TG with the steps
// of laying out one chapter as a single vertical-rl strip. All coordinates
// returned are viewport CSS px at scroll position 0 (deviceScaleFactor is 1, so
// CSS px are device px).
(() => {
  const RUBY_ANNOTATION = 'rt, rp, rtc';
  const SKIP_TAGS = new Set(['rt', 'rp', 'rtc', 'script', 'style', 'head', 'title']);
  const FOLLOWING_CHARS = 8; // furigana.pyj
  const ASCII_WS = /^[ \t\n\r\f]$/;

  function isRubyAnnotation(node) {
    const el = node.nodeType === Node.TEXT_NODE ? node.parentElement : node;
    return !!(el && el.closest(RUBY_ANNOTATION));
  }

  function isBlock(el) {
    const d = getComputedStyle(el).display;
    return !(d.startsWith('inline') || d.startsWith('ruby') || d === 'contents' || d === 'none');
  }

  // --- Images ---------------------------------------------------------------

  function imageSource(el) {
    if (el.localName === 'img') return el.currentSrc || el.src;
    // <svg><image xlink:href=…>
    const href = el.getAttribute('href') || el.getAttributeNS('http://www.w3.org/1999/xlink', 'href');
    return href ? new URL(href, document.baseURI).href : null;
  }

  // Replace every block whose only visible content is one image (the light
  // novel illustration pattern) with a zero-size marker. Returns the sources in
  // document order; each marker forces a page break and becomes a full-screen
  // illustration page (plan §5.5).
  function extractIllustrations() {
    const sources = [];
    const images = Array.from(document.body.querySelectorAll('img, svg image'));
    for (const img of images) {
      let block = img.localName === 'image' ? img.closest('svg') : img;
      // climb to the outermost ancestor that still contains nothing else
      let target = null;
      for (let el = block.parentElement; el && el !== document.body; el = el.parentElement) {
        if (el.textContent.trim() !== '' || el.querySelectorAll('img, svg image').length !== 1) break;
        if (isBlock(el)) target = el;
      }
      if (!target) continue;
      const src = imageSource(img);
      if (!src) continue;
      const marker = document.createElement('div');
      marker.setAttribute('data-tg-illus', String(sources.length));
      marker.style.cssText = 'display:block;inline-size:0;block-size:0;margin:0;padding:0;border:0';
      target.replaceWith(marker);
      sources.push(src);
    }
    const remainingText = document.body.textContent.replace(/\s/g, '');
    const remainingImages = document.body.querySelectorAll('img, svg image').length;
    return { sources, onlyIllustrations: sources.length > 0 && !remainingText && !remainingImages };
  }

  // --- Furigana (port of furigana.pyj collect_groups / following_text) -----

  function rubyBase(ruby) {
    const clone = ruby.cloneNode(true);
    clone.querySelectorAll(RUBY_ANNOTATION).forEach((el) => el.remove());
    return clone.textContent.trim();
  }

  function nextRubySibling(ruby) {
    let node = ruby.nextSibling;
    while (node && node.nodeType === Node.TEXT_NODE && !node.nodeValue.trim()) node = node.nextSibling;
    return node && node.nodeType === Node.ELEMENT_NODE && node.localName === 'ruby' ? node : null;
  }

  function enclosingBlock(node) {
    const el = node.nodeType === Node.TEXT_NODE ? node.parentElement : node;
    if (!el) return document.body;
    return el.closest('p, li, blockquote, dd, td, section, article, div, body') || document.body;
  }

  function followingText(ruby) {
    const block = enclosingBlock(ruby);
    let node = ruby;
    while (node && node !== block && !node.nextSibling) node = node.parentNode;
    if (!node || node === block) return '';
    // No filter callback: page JS is disabled, and the browser can't call back
    // into script then. Skip ruby annotations by hand instead.
    const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT);
    const start = node.nextSibling;
    let ans = start.nodeType === Node.TEXT_NODE ? start.nodeValue : '';
    walker.currentNode = start;
    while (ans.length < FOLLOWING_CHARS) {
      const t = walker.nextNode();
      if (!t) break;
      if (!isRubyAnnotation(t)) ans += t.nodeValue;
    }
    return ans.slice(0, FOLLOWING_CHARS);
  }

  let rubyGroups = [];

  function collectRubyGroups() {
    rubyGroups = [];
    const seen = new Set();
    for (const ruby of document.body.querySelectorAll('ruby')) {
      if (seen.has(ruby) || (ruby.parentElement && ruby.parentElement.closest('ruby'))) continue;
      const rubies = [ruby];
      seen.add(ruby);
      let base = rubyBase(ruby);
      if (base.length === 1) {
        let nxt = nextRubySibling(ruby);
        while (nxt && rubyBase(nxt).length === 1) {
          rubies.push(nxt);
          seen.add(nxt);
          base += rubyBase(nxt);
          nxt = nextRubySibling(nxt);
        }
      }
      let reading = '';
      for (const r of rubies) for (const rt of r.querySelectorAll('rt')) reading += rt.textContent;
      const exempt = !base || !/[ぁ-ゖ]/.test(reading);
      rubyGroups.push({ rubies, base, exempt, following: exempt ? '' : followingText(rubies[rubies.length - 1]) });
    }
    return rubyGroups.map((g) => ({ base: g.base, following: g.following, exempt: g.exempt }));
  }

  // hide[i] is true to hide group i's readings (visibility: hidden, so layout
  // is identical in every mode).
  function applyFurigana(hide) {
    const style = document.createElement('style');
    style.textContent = 'ruby.tg-furigana-hidden rt, ruby.tg-furigana-hidden rtc, ruby.tg-furigana-hidden rp { visibility: hidden !important }';
    document.documentElement.appendChild(style);
    rubyGroups.forEach((g, i) => g.rubies.forEach((r) => r.classList.toggle('tg-furigana-hidden', !!hide[i])));
  }

  // --- Text, glyphs and occupied bands ------------------------------------

  function unionRect(rects) {
    let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (const r of rects) {
      if (r.width === 0 && r.height === 0) continue;
      x0 = Math.min(x0, r.left); y0 = Math.min(y0, r.top);
      x1 = Math.max(x1, r.right); y1 = Math.max(y1, r.bottom);
    }
    return x0 === Infinity ? null : [x0, y0, x1 - x0, y1 - y0];
  }

  // Walk the document in order, building the furigana-free chapter text (code
  // points, '\n' between blocks) with one glyph box per character. Also
  // returns horizontal extents of everything drawn (glyphs, ruby, images), so
  // the paginator can cut pages only in empty gaps, and illustration markers.
  function collect() {
    const chars = [];   // code points
    const glyphs = [];  // [offset, x, y, w, h]
    const ids = {};     // element id -> text offset
    const markers = []; // [index, x, offset]
    const bands = [];   // [x0, x1]
    const inlineImages = []; // {src, box}
    let pendingBreak = false;
    const range = document.createRange();

    const breakHere = () => {
      if (chars[chars.length - 1] === ' ') chars.pop(); // collapsed space at a block end
      if (chars.length && chars[chars.length - 1] !== '\n') pendingBreak = true;
    };
    const flush = () => { if (pendingBreak) { chars.push('\n'); pendingBreak = false; } };

    // Bands use each character's em box (column centre +- half the font size),
    // not its client rect: client rects span the font's full ascent+descent,
    // so they and the ruby boxes bridge the gaps between columns and would
    // merge neighbouring columns into one uncuttable band.
    const fontSizes = new Map();
    // A ruby band is extended leftward (vertical-rl puts ruby on the right of
    // its base) by one ruby em, into its base column, so the two always merge:
    // the ruby em box alone may not quite touch the base, and the paginator
    // would then be free to cut between a word and its furigana.
    const emBand = (el, box, isRuby = false) => {
      let fs = fontSizes.get(el);
      if (fs === undefined) { fs = parseFloat(getComputedStyle(el).fontSize) || box[2]; fontSizes.set(el, fs); }
      const cx = box[0] + box[2] / 2;
      bands.push([cx - fs / 2 - (isRuby ? fs : 0), cx + fs / 2]);
    };
    for (const el of document.body.querySelectorAll('rt')) {
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      for (let t = walker.nextNode(); t; t = walker.nextNode()) {
        let j = 0;
        for (const ch of t.nodeValue) {
          range.setStart(t, j); range.setEnd(t, j + ch.length); j += ch.length;
          const box = unionRect(range.getClientRects());
          if (box && !ASCII_WS.test(ch)) emBand(t.parentElement, box, true);
        }
      }
    }

    function walk(node) {
      if (node.nodeType === Node.ELEMENT_NODE) {
        const tag = node.localName;
        if (SKIP_TAGS.has(tag)) return;
        if (node.hasAttribute('data-tg-illus')) {
          breakHere();
          const r = node.getBoundingClientRect();
          markers.push([Number(node.getAttribute('data-tg-illus')), r.left, chars.length + (pendingBreak ? 1 : 0)]);
          return;
        }
        if (tag === 'br') { breakHere(); return; }
        if (tag === 'img' || tag === 'svg') {
          const r = node.getBoundingClientRect();
          if (r.width > 0) {
            bands.push([r.left, r.right]);
            const src = imageSource(tag === 'svg' ? (node.querySelector('image') || node) : node);
            inlineImages.push({ src, box: [r.left, r.top, r.width, r.height] });
          }
          return;
        }
        const block = isBlock(node);
        if (block) breakHere();
        if (node.id && !(node.id in ids)) ids[node.id] = chars.length + (pendingBreak ? 1 : 0);
        for (const child of node.childNodes) walk(child);
        if (block) breakHere();
        return;
      }
      if (node.nodeType !== Node.TEXT_NODE) return;
      const s = node.nodeValue;
      let i = 0;
      for (const ch of s) {
        const n = ch.length; // 2 for surrogate pairs
        range.setStart(node, i);
        range.setEnd(node, i + n);
        i += n;
        const box = unionRect(range.getClientRects());
        if (ASCII_WS.test(ch)) {
          // collapsible whitespace: keep one space, only where it was rendered
          const last = chars[chars.length - 1];
          if (!box || pendingBreak || !chars.length || last === ' ' || last === '\n') continue;
          chars.push(' ');
          continue;
        }
        if (!box) continue; // not rendered
        flush();
        glyphs.push([chars.length, box[0], box[1], box[2], box[3]]);
        emBand(node.parentElement, box);
        chars.push(ch);
      }
    }
    walk(document.body);
    while (chars.length && (chars[chars.length - 1] === '\n' || chars[chars.length - 1] === ' ')) chars.pop();
    return { text: chars.join(''), glyphs, ids, markers, bands, inlineImages };
  }

  window.TG = { extractIllustrations, collectRubyGroups, applyFurigana, collect };
})();
