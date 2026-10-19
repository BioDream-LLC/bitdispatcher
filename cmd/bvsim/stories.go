package main

// What simulated callers say, the simulated vehicle crash notifications, and a simulated drone system.
// All fictional, for the demonstration and tests.

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"
)

type line struct {
	after   int    // seconds after the call is answered
	speaker string // caller or taker
	text    string // {addr} is the caller's address
}

type story struct {
	name  string
	line  string // the number dialled, when not 911
	lines []line
	dups  []string // what other callers about the same thing say
}

var stories = []story{
	{name: "chest pain", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "{addr}. My father has chest pain and he's sweating a lot"},
		{9, "taker", "How old is he? Is he awake and breathing?"}, {13, "caller", "He's 67, he's awake, he's breathing but it's hard for him"}}},
	{name: "fall", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "{addr}. My mother fell in the bathroom and she can't get up, I think her hip is broken"},
		{10, "taker", "Is she awake? Is she breathing normally?"}, {14, "caller", "She's 82, she's awake and talking, she's breathing fine"}}},
	{name: "spanish breathing", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "Ayuda por favor, mi esposo no puede respirar"},
		{11, "caller", "Tiene setenta años, está en la sala"}, {18, "caller", "Por favor rápido"}}},
	{name: "house fire", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "{addr}. There's smoke coming from the house next door and I can see flames in the back"},
		{10, "taker", "Is anyone inside?"}, {14, "caller", "I don't know, their car is in the driveway"}},
		dups: []string{"I'm driving by, there's a house on fire, lots of black smoke", "The house across the street is on fire, flames out the back windows"}},
	{name: "crash", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "Two cars crashed at the intersection near {addr}, one man is bleeding badly from his head"},
		{11, "taker", "Is he awake? Is he breathing?"}, {15, "caller", "He's awake but he's confused. One of the cars is a red Ford pickup"}},
		dups: []string{"There's a bad wreck, two cars, somebody's hurt"}},
	{name: "robbery", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "{addr}. Someone just robbed the store, he had a knife"},
		{10, "taker", "Which way did he go? Can you describe him?"}, {14, "caller", "He left in a black Nissan Altima, Virginia plate XKD 2291, heading north on Williamson Rd"},
		{20, "caller", "He's wearing a gray hoodie and jeans"}}},
	{name: "domestic", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "{addr}. My neighbour is screaming and I heard something break"},
		{10, "taker", "Are there weapons in the house?"}, {14, "caller", "I think he has a gun, he's shown it to people before"}}},
	{name: "noise complaint", line: "+15408532121", lines: []line{{1, "taker", "Roanoke Valley non-emergency, what is the address?"},
		{4, "caller", "{addr}. I'd like to make a noise complaint, the neighbours have loud music again"}}},
	{name: "parking", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "{addr}. Somebody's parked in front of my driveway and I can't get out to go to work"}}},
	{name: "overdose", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "{addr}. My friend took something and now he's unconscious, he's barely breathing"},
		{10, "taker", "Is he breathing at all? How old is he?"}, {14, "caller", "Barely, he's 24, we're in the back bedroom"}}},
	{name: "spanish crash", lines: []line{{1, "taker", "911, what is the address of your emergency?"}, {4, "caller", "Hubo un choque, hay un hombre herido, está sangrando"},
		{11, "caller", "Es un carro blanco y una camioneta roja"}}},
}

// transcribe publishes a call's words line by line after it is answered.
func (s *sim) transcribe(id, addr string, st story, lines []line) {
	start := time.Now()
	for _, l := range lines {
		time.Sleep(time.Until(start.Add(time.Duration(l.after) * time.Second)))
		s.mu.Lock()
		_, ok := s.calls[id]
		s.mu.Unlock()
		if !ok {
			return
		}
		s.publish("call.transcript", map[string]any{"call": id, "lines": []map[string]any{{"speaker": l.speaker, "text": strings.ReplaceAll(l.text, "{addr}", addr), "at": time.Now().UTC()}}})
	}
}

// ---- crash notifications ----

// crash posts a simulated vehicle crash notification to bitdispatcher's hook.
func (s *sim) crash() {
	if s.push == nil || s.push.hookURL == "" {
		return
	}
	st := streets[mrand.IntN(len(streets))]
	makes := [][3]string{{"Tesla", "Model Y", "electric"}, {"Ford", "F-150", "gasoline"}, {"Toyota", "RAV4", "hybrid"}, {"Honda", "Civic", "gasoline"}, {"Chevrolet", "Bolt", "electric"}}
	m := makes[mrand.IntN(len(makes))]
	dv := 15 + mrand.Float64()*55
	body := map[string]any{"provider": "Simulated telematics", "deltaVKmh": math.Round(dv), "direction": []string{"front", "front", "left", "right", "rear"}[mrand.IntN(5)],
		"rollover": mrand.IntN(5) == 0, "airbags": []string{"front"}, "occupants": 1 + mrand.IntN(3), "belted": mrand.IntN(4) != 0,
		"speedKmh": math.Round(dv * 1.6), "fuel": m[2], "callback": fmt.Sprintf("+1540555%04d", 100+mrand.IntN(9800)),
		"lat": st.lat + (mrand.Float64()-0.5)*0.001, "lon": st.lon + (mrand.Float64()-0.5)*0.001,
		"vehicle": map[string]any{"year": 2019 + mrand.IntN(7), "make": m[0], "model": m[1], "color": []string{"white", "black", "silver", "blue", "red"}[mrand.IntN(5)], "plate": fmt.Sprintf("SIM%04d", mrand.IntN(10000))}}
	log.Printf("crash notification near %s %s", st.num, st.name)
	s.push.post(s.push.hookURL+"/acn", s.push.hookToken, "application/json", mustJSON(body))
}

// ---- a drone system ----

type drone struct {
	ID, Name   string
	BaseLat    float64
	BaseLon    float64
	Lat, Lon   float64
	Status     string
	Incident   string
	TargetLat  float64
	TargetLon  float64
	Battery    int
	launchedAt time.Time
}

type drones struct {
	mu   sync.Mutex
	list []*drone
	sim  *sim
}

func newDrones(s *sim) *drones {
	d := &drones{sim: s}
	for i, b := range [][2]float64{{37.2709, -79.9403}, {37.2905, -80.0530}} {
		d.list = append(d.list, &drone{ID: fmt.Sprintf("DFR%d", i+1), Name: fmt.Sprintf("Sky %d (SIMULATED)", i+1), BaseLat: b[0], BaseLon: b[1], Lat: b[0], Lon: b[1], Status: "docked", Battery: 100})
	}
	return d
}

// launch answers the drone system's launch request: the nearest docked drone flies.
func (d *drones) launch(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+d.sim.droneToken {
		http.Error(w, "unauthorised", 401)
		return
	}
	var req struct {
		Incident string  `json:"incident"`
		Lat      float64 `json:"lat"`
		Lon      float64 `json:"lon"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Incident == "" {
		http.Error(w, "bad request", 400)
		return
	}
	d.mu.Lock()
	var best *drone
	for _, x := range d.list {
		if x.Status == "docked" && (best == nil || dist(x.Lat, x.Lon, req.Lat, req.Lon) < dist(best.Lat, best.Lon, req.Lat, req.Lon)) {
			best = x
		}
	}
	if best == nil || dist(best.Lat, best.Lon, req.Lat, req.Lon) > 12000 {
		d.mu.Unlock()
		writeJSON(w, map[string]string{"detail": "declined: no drone free within range"})
		return
	}
	best.Status, best.Incident, best.TargetLat, best.TargetLon, best.launchedAt = "launching", req.Incident, req.Lat, req.Lon, time.Now()
	name := best.Name
	d.mu.Unlock()
	d.report(best)
	writeJSON(w, map[string]string{"detail": "launching", "drone": name})
}

func dist(lat1, lon1, lat2, lon2 float64) float64 {
	dy := (lat2 - lat1) * 111320
	dx := (lon2 - lon1) * 111320 * math.Cos(lat1*math.Pi/180)
	return math.Hypot(dx, dy)
}

// fly moves the flying drones every few seconds and reports them.
func (d *drones) fly() {
	for range time.Tick(3 * time.Second) {
		d.mu.Lock()
		var changed []*drone
		for _, x := range d.list {
			switch x.Status {
			case "launching":
				x.Status = "enroute"
				changed = append(changed, x)
			case "enroute", "returning":
				tlat, tlon := x.TargetLat, x.TargetLon
				if x.Status == "returning" {
					tlat, tlon = x.BaseLat, x.BaseLon
				}
				m := dist(x.Lat, x.Lon, tlat, tlon)
				step := 18.0 * 3 // 18 m/s
				if m <= step {
					x.Lat, x.Lon = tlat, tlon
					if x.Status == "enroute" {
						x.Status = "onscene"
					} else {
						x.Status, x.Incident, x.Battery = "docked", "", 100
					}
				} else {
					x.Lat += (tlat - x.Lat) * step / m
					x.Lon += (tlon - x.Lon) * step / m
				}
				x.Battery = max(20, x.Battery-1)
				changed = append(changed, x)
			case "onscene":
				// A slow orbit; home after six minutes.
				a := float64(time.Now().Unix()%60) / 60 * 2 * math.Pi
				x.Lat, x.Lon = x.TargetLat+0.0004*math.Sin(a), x.TargetLon+0.0005*math.Cos(a)
				x.Battery = max(20, x.Battery-1)
				if time.Since(x.launchedAt) > 6*time.Minute {
					x.Status = "returning"
				}
				changed = append(changed, x)
			}
		}
		d.mu.Unlock()
		for _, x := range changed {
			d.report(x)
		}
	}
}

func (d *drones) report(x *drone) {
	p := d.sim.push
	if p == nil || p.hookURL == "" {
		return
	}
	d.mu.Lock()
	body := map[string]any{"id": x.ID, "name": x.Name, "status": x.Status, "lat": x.Lat, "lon": x.Lon, "altM": 60, "battery": x.Battery, "incident": x.Incident, "pilot": "Remote pilot (SIMULATED)"}
	d.mu.Unlock()
	go p.post(p.hookURL+"/drone", p.hookToken, "application/json", mustJSON(body))
}

// radioWords are what simulated units say, for the radio transcript.
var radioWords = []string{"copy", "10-4", "checking the area", "clear of the last call", "on the air", "show me in service", "received, thanks"}

func radioText(alias string) string {
	return alias + ", " + radioWords[mrand.IntN(len(radioWords))] + "."
}

// runID keeps call IDs unique across restarts, so a restarted simulator never reuses an ended call's ID.
var runID = fmt.Sprintf("%x", time.Now().Unix()%0xfffff)
