package cad

// Drone video as evidence. A drone bitdispatcher flies records its picture for the incident it is over,
// in one file per stretch of flight. While a clip is being recorded the server writes a checkpoint to the
// ledger every so often: the clip's length so far and the SHA-256 of its bytes up to there. A clip only
// grows, so each checkpoint proves the recording up to that point was not changed afterwards, even if the
// server stopped before the clip ended. The last checkpoint is marked final.

import (
	"fmt"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const EvDroneClip = "drone.video.clip"

// DroneClip is a recording of a drone's video for an incident.
type DroneClip struct {
	ID     string    `json:"id"`
	Drone  string    `json:"drone"`
	Name   string    `json:"name,omitempty"` // the drone's name
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`    // the last frame so far
	Size   int64     `json:"size"`   // bytes so far
	SHA256 string    `json:"sha256"` // of the first Size bytes
	Final  bool      `json:"final,omitempty"`
	Type   string    `json:"type"` // video/mp4
	Codec  string    `json:"codec,omitempty"`
}

// DroneClip records a checkpoint of a drone video clip for an incident.
func (e *Engine) DroneClip(a Actor, number string, c DroneClip) error {
	if c.ID == "" || c.Drone == "" || len(c.SHA256) != 64 || c.Size <= 0 {
		return problem("clip", "a clip needs its id, drone, size and SHA-256")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return problem("incident", "no incident %s", number)
	}
	for _, x := range in.DroneClips {
		if x.ID == c.ID && (x.Final || c.Size < x.Size) {
			return problem("clip", "clip %s is already closed or longer", c.ID)
		}
	}
	_, err := e.write(a, EvDroneClip, refs{Incident: number}, c)
	return err
}

func (s *State) applyDroneClip(e ledger.Event) (bool, error) {
	if e.Type != EvDroneClip {
		return false, nil
	}
	c, err := decode[DroneClip](e)
	if err != nil {
		return true, err
	}
	in, ok := s.Incidents[e.Incident]
	if !ok {
		return true, nil
	}
	name := firstNonEmpty(c.Name, c.Drone)
	for i := range in.DroneClips {
		if in.DroneClips[i].ID == c.ID {
			in.DroneClips[i] = c
			if c.Final {
				in.add(e, "drone", fmt.Sprintf("Drone %s video recorded: %s, %s (SHA-256 %s)", name,
					c.End.Sub(c.Start).Round(time.Second), byteWords(c.Size), c.SHA256[:12]))
			}
			return true, nil
		}
	}
	in.DroneClips = append(in.DroneClips, c)
	text := "Drone " + name + " video recording started"
	if c.Final {
		text = fmt.Sprintf("Drone %s video recorded: %s, %s (SHA-256 %s)", name, c.End.Sub(c.Start).Round(time.Second), byteWords(c.Size), c.SHA256[:12])
	}
	in.add(e, "drone", text)
	return true, nil
}

func byteWords(n int64) string {
	switch {
	case n >= 1<<20:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/(1<<20)), ".0") + " MB"
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}
