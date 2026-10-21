package cad

// Attachments: files on an incident (a crew's photo of the scene, a caller's picture, a PDF from a
// partner, a document for the report). The file is kept beside the ledger; its name, type, size and
// SHA-256 are in the ledger, so a changed or swapped file is found. Removing one keeps the record and the
// file and only hides it, with who and why: an incident's files are records.

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/ledger"
)

const (
	EvFileAdded  = "incident.file.added"
	EvFileHidden = "incident.file.hidden"
)

// MaxIncidentFiles is how many files an incident keeps.
const MaxIncidentFiles = 100

// IncidentFile is one attachment.
type IncidentFile struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Caption  string    `json:"caption,omitempty"`
	Type     string    `json:"type"`
	SHA256   string    `json:"sha256"`
	Size     int64     `json:"size"`
	Added    time.Time `json:"added"`
	By       string    `json:"by"`
	Unit     string    `json:"unit,omitempty"` // the crew that sent it
	Hidden   bool      `json:"hidden,omitempty"`
	HiddenBy string    `json:"hiddenBy,omitempty"`
	Why      string    `json:"why,omitempty"`
}

// AddIncidentFile records a file on an incident (the file is already stored under f.ID).
func (e *Engine) AddIncidentFile(a Actor, number string, f IncidentFile) error {
	if err := mayEnter(a); err != nil {
		return err
	}
	f.Name, f.Caption = cleanName(f.Name), strings.TrimSpace(f.Caption)
	if len(f.Caption) > 300 {
		return problem("caption", "a caption is up to 300 characters")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return problem("incident", "no incident %s", number)
	}
	if len(in.Files) >= MaxIncidentFiles {
		return problem("file", "an incident keeps up to %d files", MaxIncidentFiles)
	}
	f.Added, f.By, f.Hidden, f.HiddenBy, f.Why = e.now(), a.User, false, "", ""
	_, err := e.write(a, EvFileAdded, refs{Incident: number, Unit: f.Unit}, f)
	return err
}

// HideIncidentFile hides a file from the incident (a wrong picture, one not to be shown), with why.
func (e *Engine) HideIncidentFile(a Actor, number, id, why string) error {
	why = strings.TrimSpace(why)
	if why == "" {
		return problem("why", "say why the file is removed")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	in, ok := e.st.Incidents[number]
	if !ok {
		return problem("incident", "no incident %s", number)
	}
	if !slices.ContainsFunc(in.Files, func(f IncidentFile) bool { return f.ID == id && !f.Hidden }) {
		return problem("file", "no file %s on %s", id, number)
	}
	_, err := e.write(a, EvFileHidden, refs{Incident: number}, map[string]string{"id": id, "why": why})
	return err
}

// cleanName keeps a file's name to its last part, without control characters, up to 120 characters.
func cleanName(n string) string {
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	n = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' {
			return -1
		}
		return r
	}, strings.TrimSpace(n))
	if len([]rune(n)) > 120 {
		n = string([]rune(n)[:120])
	}
	if n == "" {
		n = "file"
	}
	return n
}

func (s *State) applyFiles(e ledger.Event) (bool, error) {
	switch e.Type {
	case EvFileAdded:
		f, err := decode[IncidentFile](e)
		if err != nil {
			return true, err
		}
		if in, ok := s.Incidents[e.Incident]; ok {
			in.Files = append(in.Files, f)
			who := f.Unit
			if who == "" {
				who = f.By
			}
			in.add(e, "file", fmt.Sprintf("file added by %s: %s%s", who, f.Name, suffix(f.Caption)))
		}
		return true, nil
	case EvFileHidden:
		v, _ := decode[map[string]string](e)
		if in, ok := s.Incidents[e.Incident]; ok {
			for i := range in.Files {
				if in.Files[i].ID == v["id"] {
					in.Files[i].Hidden, in.Files[i].HiddenBy, in.Files[i].Why = true, e.Actor, v["why"]
					in.add(e, "file", fmt.Sprintf("file removed: %s (%s)", in.Files[i].Name, v["why"]))
				}
			}
		}
		return true, nil
	}
	return false, nil
}
