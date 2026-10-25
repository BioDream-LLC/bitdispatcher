package cad

// Cameras near an incident: the agency's own list of cameras (traffic cameras, its buildings, schools and
// partners that agreed to share), each with where it is and which way it looks, a link to its live view
// and, for many traffic cameras, a still picture. It is off until an administrator switches it on. Opening
// a camera asks why (the incident, or a reason), and every look is recorded with who, when and why: a
// camera is something a dispatcher looks at for a call, not a way of watching people. bitdispatcher keeps
// no video; the camera's own system does whatever recording it does.

import (
	"net/url"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/geo"
	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvCameraSaved   = "camera.saved"
	EvCameraRemoved = "camera.removed"
	EvCameraLooked  = "camera.looked"
)

// Camera is one camera on the agency's list.
type Camera struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Owner   string   `json:"owner,omitempty"` // whose camera it is: "VDOT", "City schools"
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	Heading *float64 `json:"heading,omitempty"` // degrees from north it looks toward; empty: turns or unknown
	FOV     float64  `json:"fov,omitempty"`     // degrees across (0: 70)
	Range   float64  `json:"range,omitempty"`   // metres it usefully sees (0: 150)
	// Live is the address of its live view in the camera system (opened in a new window; that system signs
	// the dispatcher in). Still is a picture of what it sees now, fetched through bitdispatcher.
	Live  string `json:"live,omitempty"`
	Still string `json:"still,omitempty"`
	Note  string `json:"note,omitempty"`
	Off   bool   `json:"off,omitempty"`
}

// CameraLook is one opening of a camera, kept for the record.
type CameraLook struct {
	At       time.Time `json:"at"`
	User     string    `json:"user"`
	Camera   string    `json:"camera"`
	Name     string    `json:"name"`
	Incident string    `json:"incident,omitempty"`
	Purpose  string    `json:"purpose"`
	Seq      uint64    `json:"seq"`
}

// CameraPolicy is how the agency uses cameras.
type CameraPolicy struct {
	On     bool    `json:"on,omitempty"`     // off until switched on
	Radius float64 `json:"radius,omitempty"` // how far from an incident cameras are offered (0: 400 m)
}

// RadiusOrDefault is the offering radius.
func (p CameraPolicy) RadiusOrDefault() float64 {
	if p.Radius <= 0 {
		return 400
	}
	return p.Radius
}

const keepCameraLooks = 2000

func cameraURL(field, u string) error {
	if u == "" {
		return nil
	}
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") || p.Host == "" {
		return problem(field, "an address starting with https:// (or http:// on the agency's own network)")
	}
	return nil
}

// SaveCamera adds or changes a camera.
func (e *Engine) SaveCamera(a Actor, c Camera) (string, error) {
	c.Name, c.Owner, c.Note = strings.TrimSpace(c.Name), strings.TrimSpace(c.Owner), strings.TrimSpace(c.Note)
	c.Live, c.Still = strings.TrimSpace(c.Live), strings.TrimSpace(c.Still)
	switch {
	case c.Name == "":
		return "", problem("name", "give the camera a name")
	case c.Lat < -90 || c.Lat > 90 || c.Lon < -180 || c.Lon > 180 || (c.Lat == 0 && c.Lon == 0):
		return "", problem("lat", "place the camera on the map")
	case c.Heading != nil && (*c.Heading < 0 || *c.Heading >= 360):
		return "", problem("heading", "a heading is 0 to 359 degrees")
	case c.FOV < 0 || c.FOV > 360 || c.Range < 0 || c.Range > 5000:
		return "", problem("fov", "view is up to 360 degrees across and 5 km deep")
	case c.Live == "" && c.Still == "":
		return "", problem("live", "give the address of its live view, its still picture, or both")
	}
	if err := cameraURL("live", c.Live); err != nil {
		return "", err
	}
	if err := cameraURL("still", c.Still); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if c.ID == "" {
		c.ID = e.newID("CAM", func(id string) bool { return e.st.Cameras[id] != nil })
	} else if e.st.Cameras[c.ID] == nil {
		return "", problem("id", "no camera %s", c.ID)
	}
	_, err := e.write(a, EvCameraSaved, refs{}, c)
	return c.ID, err
}

// RemoveCamera takes a camera off the list.
func (e *Engine) RemoveCamera(a Actor, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.Cameras[id] == nil {
		return problem("id", "no camera %s", id)
	}
	_, err := e.write(a, EvCameraRemoved, refs{}, idData{id})
	return err
}

// LookAtCamera records that someone opens a camera, and why. It refuses when cameras are off, the camera
// is off, or no purpose is given.
func (e *Engine) LookAtCamera(a Actor, id, incident, purpose string) (Camera, error) {
	purpose = strings.TrimSpace(purpose)
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.st.Cameras[id]
	switch {
	case !e.st.Settings.Next.Cameras.On:
		return Camera{}, problem("camera", "cameras are switched off (Admin › Cameras)")
	case c == nil || c.Off:
		return Camera{}, problem("camera", "no camera %s in service", id)
	case incident == "" && purpose == "":
		return Camera{}, problem("purpose", "say why you are opening this camera: the incident, or a reason")
	case incident != "" && e.st.Incidents[incident] == nil:
		return Camera{}, problem("incident", "no incident %s", incident)
	case len(purpose) > 300:
		return Camera{}, problem("purpose", "under 300 characters")
	}
	if purpose == "" {
		purpose = "incident " + incident
	}
	_, err := e.write(a, EvCameraLooked, refs{Incident: incident}, CameraLook{User: a.User, Camera: id, Name: c.Name, Incident: incident, Purpose: purpose})
	return *c, err
}

// CameraNear is a camera near a place: how far, and whether it looks toward it.
type CameraNear struct {
	*Camera
	Metres float64 `json:"metres"`
	Facing *bool   `json:"facing,omitempty"` // the place is in its view; empty when its heading is unknown
}

// CamerasNear are the cameras in service within the policy's radius of a place, nearest first.
func (s *State) CamerasNear(lat, lon float64) []CameraNear {
	r := s.Settings.Next.Cameras.RadiusOrDefault()
	var out []CameraNear
	for _, c := range s.Cameras {
		if c.Off {
			continue
		}
		d := geo.Distance(c.Lat, c.Lon, lat, lon)
		if d > r {
			continue
		}
		n := CameraNear{Camera: c, Metres: d}
		if c.Heading != nil {
			fov, rng := c.FOV, c.Range
			if fov <= 0 {
				fov = 70
			}
			if rng <= 0 {
				rng = 150
			}
			b := geo.Bearing(c.Lat, c.Lon, lat, lon)
			diff := b - *c.Heading
			for diff > 180 {
				diff -= 360
			}
			for diff < -180 {
				diff += 360
			}
			f := d <= rng && (d < 15 || diff <= fov/2 && diff >= -fov/2)
			n.Facing = &f
		}
		out = append(out, n)
	}
	sortCameras(out)
	return out
}

func sortCameras(c []CameraNear) {
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && rankCam(c[j]) < rankCam(c[j-1]); j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}

// rankCam puts the cameras looking at the place first, then the nearest.
func rankCam(c CameraNear) float64 {
	if c.Facing != nil && *c.Facing {
		return c.Metres
	}
	return c.Metres + 100000
}

func (s *State) applyCameras(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvCameraSaved:
		c, err := decode[Camera](e)
		if err != nil {
			return true, err
		}
		s.Cameras[c.ID] = &c
	case EvCameraRemoved:
		v, err := decode[idData](e)
		if err != nil {
			return true, err
		}
		delete(s.Cameras, v.ID)
	case EvCameraLooked:
		l, err := decode[CameraLook](e)
		if err != nil {
			return true, err
		}
		l.At, l.Seq = e.At, e.Seq
		s.CameraLooks = append(s.CameraLooks, l)
		if len(s.CameraLooks) > keepCameraLooks {
			s.CameraLooks = append([]CameraLook(nil), s.CameraLooks[len(s.CameraLooks)-keepCameraLooks:]...)
		}
		if in := s.Incidents[l.Incident]; in != nil {
			in.add(e, "camera", "Camera opened: "+l.Name)
		}
	default:
		return false, nil
	}
	return true, nil
}
