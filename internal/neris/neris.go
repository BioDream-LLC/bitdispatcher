// Package neris sends fire and EMS incidents to NERIS, the National Emergency Response Information
// System that replaced NFIRS in 2026, as a CAD integration: the dispatch part of the record (when the
// call came in and was answered, when each unit was dispatched, went en route, arrived and cleared, where,
// and how it ended). NERIS keeps a CAD record as "pending incident data" and the fire department
// completes the rest (the final incident type, actions, fire and medical detail) in NERIS or its RMS.
//
// The API, its OAuth client-credentials sign-in and the payload follow NERIS's published OpenAPI
// specification (api.neris.fsri.org/openapi.json, version 1.5.2 when this was written).
package neris

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Hosts: NERIS's test environment for building and testing, production for real records.
const (
	Production = "https://api.neris.fsri.org/v1"
	Test       = "https://api-test.neris.fsri.org/v1"
)

// Config is what an administrator sets in Admin › Integrations › NERIS.
type Config struct {
	On           bool   `json:"on"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Department   string `json:"department"` // the fire department's NERIS ID: FD and eight digits
	Center       string `json:"center"`     // the dispatch centre's name or ID, sent as center_id
	Test         bool   `json:"test"`       // NERIS's test environment
	// Auto sends each fire or EMS incident when it is closed; otherwise only when someone presses Send.
	Auto bool `json:"auto"`
}

var reDept = regexp.MustCompile(`^FD\d{8}$`)

// Check says what is wrong with a configuration that is on.
func (c Config) Check() error {
	if !c.On {
		return nil
	}
	if !reDept.MatchString(c.Department) {
		return errors.New("the department's NERIS ID is FD and eight digits, such as FD51161234")
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return errors.New("NERIS needs the integration's client ID and secret")
	}
	return nil
}

// Incident is what bitdispatcher knows of an incident, for NERIS.
type Incident struct {
	Number      string
	Type        string // the call type's code (incident_code)
	TypeName    string
	Created     time.Time
	CallArrival time.Time // the first call started ringing (or the incident was created)
	CallAnswer  time.Time
	Closed      time.Time
	Disposition string
	Address     string
	Civic       *Civic
	Lat, Lon    *float64
	Alarm       bool // it came from an alarm company
	Units       []Unit
	Comments    []Comment
}

// Civic is the parts of an address, where call handling gave them.
type Civic struct {
	Number, Street, City, County, State, Postal string
}

// Unit is one unit's times on the incident.
type Unit struct {
	ID                                    string
	Dispatched, Enroute, OnScene, Cleared time.Time
	CanceledEnroute                       bool
}

// Comment is a note on the incident.
type Comment struct {
	At   time.Time
	Text string
}

// Payload is the NERIS IncidentPayload for a CAD integration.
func Payload(c Config, in Incident) map[string]any {
	loc := location(in)
	var point any
	if in.Lat != nil && in.Lon != nil {
		point = map[string]any{"crs": 4326, "geometry": map[string]any{"type": "Point", "coordinates": []float64{*in.Lon, *in.Lat}}}
	}
	ts := func(t time.Time) any {
		if t.IsZero() {
			return nil
		}
		return t.Format(time.RFC3339)
	}
	arrival := in.CallArrival
	if arrival.IsZero() {
		arrival = in.Created
	}
	answer := in.CallAnswer
	if answer.IsZero() {
		answer = arrival
	}
	units := []map[string]any{}
	for _, u := range in.Units {
		r := map[string]any{"reported_unit_id": u.ID, "dispatch": ts(u.Dispatched), "enroute_to_scene": ts(u.Enroute), "on_scene": ts(u.OnScene), "unit_clear": ts(u.Cleared)}
		if u.CanceledEnroute && !u.Cleared.IsZero() {
			r["canceled_enroute"] = ts(u.Cleared)
		}
		units = append(units, r)
	}
	comments := []map[string]any{}
	for _, k := range in.Comments {
		if strings.TrimSpace(k.Text) != "" && !k.At.Before(minTime) {
			comments = append(comments, map[string]any{"timestamp": k.At.Format(time.RFC3339), "comment": k.Text})
		}
	}
	dispatch := map[string]any{
		"incident_number": in.Number, "call_arrival": ts(arrival), "call_answered": ts(answer), "call_create": ts(in.Created),
		"location": loc, "unit_responses": units, "incident_code": strings.TrimSpace(in.Type + " " + in.TypeName),
		"automatic_alarm": in.Alarm, "incident_clear": ts(in.Closed), "disposition": nilIfEmpty(in.Disposition), "point": point,
	}
	if c.Center != "" {
		dispatch["center_id"] = c.Center
	}
	if len(comments) > 0 {
		dispatch["comments"] = comments
	}
	return map[string]any{
		"base":           map[string]any{"department_neris_id": c.Department, "incident_number": in.Number, "location": loc, "point": point},
		"incident_types": []map[string]string{{"type": "UNDETERMINED"}}, // the department sets the type
		"dispatch":       dispatch,
	}
}

// minTime is the earliest comment time NERIS takes (it refuses times before 2025).
var minTime = time.Date(2025, 1, 1, 5, 0, 0, 0, time.UTC)

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

var reNumberStreet = regexp.MustCompile(`^\s*(\d+[A-Za-z]?)\s+(.+)$`)

// location is the NERIS LocationPayload: from the civic parts when call handling gave them, else from
// the one-line address (number and street; the rest in additional_info).
func location(in Incident) map[string]any {
	l := map[string]any{"country": "US"}
	if c := in.Civic; c != nil {
		if c.Number != "" {
			l["complete_number"] = c.Number
			if n, err := strconv.Atoi(c.Number); err == nil {
				l["number"] = n
			}
		}
		set := func(k, v string) {
			if v = strings.TrimSpace(v); v != "" {
				l[k] = v
			}
		}
		set("street", c.Street)
		set("incorporated_municipality", c.City)
		set("county", c.County)
		set("postal_code", c.Postal)
		if len(c.State) == 2 {
			l["state"] = strings.ToUpper(c.State)
		}
	}
	addr := strings.TrimSpace(in.Address)
	if addr != "" {
		if _, ok := l["street"]; !ok {
			first := strings.TrimSpace(strings.SplitN(addr, ",", 2)[0])
			if m := reNumberStreet.FindStringSubmatch(first); m != nil {
				l["complete_number"] = m[1]
				if n, err := strconv.Atoi(m[1]); err == nil {
					l["number"] = n
				}
				l["street"] = m[2]
			} else {
				l["street"] = first
			}
		}
		if len(addr) > 255 {
			addr = addr[:255]
		}
		l["additional_info"] = addr
	}
	return l
}

// Client talks to NERIS.
type Client struct {
	HTTP  *http.Client
	Agent string // NERIS refuses requests without a User-Agent
	Base  string // overrides the host (tests)

	mu    sync.Mutex
	token string
	exp   time.Time
	key   string // the credentials the token is for
}

func (cl *Client) base(c Config) string {
	if cl.Base != "" {
		return cl.Base
	}
	if c.Test {
		return Test
	}
	return Production
}

func (cl *Client) http() *http.Client {
	if cl.HTTP != nil {
		return cl.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (cl *Client) bearer(ctx context.Context, c Config) (string, error) {
	key := c.ClientID + "\x00" + c.ClientSecret + "\x00" + cl.base(c)
	cl.mu.Lock()
	if cl.token != "" && cl.key == key && time.Now().Before(cl.exp) {
		t := cl.token
		cl.mu.Unlock()
		return t, nil
	}
	cl.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, "POST", cl.base(c)+"/token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.ClientID+":"+c.ClientSecret)))
	req.Header.Set("User-Agent", cl.Agent)
	resp, err := cl.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var b struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Detail      any    `json:"detail"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&b)
	if resp.StatusCode != 200 || b.AccessToken == "" {
		return "", fmt.Errorf("NERIS refused the sign-in (%s)", resp.Status)
	}
	life := time.Duration(b.ExpiresIn) * time.Second
	if life <= 0 {
		life = time.Hour
	}
	cl.mu.Lock()
	cl.token, cl.exp, cl.key = b.AccessToken, time.Now().Add(life-2*time.Minute), key
	cl.mu.Unlock()
	return b.AccessToken, nil
}

// Result is what NERIS said.
type Result struct {
	ID     string `json:"id,omitempty"` // NERIS's ID for the incident
	Status int    `json:"status"`
	Body   string `json:"body,omitempty"` // NERIS's answer, for the administrator (errors say what is wrong)
}

// Send creates the incident in NERIS, or updates it when NERIS already has it (id).
func (cl *Client) Send(ctx context.Context, c Config, in Incident, id string) (Result, error) {
	if err := c.Check(); err != nil {
		return Result{}, err
	}
	if id != "" {
		return cl.do(ctx, c, "PUT", "/incident/"+c.Department+"/"+id, Payload(c, in))
	}
	return cl.do(ctx, c, "POST", "/incident/"+c.Department, Payload(c, in))
}

// Validate asks NERIS to check the record without keeping it.
func (cl *Client) Validate(ctx context.Context, c Config, in Incident) (Result, error) {
	if err := c.Check(); err != nil {
		return Result{}, err
	}
	return cl.do(ctx, c, "POST", "/incident/"+c.Department+"/validate", Payload(c, in))
}

func (cl *Client) do(ctx context.Context, c Config, method, path string, body any) (Result, error) {
	tok, err := cl.bearer(ctx, c)
	if err != nil {
		return Result{}, err
	}
	data, _ := json.Marshal(body)
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, cl.base(c)+path, bytes.NewReader(data))
		if err != nil {
			return Result{}, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", cl.Agent)
		resp, err := cl.http().Do(req)
		if err != nil {
			return Result{}, err
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		// Too many requests: back off, a few times.
		if resp.StatusCode == 429 && attempt < 3 {
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
			continue
		}
		r := Result{Status: resp.StatusCode, Body: string(raw)}
		var x map[string]any
		if json.Unmarshal(raw, &x) == nil {
			for _, k := range []string{"neris_id", "id", "incident_id", "neris_id_incident"} {
				if v, ok := x[k].(string); ok && v != "" {
					r.ID = v
					break
				}
			}
		}
		if resp.StatusCode >= 300 {
			return r, fmt.Errorf("NERIS: %s", resp.Status)
		}
		return r, nil
	}
}
