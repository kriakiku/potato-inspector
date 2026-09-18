package caportal

import (
	"html/template"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/potatoinspector/potato-inspector/internal/dnsfwd"
	"github.com/potatoinspector/potato-inspector/internal/mitm"
)

type Server struct {
	mu      sync.Mutex
	mitm    *mitm.Manager
	httpSrv *http.Server
	enabled bool
}

func New(mm *mitm.Manager) *Server {
	return &Server{mitm: mm}
}

func (s *Server) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/ca.crt", s.handleCA)
	mux.HandleFunc("/favicon.svg", s.handleFavicon)
	ln, err := net.Listen("tcp", ":80")
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.httpSrv = srv
	s.enabled = true
	go func() {
		_ = srv.Serve(ln)
	}()
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled || s.httpSrv == nil {
		return
	}
	_ = s.httpSrv.Close()
	s.enabled = false
}

func (s *Server) handleCA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	path, _, err := s.mitm.EnsureCA()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="potatoinspector-ca.crt"`)
	_, _ = w.Write(data)
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(faviconSVG))
}

type pageData struct {
	Host         string
	ShareHost    string
	OS           string // android|ios|macos|windows|unknown
	ShowAll      bool
	TitleOS      string
	ForceInstall bool
}

func detectOS(ua string) string {
	u := strings.ToLower(ua)
	switch {
	case strings.Contains(u, "android"):
		return "android"
	case strings.Contains(u, "iphone") || strings.Contains(u, "ipad") || strings.Contains(u, "ipod"):
		return "ios"
	case strings.Contains(u, "mac os") || strings.Contains(u, "macintosh"):
		return "macos"
	case strings.Contains(u, "windows"):
		return "windows"
	default:
		return "unknown"
	}
}

func osTitle(os string) string {
	switch os {
	case "android":
		return "Android"
	case "ios":
		return "iOS / iPadOS"
	case "macos":
		return "macOS"
	case "windows":
		return "Windows"
	default:
		return "your device"
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	force := r.URL.Query().Get("os")
	osName := force
	if osName == "" {
		osName = detectOS(r.UserAgent())
	}
	showAll := osName == "unknown" || osName == "all"
	if showAll {
		osName = "unknown"
	}
	install := r.URL.Query().Get("install") == "1" || r.URL.Query().Get("install") == "true"
	data := pageData{
		Host:         dnsfwd.PortalHost,
		ShareHost:    dnsfwd.ShareHost,
		OS:           osName,
		ShowAll:      showAll,
		TitleOS:      osTitle(osName),
		ForceInstall: install,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = indexTmpl.Execute(w, data)
}

var indexTmpl = template.Must(template.New("index").Parse(indexHTML))

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>PotatoInspector · {{.Host}}</title>
<link rel="icon" href="/favicon.svg" type="image/svg+xml"/>
<link rel="preconnect" href="https://fonts.googleapis.com"/>
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin/>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet"/>
<style>
  :root {
    --bg0: #0f1410; --bg1: #17201a; --bg2: #1e2a22;
    --text: #e8efe6; --muted: #8fa094; --line: #2d3d32;
    --accent: #c4e86a; --link: #9ec4e8;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; min-height: 100vh;
    font-family: "IBM Plex Sans", system-ui, sans-serif;
    color: var(--text);
    background-color: var(--bg0);
    background-image:
      radial-gradient(1200px 600px at 10% -10%, #1a2e20 0%, transparent 55%),
      radial-gradient(900px 500px at 100% 0%, #243018 0%, transparent 50%);
    background-repeat: no-repeat;
    background-attachment: fixed;
    line-height: 1.5;
  }
  .wrap { max-width: 36rem; margin: 0 auto; padding: 1.5rem 1.15rem 2.5rem; }
  .wrap.share-mode { max-width: 48rem; display: flex; flex-direction: column; min-height: calc(100vh - 3rem); }
  .brand {
    display: flex; align-items: center; gap: 0.4rem;
    font-weight: 600; font-size: 0.95rem; letter-spacing: -0.02em;
    color: var(--accent); margin: 0 0 1rem;
  }
  .brand-logo { font-size: 1.15rem; line-height: 1; filter: saturate(1.1); }
  .brand-host { color: var(--muted); font-weight: 500; font-size: 0.85rem; }
  h1 { font-size: 1.55rem; font-weight: 650; margin: 0 0 0.5rem; letter-spacing: -0.02em; }
  .lead { color: var(--muted); margin: 0 0 1.5rem; font-size: 0.95rem; }
  .dl {
    display: flex; flex-direction: column; align-items: center; gap: 0.75rem;
    padding: 1.5rem 1rem; margin-bottom: 1.5rem;
    background: var(--bg1); border: 1px solid var(--line); border-radius: 12px;
    text-decoration: none; color: var(--text);
  }
  .dl:hover { border-color: var(--accent); }
  .dl-icon {
    width: 4.5rem; height: 4.5rem; border-radius: 1rem;
    background: color-mix(in srgb, var(--accent) 18%, var(--bg2));
    display: flex; align-items: center; justify-content: center;
  }
  .dl-icon svg { width: 2.4rem; height: 2.4rem; fill: var(--accent); }
  .dl-label { font-size: 1.15rem; font-weight: 600; }
  .dl-sub { font-size: 0.85rem; color: var(--muted); }
  h2 { font-size: 1.05rem; margin: 0 0 0.65rem; }
  ol { margin: 0 0 1.25rem; padding-left: 1.25rem; }
  li { margin-bottom: 0.45rem; font-size: 0.92rem; }
  .note { font-size: 0.85rem; color: var(--muted); margin: 0 0 1rem; }
  .os-nav { display: flex; flex-wrap: wrap; gap: 0.4rem; margin-bottom: 1.25rem; }
  .os-nav a {
    font-size: 0.8rem; padding: 0.3rem 0.65rem; border-radius: 6px;
    background: var(--bg2); color: var(--link); text-decoration: none;
  }
  .os-nav a:hover { color: var(--text); }
  .section { display: none; }
  .section.on { display: block; }
  .all .section { display: block; margin-bottom: 1.5rem; padding-bottom: 1rem; border-bottom: 1px solid var(--line); }
  code { font-family: ui-monospace, monospace; font-size: 0.88em; background: var(--bg2); padding: 0.1em 0.35em; border-radius: 4px; }
  .probe-status { font-size: 0.85rem; color: var(--muted); margin: 0 0 1rem; }
  .probe-status.ok { color: var(--accent); }
  .hidden { display: none !important; }
  .share-toolbar { margin-bottom: 0.75rem; }
  .share-actions { display: flex; flex-wrap: wrap; gap: 0.5rem; align-items: center; margin: 0.75rem 0 0; }
  .share-actions button {
    font: inherit; font-size: 0.9rem; padding: 0.45rem 0.85rem;
    border-radius: 8px; border: 1px solid var(--line);
    background: var(--bg2); color: var(--text); cursor: pointer;
  }
  .share-actions button:hover { border-color: var(--accent); }
  .share-actions button.primary {
    background: color-mix(in srgb, var(--accent) 22%, var(--bg2));
    border-color: color-mix(in srgb, var(--accent) 45%, var(--line));
  }
  .share-meta { margin-left: auto; font-family: ui-monospace, monospace; font-size: 0.8rem; color: var(--muted); }
  .share-textarea {
    flex: 1; width: 100%; min-height: 16rem;
    font-family: ui-monospace, monospace; font-size: 0.9rem;
    padding: 0.85rem; border-radius: 10px;
    border: 1px solid var(--line); background: var(--bg1); color: var(--text);
    resize: vertical; line-height: 1.45;
  }
  .share-textarea:focus { outline: 2px solid color-mix(in srgb, var(--accent) 40%, transparent); outline-offset: 1px; }
  .share-footer { margin-top: 0.75rem; font-size: 0.8rem; }
  .share-footer a { color: var(--link); }
</style>
</head>
<body>
<div class="wrap{{if .ShowAll}} all{{end}}" id="wrap">
  <div class="brand">
    <span class="brand-logo" aria-hidden="true">🥔</span>
    PotatoInspector
    <span class="brand-host">· {{.Host}}</span>
  </div>

  <div id="install">
    <h1>Install the MITM root CA</h1>
    <p class="lead">
      {{if .ShowAll}}Pick your platform below, download the certificate, then follow the steps.
      {{else}}Detected <strong>{{.TitleOS}}</strong>. Download the certificate, then follow the steps.
      {{end}}
      After the CA is trusted, this page switches to the live Share notepad.
    </p>
    <p class="probe-status" id="probe-status">Checking HTTPS trust…</p>

    <a class="dl" href="/ca.crt" download="potatoinspector-ca.crt">
      <span class="dl-icon" aria-hidden="true">
        <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="M12 3a1 1 0 0 1 1 1v9.59l2.3-2.3a1 1 0 1 1 1.4 1.42l-4 4a1 1 0 0 1-1.4 0l-4-4a1 1 0 1 1 1.4-1.42L11 13.59V4a1 1 0 0 1 1-1zm-7 14a1 1 0 0 1 1 1v1h12v-1a1 1 0 1 1 2 0v2a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1v-2a1 1 0 0 1 1-1z"/></svg>
      </span>
      <span class="dl-label">Download CA certificate</span>
      <span class="dl-sub">potatoinspector-ca.crt</span>
    </a>

    <div class="os-nav">
      <a href="/?os=android&amp;install=1">Android</a>
      <a href="/?os=ios&amp;install=1">iOS</a>
      <a href="/?os=macos&amp;install=1">macOS</a>
      <a href="/?os=windows&amp;install=1">Windows</a>
      <a href="/?os=all&amp;install=1">Show all</a>
    </div>

    <div class="section{{if or (eq .OS "android") .ShowAll}} on{{end}}" id="android">
      <h2>Android</h2>
      <ol>
        <li>Tap <strong>Download CA certificate</strong> above (or open the file from Downloads).</li>
        <li>Go to <strong>Settings → Security → Encryption &amp; credentials → Install a certificate → CA certificate</strong> (wording varies by OEM).</li>
        <li>Confirm the warning — expected for this lab.</li>
        <li>Name it e.g. <code>PotatoInspector</code>. It should appear under <strong>User</strong> trusted credentials.</li>
      </ol>
      <p class="note">Some apps ignore user CAs. Put those hosts on the Ignore list in the panel.</p>
    </div>

    <div class="section{{if or (eq .OS "ios") .ShowAll}} on{{end}}" id="ios">
      <h2>iOS / iPadOS</h2>
      <ol>
        <li>Tap <strong>Download CA certificate</strong> and allow the profile download.</li>
        <li><strong>Settings → General → VPN &amp; Device Management</strong> → install the downloaded profile.</li>
        <li><strong>Settings → General → About → Certificate Trust Settings</strong>.</li>
        <li>Enable <strong>Full Trust</strong> for the PotatoInspector root. Confirm the warning.</li>
      </ol>
      <p class="note">Without Full Trust, Safari and apps will still reject MITM certificates.</p>
    </div>

    <div class="section{{if or (eq .OS "macos") .ShowAll}} on{{end}}" id="macos">
      <h2>macOS</h2>
      <ol>
        <li>Click <strong>Download CA certificate</strong>.</li>
        <li>Open the file in <strong>Keychain Access</strong> (login or System keychain).</li>
        <li>Double-click the cert → <strong>Trust</strong> → set <strong>When using this certificate</strong> to <strong>Always Trust</strong>.</li>
        <li>Close and authenticate if prompted. Restart the browser if needed.</li>
      </ol>
    </div>

    <div class="section{{if or (eq .OS "windows") .ShowAll}} on{{end}}" id="windows">
      <h2>Windows</h2>
      <ol>
        <li>Click <strong>Download CA certificate</strong>.</li>
        <li>Run <code>certmgr.msc</code> (or open the .crt → Install Certificate).</li>
        <li>Import into <strong>Trusted Root Certification Authorities</strong>.</li>
        <li>Accept the security warning. Restart the browser if HTTPS still fails.</li>
      </ol>
    </div>
  </div>

  <div id="share" class="hidden">
    <div class="share-toolbar">
      <h1>Share</h1>
      <p class="lead" style="margin-bottom:0">Live notepad — edits sync across tunnel clients (last write wins).</p>
      <div class="share-actions">
        <button type="button" id="btn-clear">Clear</button>
        <button type="button" id="btn-copy">Copy</button>
        <button type="button" class="primary" id="btn-paste">Paste</button>
        <span class="share-meta" id="share-meta">v0</span>
      </div>
    </div>
    <textarea class="share-textarea" id="share-text" placeholder="Loading…" spellcheck="false" disabled></textarea>
    <p class="share-footer note">
      CA install guide: <a href="/?install=1">show again</a>
      · API <code>https://{{.ShareHost}}</code>
    </p>
  </div>
</div>
<script>
(function () {
  var SHARE_BASE = "https://{{.ShareHost}}";
  var FORCE_INSTALL = {{if .ForceInstall}}true{{else}}false{{end}};
  var DEBOUNCE_MS = 300;
  var installEl = document.getElementById("install");
  var shareEl = document.getElementById("share");
  var wrap = document.getElementById("wrap");
  var probeEl = document.getElementById("probe-status");
  var textEl = document.getElementById("share-text");
  var metaEl = document.getElementById("share-meta");
  var version = 0;
  var applyingRemote = false;
  var debounceTimer = null;
  var es = null;
  var shareReady = false;

  function showInstall() {
    installEl.classList.remove("hidden");
    shareEl.classList.add("hidden");
    wrap.classList.remove("share-mode");
  }
  function showShare() {
    installEl.classList.add("hidden");
    shareEl.classList.remove("hidden");
    wrap.classList.add("share-mode");
  }

  function setProbe(msg, ok) {
    if (!probeEl) return;
    probeEl.textContent = msg;
    probeEl.className = "probe-status" + (ok ? " ok" : "");
  }

  async function probe() {
    try {
      var res = await fetch(SHARE_BASE + "/health", { method: "GET", cache: "no-store" });
      if (!res.ok) throw new Error("status " + res.status);
      return true;
    } catch (e) {
      return false;
    }
  }

  function pushText(next) {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(async function () {
      try {
        var res = await fetch(SHARE_BASE + "/api/share", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ text: next })
        });
        if (!res.ok) throw new Error("save failed");
        var snap = await res.json();
        version = snap.version || 0;
        metaEl.textContent = "v" + version;
      } catch (e) { /* keep local */ }
    }, DEBOUNCE_MS);
  }

  function bootShare() {
    if (shareReady) return;
    shareReady = true;
    showShare();
    fetch(SHARE_BASE + "/api/share", { cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (snap) {
        version = snap.version || 0;
        metaEl.textContent = "v" + version;
        textEl.value = snap.text || "";
        textEl.disabled = false;
        textEl.placeholder = "Type here…";
      })
      .catch(function () {
        textEl.placeholder = "Could not load share pad";
      });

    if (es) es.close();
    es = new EventSource(SHARE_BASE + "/api/share/stream");
    es.onmessage = function (ev) {
      try {
        var snap = JSON.parse(ev.data);
        if ((snap.version || 0) <= version) return;
        version = snap.version;
        metaEl.textContent = "v" + version;
        applyingRemote = true;
        textEl.value = snap.text || "";
        queueMicrotask(function () { applyingRemote = false; });
      } catch (e) {}
    };
  }

  textEl.addEventListener("input", function () {
    if (applyingRemote) return;
    pushText(textEl.value);
  });
  document.getElementById("btn-clear").addEventListener("click", function () {
    textEl.value = "";
    pushText("");
  });
  document.getElementById("btn-copy").addEventListener("click", function () {
    navigator.clipboard.writeText(textEl.value).catch(function () {});
  });
  document.getElementById("btn-paste").addEventListener("click", function () {
    navigator.clipboard.readText().then(function (clip) {
      textEl.value = clip;
      pushText(clip);
    }).catch(function () {});
  });

  async function tick() {
    var ok = await probe();
    if (ok) {
      setProbe("CA trusted — Share is ready.", true);
      if (!FORCE_INSTALL) bootShare();
      else setProbe("CA trusted. Close install view or open / without ?install=1 for Share.", true);
      return;
    }
    setProbe("Waiting for CA trust… probing https://{{.ShareHost}}", false);
    showInstall();
    setTimeout(tick, 4000);
  }

  if (FORCE_INSTALL) {
    showInstall();
    tick();
  } else {
    showInstall();
    tick();
  }
})();
</script>
</body>
</html>
`

// Same potato mark as web/public/favicon.svg
const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <ellipse cx="16" cy="17" rx="11" ry="9.5" fill="#c4a35a"/>
  <ellipse cx="16" cy="16.5" rx="10" ry="8.5" fill="#d4b86a"/>
  <ellipse cx="12" cy="13" rx="2.2" ry="1.6" fill="#b8954a" opacity=".55"/>
  <ellipse cx="20" cy="14.5" rx="1.6" ry="1.2" fill="#b8954a" opacity=".5"/>
  <ellipse cx="14.5" cy="20" rx="1.8" ry="1.3" fill="#b8954a" opacity=".45"/>
  <ellipse cx="21" cy="19" rx="1.3" ry="1" fill="#b8954a" opacity=".4"/>
  <circle cx="11.5" cy="12.5" r=".7" fill="#6b4a28"/>
  <circle cx="19.5" cy="13.8" r=".55" fill="#6b4a28"/>
  <circle cx="14" cy="19.2" r=".6" fill="#6b4a28"/>
  <circle cx="20.5" cy="18.5" r=".5" fill="#6b4a28"/>
  <path d="M14 7.2c.4-1.6 1.6-2.6 2.8-2.4.3 1.4-.4 2.6-1.4 3.1-.6.3-1.3.1-1.4-.7z" fill="#5a8f3a"/>
  <path d="M16.2 5.2c.9-.2 1.8.3 2.1 1.2-.7.5-1.5.5-2.1 0V5.2z" fill="#4a7a30"/>
</svg>
`
