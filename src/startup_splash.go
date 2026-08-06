// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type startupSplashState struct {
	Version   string `json:"version"`
	Percent   int    `json:"percent"`
	Stage     string `json:"stage"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
	Done      bool   `json:"done"`
	TargetURL string `json:"target_url,omitempty"`
	LogPath   string `json:"log_path"`
}

type startupSplash struct {
	mu       sync.RWMutex
	state    startupSplashState
	token    string
	baseURL  string
	server   *http.Server
	listener net.Listener
	actions  chan string
	cfg      Config
}

func randomStartupToken() (string, error) {
	data := make([]byte, 24)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func startStartupSplash(cfg Config, appVersion string) (*startupSplash, error) {
	token, err := randomStartupToken()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &startupSplash{
		token:    token,
		listener: listener,
		actions:  make(chan string, 2),
		cfg:      cfg,
		state: startupSplashState{
			Version: appVersion,
			Percent: 1,
			Stage: localizeConfigText(cfg,
				"LocalCode wird gestartet …",
				"Starting LocalCode …"),
			LogPath: logPath(),
		},
	}
	s.baseURL = "http://" + listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handlePage)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/action", s.handleAction)
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		if serveErr := s.server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			s.Fail(serveErr)
		}
	}()
	return s, nil
}

func (s *startupSplash) URL() string {
	if s == nil {
		return ""
	}
	return s.baseURL + "/?token=" + s.token
}

func (s *startupSplash) authorized(r *http.Request) bool {
	if s == nil || r == nil {
		return false
	}
	if !loopbackRequestHost(r.Host) {
		return false
	}
	provided := strings.TrimSpace(r.URL.Query().Get("token"))
	if provided == "" {
		provided = strings.TrimSpace(r.Header.Get("X-LocalCode-Startup-Token"))
	}
	return provided != "" && provided == s.token
}

func (s *startupSplash) handlePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || !s.authorized(r) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	data := struct {
		Token   string
		German  bool
		Version string
		LogPath string
	}{
		Token:   s.token,
		German:  strings.HasPrefix(strings.ToLower(resolvedLanguage(s.cfg)), "de"),
		Version: s.state.Version,
		LogPath: logPath(),
	}
	_ = startupPageTemplate.Execute(w, data)
}

func (s *startupSplash) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !s.authorized(r) {
		http.NotFound(w, r)
		return
	}
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(state)
}

func (s *startupSplash) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.authorized(r) {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	switch action {
	case "retry", "continue", "exit":
		select {
		case s.actions <- action:
		default:
		}
	case "open-log":
		if err := openProjectTarget(filepath.Dir(logPath()), "explorer", s.cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *startupSplash) Update(progress BootstrapProgress) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if progress.Percent < s.state.Percent && s.state.Error == "" {
		progress.Percent = s.state.Percent
	}
	if progress.Percent < 0 {
		progress.Percent = 0
	}
	if progress.Percent > 100 {
		progress.Percent = 100
	}
	s.state.Percent = progress.Percent
	s.state.Stage = strings.TrimSpace(progress.Stage)
	s.state.Detail = strings.TrimSpace(progress.Detail)
	s.state.Error = ""
	s.state.Done = false
	s.state.TargetURL = ""
	s.mu.Unlock()
}

func (s *startupSplash) Reset() {
	if s == nil {
		return
	}
	s.Update(BootstrapProgress{
		Percent: 1,
		Stage: localizeConfigText(s.cfg,
			"Einrichtung wird erneut gestartet …",
			"Restarting setup …"),
	})
}

func (s *startupSplash) Fail(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	s.state.Error = err.Error()
	s.state.Stage = localizeConfigText(s.cfg,
		"Die automatische Einrichtung benötigt Ihre Entscheidung.",
		"Automatic setup needs your decision.")
	s.state.Detail = localizeConfigText(s.cfg,
		"Sie können den Vorgang erneut versuchen, den Log-Ordner öffnen oder LocalCode im eingeschränkten Modus starten.",
		"You can retry, open the log folder, or start LocalCode in limited mode.")
	s.state.Done = false
	s.state.TargetURL = ""
	s.mu.Unlock()
}

func (s *startupSplash) Complete(targetURL string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.state.Percent = 100
	s.state.Stage = localizeConfigText(s.cfg, "LocalCode ist bereit.", "LocalCode is ready.")
	s.state.Detail = localizeConfigText(s.cfg, "Die Anwendung wird geöffnet …", "Opening the application …")
	s.state.Error = ""
	s.state.Done = true
	s.state.TargetURL = strings.TrimSpace(targetURL)
	s.mu.Unlock()
}

func (s *startupSplash) WaitAction(ctx context.Context) string {
	if s == nil {
		return "exit"
	}
	select {
	case action := <-s.actions:
		return action
	case <-ctx.Done():
		return "exit"
	}
}

func (s *startupSplash) Close() {
	if s == nil || s.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
}

var startupPageTemplate = template.Must(template.New("startup").Parse(`<!doctype html>
<html lang="{{if .German}}de{{else}}en{{end}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>LocalCode</title>
<style>
:root{color-scheme:dark;font-family:"Segoe UI",system-ui,sans-serif;background:#101417;color:#eef2f4}
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:radial-gradient(circle at 25% 10%,#26343b 0,#151b1f 35%,#0d1113 78%);overflow:hidden}
.shell{width:min(720px,calc(100vw - 34px));border:1px solid #344149;border-radius:22px;background:rgba(25,31,35,.96);box-shadow:0 30px 90px #000b;padding:34px 38px 30px;position:relative;overflow:hidden}
.glow{position:absolute;width:320px;height:320px;border-radius:50%;background:#2f81f722;filter:blur(60px);right:-150px;top:-170px}.brand{display:flex;align-items:center;gap:16px;position:relative}.logo{width:58px;height:58px;border-radius:16px;background:linear-gradient(145deg,#438ee8,#215cb3);display:grid;place-items:center;font-weight:800;font-size:23px;box-shadow:inset 0 1px #ffffff44,0 10px 30px #1e67c255}.brand h1{font-size:25px;margin:0 0 3px}.version{color:#98a4aa;font-size:13px}.stage{font-size:21px;font-weight:650;margin:34px 0 8px;min-height:29px}.detail{color:#aeb8bd;font-size:14px;line-height:1.55;min-height:44px;white-space:pre-wrap;overflow-wrap:anywhere}.track{height:10px;border-radius:99px;background:#0d1113;border:1px solid #303a40;overflow:hidden;margin-top:22px}.bar{height:100%;width:1%;background:linear-gradient(90deg,#2f81f7,#60a5fa);border-radius:99px;transition:width .35s ease}.meta{display:flex;justify-content:space-between;color:#7f8a90;font-size:12px;margin-top:8px}.error{display:none;margin-top:20px;padding:14px 15px;border:1px solid #7c3535;background:#351d1d;border-radius:11px;color:#ffd2d2;white-space:pre-wrap;overflow-wrap:anywhere;max-height:180px;overflow:auto}.actions{display:none;flex-wrap:wrap;gap:9px;margin-top:18px}.actions button{border:1px solid #46535a;background:#242d32;color:#edf2f4;border-radius:9px;padding:9px 13px;font-weight:600;cursor:pointer}.actions button.primary{background:#2f81f7;border-color:#2f81f7}.actions button.danger{margin-left:auto;color:#ffcdcd}.foot{margin-top:28px;padding-top:18px;border-top:1px solid #30383d;color:#737e84;font:11px/1.5 Consolas,monospace;overflow-wrap:anywhere}
.spinner{width:18px;height:18px;border:2px solid #ffffff2b;border-top-color:#fff;border-radius:50%;animation:spin .8s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}
</style>
</head>
<body><main class="shell"><div class="glow"></div><div class="brand"><div class="logo">LC</div><div><h1>LocalCode</h1><div class="version">Version {{.Version}}</div></div><div class="spinner" id="spinner" style="margin-left:auto"></div></div>
<div class="stage" id="stage">{{if .German}}LocalCode wird gestartet …{{else}}Starting LocalCode …{{end}}</div>
<div class="detail" id="detail">{{if .German}}Die lokale Entwicklungsumgebung wird geprüft.{{else}}Checking the local development environment.{{end}}</div>
<div class="track"><div class="bar" id="bar"></div></div><div class="meta"><span id="percent">1 %</span><span>{{if .German}}Bitte dieses Fenster geöffnet lassen{{else}}Keep this window open{{end}}</span></div>
<div class="error" id="error"></div>
<div class="actions" id="actions"><button class="primary" data-action="retry">{{if .German}}Erneut versuchen{{else}}Retry{{end}}</button><button data-action="open-log">{{if .German}}Log-Ordner öffnen{{else}}Open log folder{{end}}</button><button data-action="continue">{{if .German}}Eingeschränkt starten{{else}}Start in limited mode{{end}}</button><button class="danger" data-action="exit">{{if .German}}Beenden{{else}}Exit{{end}}</button></div>
<div class="foot">Log: {{.LogPath}}</div></main>
<script>
const token={{.Token}};let redirecting=false;
const el=id=>document.getElementById(id);
async function state(){const r=await fetch('/api/state?token='+encodeURIComponent(token),{cache:'no-store'});if(!r.ok)throw new Error('HTTP '+r.status);return r.json()}
async function action(name){await fetch('/api/action?token='+encodeURIComponent(token),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:name})});if(name==='retry'){el('actions').style.display='none';el('error').style.display='none';el('spinner').style.display='block'}}
document.querySelectorAll('[data-action]').forEach(b=>b.onclick=()=>action(b.dataset.action));
async function poll(){try{const s=await state();const p=Math.max(0,Math.min(100,Number(s.percent)||0));el('bar').style.width=p+'%';el('percent').textContent=p+' %';el('stage').textContent=s.stage||'';el('detail').textContent=s.detail||'';if(s.error){el('error').textContent=s.error;el('error').style.display='block';el('actions').style.display='flex';el('spinner').style.display='none'}else{el('error').style.display='none';el('actions').style.display='none';el('spinner').style.display='block'}if(s.done&&s.target_url&&!redirecting){redirecting=true;try{window.moveTo(0,0);window.resizeTo(screen.availWidth,screen.availHeight)}catch(_){}location.replace(s.target_url)}}catch(e){el('detail').textContent=e.message}finally{if(!redirecting)setTimeout(poll,350)}}poll();
</script></body></html>`))

func (s startupSplashState) String() string {
	return fmt.Sprintf("%d%% %s %s", s.Percent, s.Stage, s.Detail)
}
