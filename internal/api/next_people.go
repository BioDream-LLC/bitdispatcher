package api

// Cameras near an incident, and links that let a caller share their location (cad/cameras.go,
// cad/locationlink.go), and the board's view of the October 2026 features.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/assist"
	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

// NextView is what the board carries of the October 2026 features.
type NextView struct {
	Roads     roadsBrief                     `json:"roads"`
	Cameras   bool                           `json:"cameras"` // cameras are on and some are listed
	LocLinks  map[string][]*cad.LocationLink `json:"locLinks"`
	Intake    bool                           `json:"intake"`
	Languages []assist.Language              `json:"languages"`
}

func (s *Server) nextView(st *cad.State, calls []*cad.Call) NextView {
	v := NextView{LocLinks: map[string][]*cad.LocationLink{}, Languages: assist.Languages()}
	v.Cameras = st.Settings.Next.Cameras.On && len(st.Cameras) > 0
	v.Intake = st.Settings.Next.Intake.On
	for _, c := range calls {
		if l := st.LinksFor(c.ID); len(l) > 0 {
			cp := make([]*cad.LocationLink, len(l))
			for i, x := range l {
				xx := *x
				xx.Hash = ""
				cp[i] = &xx
			}
			v.LocLinks[c.ID] = cp
		}
	}
	return v
}

func (s *Server) nextRoutes() {
	s.setupRoutes()
	s.roadRoutes()
	s.cameraRoutes()
	s.hereRoutes()
	s.alarmRoutes()
	s.intakeRoutes()
}

// ---- cameras ----

func (s *Server) cameraRoutes() {
	s.handle("GET /api/admin/cameras", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		out := []*cad.Camera{}
		var pol cad.CameraPolicy
		s.Eng.Read(func(st *cad.State) {
			for _, c := range st.Cameras {
				cc := *c
				out = append(out, &cc)
			}
			pol = st.Settings.Next.Cameras
		})
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		writeJSON(w, 200, map[string]any{"items": out, "policy": pol})
	})
	s.handle("PUT /api/admin/cameras", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var c cad.Camera
		if !readJSON(w, r, &c) {
			return
		}
		id, err := s.Eng.SaveCamera(a, c)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"id": id})
	})
	s.handle("DELETE /api/admin/cameras/{id}", auth.Admin, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.RemoveCamera(a, r.PathValue("id")))
	})
	// Who opened which camera, and why: for the supervisor's review.
	s.handle("GET /api/admin/cameras/looks", auth.Supervisor, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var out []cad.CameraLook
		s.Eng.Read(func(st *cad.State) {
			for i := len(st.CameraLooks) - 1; i >= 0 && len(out) < 500; i-- {
				out = append(out, st.CameraLooks[i])
			}
		})
		if out == nil {
			out = []cad.CameraLook{}
		}
		writeJSON(w, 200, map[string]any{"items": out})
	})
	// The cameras near an incident (or a point), when cameras are on.
	s.handle("GET /api/cameras/near", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		q := r.URL.Query()
		var lat, lon float64
		var on, found bool
		s.Eng.Read(func(st *cad.State) {
			on = st.Settings.Next.Cameras.On
			if in := st.Incidents[q.Get("incident")]; in != nil && in.Location != nil && in.Location.Lat != nil && in.Location.Lon != nil {
				lat, lon, found = *in.Location.Lat, *in.Location.Lon, true
			}
		})
		if !found {
			if _, err := fmt.Sscanf(q.Get("lat")+" "+q.Get("lon"), "%g %g", &lat, &lon); err == nil {
				found = true
			}
		}
		if !on {
			writeJSON(w, 200, map[string]any{"on": false, "items": []any{}})
			return
		}
		if !found {
			fail(w, 422, "the incident has no map position")
			return
		}
		type item struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			Owner   string   `json:"owner,omitempty"`
			Lat     float64  `json:"lat"`
			Lon     float64  `json:"lon"`
			Heading *float64 `json:"heading,omitempty"`
			Metres  float64  `json:"metres"`
			Facing  *bool    `json:"facing,omitempty"`
			Live    bool     `json:"live"`
			Still   bool     `json:"still"`
			Note    string   `json:"note,omitempty"`
		}
		out := []item{}
		s.Eng.Read(func(st *cad.State) {
			for _, c := range st.CamerasNear(lat, lon) {
				out = append(out, item{c.ID, c.Name, c.Owner, c.Lat, c.Lon, c.Heading, c.Metres, c.Facing, c.Live != "", c.Still != "", c.Note})
			}
		})
		writeJSON(w, 200, map[string]any{"on": true, "items": out})
	})
	// Opening a camera: recorded with why; answers its live view's address.
	s.handle("POST /api/cameras/{id}/open", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Incident string `json:"incident"`
			Purpose  string `json:"purpose"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		c, err := s.Eng.LookAtCamera(a, r.PathValue("id"), b.Incident, b.Purpose)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"live": c.Live, "still": c.Still != ""})
	})
	// A camera's still picture, through bitdispatcher: only for someone who opened it in the last half hour.
	s.handle("GET /api/cameras/{id}/still", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		id := r.PathValue("id")
		var still string
		opened := false
		now := s.Eng.Now()
		s.Eng.Read(func(st *cad.State) {
			if c := st.Cameras[id]; c != nil && !c.Off && st.Settings.Next.Cameras.On {
				still = c.Still
			}
			for i := len(st.CameraLooks) - 1; i >= 0; i-- {
				l := st.CameraLooks[i]
				if now.Sub(l.At) > 30*time.Minute {
					break
				}
				if l.Camera == id && l.User == a.User {
					opened = true
					break
				}
			}
		})
		if still == "" {
			fail(w, 404, "no still picture for this camera")
			return
		}
		if !opened {
			fail(w, 403, "open the camera first (it asks why, for the record)")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, still, nil)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fail(w, 502, "the camera did not answer: "+err.Error())
			return
		}
		defer resp.Body.Close()
		ct := resp.Header.Get("Content-Type")
		if resp.StatusCode/100 != 2 || (ct != "image/jpeg" && ct != "image/png" && ct != "image/webp" && ct != "image/gif") {
			fail(w, 502, "the camera's picture was not an image ("+resp.Status+", "+ct+")")
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		io.Copy(w, io.LimitReader(resp.Body, 8<<20))
	})
}

// ---- location links ----

// hereRun is a live location link's rate limit.
type hereRun struct {
	last  time.Time
	fixes int
}

const hereMaxFixes = 900

func (s *Server) hereRoutes() {
	// The call-taker makes a link for a call, and texts it when the call can take a text.
	s.handle("POST /api/calls/{id}/location-link", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		var b struct {
			Send bool `json:"send"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		id := r.PathValue("id")
		var minutes int
		var text string
		s.Eng.Read(func(st *cad.State) {
			minutes, text = st.Settings.Next.LocLinkMinutes, st.Settings.Next.LocLinkText
		})
		if minutes <= 0 {
			minutes = 30
		}
		if text == "" || !strings.Contains(text, "{link}") {
			text = cad.DefaultLocLinkText
		}
		raw := make([]byte, 18)
		rand.Read(raw)
		secret := base64.RawURLEncoding.EncodeToString(raw)
		base := strings.TrimRight(s.pagingConfig().PublicURL, "/")
		if base == "" {
			base = "https://" + r.Host
		}
		link := base + "/here/" + secret
		sent := "read it to the caller"
		if b.Send {
			if err := s.bvFor(id).Text(r.Context(), id, "", strings.ReplaceAll(text, "{link}", link)); err != nil {
				sent = "not texted (" + err.Error() + "): read it to the caller"
			} else {
				sent = "texted"
			}
		}
		l, err := s.Eng.MakeLocationLink(a, id, cad.HashSecret(secret), time.Duration(minutes)*time.Minute, sent)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.Eng.RecordCallAction(a, id, "location link", true, sent)
		writeJSON(w, 200, map[string]any{"id": l.ID, "url": link, "expires": l.Expires, "sent": sent, "public": s.pagingConfig().PublicURL != ""})
	})
	s.handle("POST /api/location-links/{id}/end", auth.Dispatcher, func(w http.ResponseWriter, r *http.Request, se *auth.Session, a cad.Actor) {
		done(w, s.Eng.EndLocationLink(a, r.PathValue("id"), "ended by "+a.User))
	})
	// The caller's page: no sign-in; the secret in the address is the key.
	s.mux.HandleFunc("GET /here/{secret}", func(w http.ResponseWriter, r *http.Request) {
		l := s.hereLink(r.PathValue("secret"))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		if l == nil || !l.Live(s.Eng.Now()) {
			w.WriteHeader(http.StatusGone)
			io.WriteString(w, herePage("This link has ended", "If you still need help, call 911.", false))
			return
		}
		io.WriteString(w, herePage("Show 911 where you are", "Press Share and allow your location. Keep this page open: it keeps sending your position as you move. Stay on the line if you are on a call.", true))
	})
	s.mux.HandleFunc("GET /here-share.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		io.WriteString(w, hereScript)
	})
	s.mux.HandleFunc("POST /here/{secret}", func(w http.ResponseWriter, r *http.Request) {
		secret := r.PathValue("secret")
		l := s.hereLink(secret)
		now := s.Eng.Now()
		if l == nil || !l.Live(now) {
			fail(w, http.StatusGone, "this link has ended")
			return
		}
		var b struct {
			Lat      float64  `json:"lat"`
			Lon      float64  `json:"lon"`
			Accuracy float64  `json:"accuracy"`
			Altitude *float64 `json:"altitude"`
			AltAcc   float64  `json:"altitudeAccuracy"`
			Stop     bool     `json:"stop"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			fail(w, 400, "not a position")
			return
		}
		if b.Stop {
			s.Eng.EndLocationLink(cad.Actor{User: "caller"}, l.ID, "the caller stopped sharing")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if b.Lat < -90 || b.Lat > 90 || b.Lon < -180 || b.Lon > 180 || (b.Lat == 0 && b.Lon == 0) || b.Accuracy < 0 || b.Accuracy > 100000 {
			fail(w, 422, "not a position")
			return
		}
		key := cad.HashSecret(secret)
		s.next.mu.Lock()
		if s.next.here == nil {
			s.next.here = map[string]*hereRun{}
		}
		run := s.next.here[key]
		if run == nil {
			run = &hereRun{}
			s.next.here[key] = run
		}
		tooSoon := time.Since(run.last) < 2*time.Second
		tooMany := run.fixes >= hereMaxFixes
		if !tooSoon && !tooMany {
			run.last, run.fixes = time.Now(), run.fixes+1
		}
		s.next.mu.Unlock()
		if tooMany {
			fail(w, http.StatusTooManyRequests, "this link has sent all the positions it can")
			return
		}
		if tooSoon {
			w.WriteHeader(http.StatusNoContent) // a phone sending fast: the next one will do
			return
		}
		lat, lon := b.Lat, b.Lon
		loc := &ng911.Location{Lat: &lat, Lon: &lon, Radius: b.Accuracy, Altitude: b.Altitude, VRadius: b.AltAcc,
			Method: "Shared by the caller (phone's location, by link)", Source: "caller-link", At: time.Now().UTC()}
		if err := s.Eng.UpsertCall(cad.Actor{User: "caller-link"}, l.Call, cad.CallData{Source: "caller-link", Location: loc}); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (s *Server) hereLink(secret string) *cad.LocationLink {
	if len(secret) < 16 || len(secret) > 64 {
		return nil
	}
	var out *cad.LocationLink
	h := cad.HashSecret(secret)
	s.Eng.Read(func(st *cad.State) {
		if l := st.LocationLinkByHash(h); l != nil {
			ll := *l
			out = &ll
		}
	})
	return out
}

func herePage(title, text string, share bool) string {
	btn := ""
	if share {
		btn = `<button id="share">Share my location</button><button id="stop" hidden>Stop sharing</button><p id="state" role="status"></p>`
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>` + title + `</title>
<style>body{font:18px/1.45 system-ui,sans-serif;margin:0;padding:24px;background:#0b1220;color:#e5e7eb}h1{font-size:24px;margin:0 0 12px}
button{display:block;width:100%;font:700 20px system-ui;padding:18px;border:0;border-radius:10px;margin:18px 0 8px;background:#f59e0b;color:#111}
#stop{background:#334155;color:#e5e7eb;font-size:16px;padding:12px}#state{font-size:16px;color:#a5f3fc}</style></head>
<body><h1>` + title + `</h1><p>` + text + `</p>` + btn + `<script src="/here-share.js"></script></body></html>`
}

// hereScript asks the phone for its position and sends it, every few seconds, while the page is open.
const hereScript = `(function(){
var share=document.getElementById('share'),stop=document.getElementById('stop'),state=document.getElementById('state');
if(!share)return;var id=null,sent=0;
function say(t){state.textContent=t}
function post(body){return fetch(location.pathname,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)})}
share.onclick=function(){
 if(!navigator.geolocation){say('This phone cannot share its location from the browser. Tell the call-taker where you are.');return}
 say('Asking for your location…');
 id=navigator.geolocation.watchPosition(function(p){
  post({lat:p.coords.latitude,lon:p.coords.longitude,accuracy:p.coords.accuracy,altitude:p.coords.altitude,altitudeAccuracy:p.coords.altitudeAccuracy||0}).then(function(r){
   if(r.status===410){say('This link has ended.');navigator.geolocation.clearWatch(id);return}
   sent++;say('Sent to 911 ('+sent+'), within about '+Math.round(p.coords.accuracy)+' m. Keep this page open.')
  }).catch(function(){say('Could not reach 911 just now; trying again.')})
 },function(e){say(e.code===1?'Location is blocked. Allow it for this page in your settings, or tell the call-taker where you are.':'Your phone could not find its location yet; trying again.')},{enableHighAccuracy:true,maximumAge:5000,timeout:30000});
 share.hidden=true;stop.hidden=false};
stop.onclick=function(){if(id!==null)navigator.geolocation.clearWatch(id);post({stop:true});stop.hidden=true;say('Stopped. Thank you.')};
})();`
