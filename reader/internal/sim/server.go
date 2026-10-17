// Package sim is the desktop simulator (plan §6.2): a local web page that
// shows the framebuffer and turns mouse presses into touch gestures, so the
// whole reader runs without a Kindle.
package sim

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
)

// Server serves the simulator page.
type Server struct {
	Disp     *display.Sim
	Gestures chan input.Gesture
	Wake     chan struct{}
}

// New creates a server around a simulated display.
func New(disp *display.Sim) *Server {
	return &Server{Disp: disp, Gestures: make(chan input.Gesture, 16), Wake: make(chan struct{}, 1)}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /screen.png", s.screen)
	mux.HandleFunc("GET /wait", s.wait)
	mux.HandleFunc("POST /touch", s.touch)
	mux.HandleFunc("POST /wake", func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.Wake <- struct{}{}:
		default:
		}
	})
	return mux
}

func (s *Server) screen(w http.ResponseWriter, r *http.Request) {
	png, v := s.Disp.Snapshot()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Version", strconv.Itoa(v))
	w.Write(png)
}

// wait long-polls until the framebuffer version passes ?v=.
func (s *Server) wait(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.Atoi(r.URL.Query().Get("v"))
	for s.Disp.Version() <= since {
		select {
		case <-s.Disp.Changed():
		case <-time.After(25 * time.Second):
			fmt.Fprint(w, s.Disp.Version())
			return
		case <-r.Context().Done():
			return
		}
	}
	fmt.Fprint(w, s.Disp.Version())
}

type touchReq struct {
	X0, Y0, X1, Y1 int
	Ms             int
}

func (s *Server) touch(w http.ResponseWriter, r *http.Request) {
	var t touchReq
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.Gestures <- input.Classify(input.Touch{X0: t.X0, Y0: t.Y0, X1: t.X1, Y1: t.Y1,
		RawX0: t.X0, RawY0: t.Y0, Duration: time.Duration(t.Ms) * time.Millisecond})
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	size := s.Disp.Size()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, page, size.X, size.Y)
}

const page = `<!doctype html><html><head><meta charset="utf-8"><title>Tategaki simulator</title>
<style>
 body { margin: 0; background: #555; display: flex; flex-direction: column; align-items: center; font: 14px system-ui; color: #eee; }
 #screen { height: calc(100vh - 48px); aspect-ratio: %[1]d / %[2]d; background: #fff; cursor: crosshair; user-select: none; -webkit-user-drag: none; image-rendering: auto; }
 #bar { height: 40px; display: flex; gap: 12px; align-items: center; }
 button { font: inherit; }
</style></head><body>
<img id="screen" src="/screen.png" draggable="false">
<div id="bar"><span>Click = tap · hold 0.6 s = hold · drag = swipe</span>
<button onclick="fetch('/wake',{method:'POST'})">Simulate wake</button><span id="v"></span></div>
<script>
const W = %[1]d, H = %[2]d, img = document.getElementById('screen');
let version = 0, down = null;
function pt(e) { const r = img.getBoundingClientRect(); return [Math.round((e.clientX - r.left) * W / r.width), Math.round((e.clientY - r.top) * H / r.height)]; }
img.addEventListener('pointerdown', e => { e.preventDefault(); down = {p: pt(e), t: performance.now()}; });
window.addEventListener('pointerup', e => {
  if (!down) return; const [x1, y1] = pt(e), [x0, y0] = down.p;
  const ms = Math.round(performance.now() - down.t); down = null;
  fetch('/touch', {method: 'POST', body: JSON.stringify({X0: x0, Y0: y0, X1: x1, Y1: y1, Ms: ms})});
});
async function poll() {
  for (;;) {
    try {
      const v = +(await (await fetch('/wait?v=' + version)).text());
      if (v !== version) { version = v; img.src = '/screen.png?v=' + v; document.getElementById('v').textContent = 'draw #' + v; }
    } catch (e) { await new Promise(r => setTimeout(r, 1000)); }
  }
}
poll();
</script></body></html>`
