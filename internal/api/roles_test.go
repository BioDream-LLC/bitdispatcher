package api

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/ng911"
)

func fresh(t *testing.T, base string, tr http.RoundTripper) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, c: &http.Client{Transport: tr, Jar: jar}}
}

// Sign-in asks for the duty, opens the session working it with that duty's rights, and the server keeps a
// call-taker from sending units; a workstation enrolled with a code gives the position.
func TestDutiesWorkstationsAndLock(t *testing.T) {
	ts, s, pw := newServer(t)
	s.Eng.SaveUnit(cad.SystemActor, cad.UnitDef{ID: "E1", Type: "engine"})
	s.Eng.SetStatus(cad.SystemActor, "E1", cad.Available, "", "")
	admin := login(t, ts, "admin", pw["admin"])
	// A workstation, and an enrolment code for another browser.
	if code, b := admin.do("PUT", "/api/admin/workstations/ct-3", cad.Workstation{Name: "Call-taker 3", Duties: []string{"calltaker"}}, true); code != 204 {
		t.Fatalf("workstation: %d %s", code, b)
	}
	code, b := admin.do("POST", "/api/admin/workstations/CT-3/code", nil, true)
	var cd struct{ Code string }
	json.Unmarshal(b, &cd)
	if code != 200 || len(cd.Code) != 11 {
		t.Fatalf("code: %d %s", code, b)
	}
	anon := fresh(t, ts.URL, ts.Client().Transport)
	code, b = anon.do("POST", "/api/workstations/enrol", map[string]string{"code": cd.Code, "label": "test"}, true)
	var tok struct{ Token, Workstation string }
	json.Unmarshal(b, &tok)
	if code != 200 || tok.Workstation != "CT-3" {
		t.Fatalf("enrol: %d %s", code, b)
	}
	if code, _ := anon.do("POST", "/api/workstations/enrol", map[string]string{"code": cd.Code}, true); code != 401 {
		t.Fatalf("an enrolment code worked twice: %d", code)
	}
	// d1 signs in at CT-3: asked for a duty, the workstation's first.
	d := fresh(t, ts.URL, ts.Client().Transport)
	code, b = d.do("POST", "/api/auth/login", map[string]string{"username": "d1", "password": pw["d1"], "workstation": tok.Token}, true)
	if code != 202 || !strings.Contains(string(b), `"duties":["calltaker"`) {
		t.Fatalf("duty asked: %d %s", code, b)
	}
	code, b = d.do("POST", "/api/auth/login", map[string]string{"username": "d1", "password": pw["d1"], "workstation": tok.Token, "duty": "calltaker"}, true)
	if code != 200 || !strings.Contains(string(b), `"console":"CT-3"`) || !strings.Contains(string(b), `"duty":"calltaker"`) {
		t.Fatalf("signed in: %d %s", code, b)
	}
	// A duty the person does not hold is refused.
	if code, _ := fresh(t, ts.URL, ts.Client().Transport).do("POST", "/api/auth/login", map[string]string{"username": "d1", "password": pw["d1"], "duty": "admin"}, true); code != 401 {
		t.Fatalf("a duty not held: %d", code)
	}
	// The call-taker enters an incident but cannot send a unit.
	code, b = d.do("POST", "/api/incidents", cad.NewIncident{Type: "SF", Location: ng911.Point(37.27, -79.94)}, true)
	var inc struct{ Number string }
	json.Unmarshal(b, &inc)
	if code != 200 && code != 201 {
		t.Fatalf("call-taker created: %d %s", code, b)
	}
	if code, b := d.do("POST", "/api/incidents/"+inc.Number+"/dispatch", map[string]any{"units": []string{"E1"}, "confirm": true}, true); code == 200 || !strings.Contains(string(b), "call-taker") {
		t.Fatalf("a call-taker dispatched: %d %s", code, b)
	}
	// Lock: nothing until unlocked; a wrong password does not unlock.
	if code, _ := d.do("POST", "/api/auth/lock", nil, true); code != 204 {
		t.Fatalf("lock: %d", code)
	}
	if code, _ := d.do("GET", "/api/state", nil, false); code != http.StatusLocked {
		t.Fatalf("locked state: %d", code)
	}
	if code, _ := d.do("POST", "/api/auth/unlock", map[string]string{"password": "wrong-password-here"}, true); code != 401 {
		t.Fatalf("wrong unlock: %d", code)
	}
	if code, _ := d.do("POST", "/api/auth/unlock", map[string]string{"password": pw["d1"]}, true); code != 200 {
		t.Fatalf("unlock: %d", code)
	}
	if code, _ := d.do("GET", "/api/state", nil, false); code != 200 {
		t.Fatalf("after unlock: %d", code)
	}
	// The ledger has the sign-in with the duty and workstation, and the lock and unlock.
	var types []string
	evs, _ := s.Eng.Events(0, 0, nil)
	for _, e := range evs {
		if e.Actor == "d1" {
			types = append(types, e.Type+"/"+e.Duty+"/"+e.Console)
		}
	}
	j := strings.Join(types, " ")
	if !strings.Contains(j, "session.signin/calltaker/CT-3") || !strings.Contains(j, cad.EvLocked) || !strings.Contains(j, cad.EvUnlocked) {
		t.Fatalf("audit: %s", j)
	}
}

// With multi-factor sign-in on, a person sets up an authenticator at their first sign-in and types its code
// every time after; the same code never works twice.
func TestMFA(t *testing.T) {
	ts, s, pw := newServer(t)
	admin := login(t, ts, "admin", pw["admin"])
	var st cad.Settings
	s.Eng.Read(func(x *cad.State) { st = x.Settings })
	st.Security.MFA = "everyone"
	if code, b := admin.do("PUT", "/api/admin/settings", st, true); code != 204 && code != 200 {
		t.Fatalf("settings: %d %s", code, b)
	}
	deadline := time.Now().Add(2 * time.Second)
	for s.Auth.MFA != "everyone" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s.Auth.SetPolicy("everyone", 0, "test") // the watch runs with Run; set it here too
	d := fresh(t, ts.URL, ts.Client().Transport)
	code, b := d.do("POST", "/api/auth/login", map[string]string{"username": "d1", "password": pw["d1"], "duty": "dispatcher"}, true)
	var need struct {
		Need auth.Need `json:"need"`
	}
	json.Unmarshal(b, &need)
	if code != 202 || need.Need.Enrol == nil || !strings.HasPrefix(need.Need.Enrol.URI, "otpauth://totp/") {
		t.Fatalf("enrol: %d %s", code, b)
	}
	codeNow := auth.TOTP(need.Need.Enrol.Secret, time.Now().Unix()/30)
	if code, b := d.do("POST", "/api/auth/login", map[string]string{"username": "d1", "password": pw["d1"], "duty": "dispatcher", "code": codeNow}, true); code != 200 {
		t.Fatalf("first code: %d %s", code, b)
	}
	// The same code again is refused; the next step's is accepted.
	d2 := fresh(t, ts.URL, ts.Client().Transport)
	if code, _ := d2.do("POST", "/api/auth/login", map[string]string{"username": "d1", "password": pw["d1"], "duty": "dispatcher", "code": codeNow}, true); code != 401 {
		t.Fatalf("a code reused: %d", code)
	}
	next := auth.TOTP(need.Need.Enrol.Secret, time.Now().Unix()/30+1)
	if code, b := d2.do("POST", "/api/auth/login", map[string]string{"username": "d1", "password": pw["d1"], "duty": "dispatcher", "code": next}, true); code != 200 {
		t.Fatalf("next code: %d %s", code, b)
	}
	// Without a code, it is asked for.
	if code, b := fresh(t, ts.URL, ts.Client().Transport).do("POST", "/api/auth/login", map[string]string{"username": "d1", "password": pw["d1"], "duty": "dispatcher"}, true); code != 202 || !strings.Contains(string(b), `"code":true`) {
		t.Fatalf("code asked: %d %s", code, b)
	}
}

// An exercise is one board for everyone who joins it; the instructor injects a call; an administrator
// stages a call type on a training board and publishes it to the live board, which records why.
func TestExerciseAndStaging(t *testing.T) {
	ts, s, pw := newServer(t)
	join := func(user, duty string) *client {
		c := fresh(t, ts.URL, ts.Client().Transport)
		if code, b := c.do("POST", "/api/auth/login", map[string]any{"username": user, "password": pw[user], "duty": duty, "training": true, "exercise": "Tuesday drill"}, true); code != 200 {
			t.Fatalf("join %s: %d %s", user, code, b)
		}
		return c
	}
	instr := join("admin", "supervisor")
	trainee := join("d1", "calltaker")
	lat, lon := 37.27, -79.94
	if code, b := trainee.do("POST", "/api/training/inject", Inject{Kind: "call", Lat: &lat, Lon: &lon}, true); code != 403 {
		t.Fatalf("a trainee injected: %d %s", code, b)
	}
	if code, b := instr.do("POST", "/api/training/inject", Inject{Kind: "call", Call: "t-ex-1", Lat: &lat, Lon: &lon, Name: "PRACTICE, P"}, true); code != 204 {
		t.Fatalf("inject: %d %s", code, b)
	}
	if _, b := trainee.do("GET", "/api/state", nil, false); !strings.Contains(string(b), "t-ex-1") {
		t.Fatal("the trainee does not see the instructor's call: not one board")
	}
	if _, b := login(t, ts, "d1", pw["d1"]).do("GET", "/api/state", nil, false); strings.Contains(string(b), "t-ex-1") {
		t.Fatal("the exercise reached the live board")
	}
	// Staging.
	stage := fresh(t, ts.URL, ts.Client().Transport)
	if code, b := stage.do("POST", "/api/auth/login", map[string]any{"username": "admin", "password": pw["admin"], "duty": "admin", "training": true}, true); code != 200 {
		t.Fatalf("staging sign-in: %d %s", code, b)
	}
	if code, b := stage.do("PUT", "/api/admin/calltypes/GASOUT", cad.CallType{Code: "GASOUT", Name: "Gas leak outside", Priority: 2, Discipline: "fire"}, true); code != 204 && code != 200 {
		t.Fatalf("staged call type: %d %s", code, b)
	}
	if code, b := stage.do("GET", "/api/training/staged", nil, false); code != 200 || !strings.Contains(string(b), "added GASOUT") {
		t.Fatalf("diff: %d %s", code, b)
	}
	if code, b := stage.do("POST", "/api/training/publish", map[string]any{"parts": []string{"calltypes"}, "why": "new gas leak type"}, true); code != 200 {
		t.Fatalf("publish: %d %s", code, b)
	}
	var ok bool
	s.Eng.Read(func(st *cad.State) { _, ok = st.CallTypes["GASOUT"] })
	if !ok {
		t.Fatal("not published")
	}
	_ = json.Marshal
}
