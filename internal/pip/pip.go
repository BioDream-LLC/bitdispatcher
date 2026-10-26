// Package pip is bitdispatcher's connector to PSC PIP, a separate and optional
// public-safety-communications personnel program. bitdispatcher is the system of record. This
// package defines, and documents, exactly what operational metadata bitdispatcher is willing to
// send to such a program, and nothing more.
//
// PSC PIP has four modules:
//
//	CEDE  - demand and exposure context (what the operational period looked like)
//	ARC   - after-action and readiness workflows
//	ROC   - recovery opportunity (breaks, overtime, consecutive duty)
//	RESET - support offers after qualifying events
//
// bitdispatcher sends only agency-approved structured operational metadata and references to its
// own source records. It never sends caller, victim or patient names, exact addresses, narratives,
// transcripts, or any clinical or wellness content. It never computes a wellness score, a readiness
// judgement, a discipline action or a diagnosis. That logic belongs to PSC PIP and stays there.
//
// No public PSC PIP API specification exists. This package is bitdispatcher's own documented
// contract. It has not been tested against a real PSC PIP system. See docs/PSC-PIP.md.
package pip

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Module identifiers.
const (
	ModCEDE  = "cede"
	ModARC   = "arc"
	ModROC   = "roc"
	ModRESET = "reset"
)

// Event classes bitdispatcher can send.
const (
	// ClassPeriod is a CEDE summary of one operational period: counts for incidents, critical
	// incidents, location exceptions, sustained-workload minutes, coordination contacts, open
	// follow-ups and mean exception resolution minutes.
	ClassPeriod = "period.summary"
	// ClassException is a forced or estimated location exception, sent to CEDE as operational
	// context only. It is never a performance item.
	ClassException = "location.exception"
	// ClassCritical is a critical incident. It goes to CEDE as demand and exposure context, and
	// also to RESET only when the agency has marked the event as qualifying.
	ClassCritical = "critical.incident"
	// ClassAfterAction is an ARC event, sent only when a supervisor opened an agency
	// after-action or readiness workflow.
	ClassAfterAction = "after.action"
	// ClassRecovery is a ROC event: a documented missed or delayed break, extended shift,
	// overtime, consecutive duty, or recovery action.
	ClassRecovery = "recovery.effect"
	// ClassSupport is a RESET event: a support offer opened or linked. It is an operational
	// action only.
	ClassSupport = "support.offer"
	// ClassCorrection is a versioned correction of an earlier event, in any module.
	ClassCorrection = "correction"
)

// Config is the agency's connection to PSC PIP, edited in the CAD's GUI.
type Config struct {
	On  bool   `json:"on"`
	URL string `json:"url"` // https only, except http://127.0.0.1 or localhost for testing
	// Token is sent as a Bearer token if set.
	Token string `json:"token,omitempty"`
	// Classes is the set of event classes the agency allows to cross. Empty means none.
	Classes []string `json:"classes"`
	// Identified, when false (the default), sends each person as a stable per-agency pseudonym
	// (HMAC-SHA256 of the username with Salt). When true, usernames are sent as given.
	Identified bool `json:"identified"`
	// Salt keys the pseudonym. It should be a per-agency secret.
	Salt string `json:"salt,omitempty"`
	// Aggregate, when true, sends only ClassPeriod. Everything else is dropped.
	Aggregate bool `json:"aggregate"`
}

// Ref is a reference to a CAD source record, never its content.
type Ref struct {
	Kind    string `json:"kind"` // incident, exception, critical, followup, period, break
	ID      string `json:"id"`
	Version int    `json:"version,omitempty"`
}

// Event is one message to PSC PIP.
type Event struct {
	// ID is stable and unique per event and version. The caller makes it.
	ID       string    `json:"id"`
	Class    string    `json:"class"`
	Modules  []string  `json:"modules"` // filled by Route
	Agency   string    `json:"agency,omitempty"`
	Period   string    `json:"period"` // stable operational-period identifier, e.g. "2026-10-25/day"
	At       time.Time `json:"at"`     // when it happened (source time, UTC)
	Version  int       `json:"version"`
	Corrects string    `json:"corrects,omitempty"` // the event ID this corrects
	// People holds usernames before Prepare, and pseudonyms after Prepare unless Identified.
	People []string `json:"people,omitempty"`
	// Roles are duty roles involved: calltaker, police, fire, ems, supervisor, trainee, trainer.
	Roles   []string           `json:"roles,omitempty"`
	Sources []Ref              `json:"sources"`
	Counts  map[string]float64 `json:"counts,omitempty"`
	// Facts holds short structured facts, e.g. "kind":"forced", "callType":"SHOTS",
	// "qualifying":"yes", "effect":"missed-break".
	Facts map[string]string `json:"facts,omitempty"`
}

// AllowedFacts lists the only fact keys that may cross to PSC PIP. Prepare strips everything else.
var AllowedFacts = []string{
	"kind", "callType", "priority", "qualifying", "effect",
	"status", "disposition", "trigger", "class", "duty", "minutes", "reason",
}

// Route sets e.Modules from the event class and facts. It does not change anything else.
//
// The routing is:
//
//	period.summary     -> cede
//	location.exception -> cede
//	critical.incident  -> cede, plus reset when Facts["qualifying"] == "yes"
//	after.action       -> arc
//	recovery.effect    -> roc
//	support.offer      -> reset
//	correction         -> the modules of the class named in Facts["class"], routed by these same rules
func Route(e *Event) {
	e.Modules = modulesFor(e.Class, e.Facts)
}

// modulesFor returns the modules for a class and its facts, following the routing rules. For a
// correction it looks at Facts["class"] to find the corrected class and routes that. A correction
// that names itself, or an unknown class, routes to nothing.
func modulesFor(class string, facts map[string]string) []string {
	switch class {
	case ClassPeriod:
		return []string{ModCEDE}
	case ClassException:
		return []string{ModCEDE}
	case ClassCritical:
		if facts["qualifying"] == "yes" {
			return []string{ModCEDE, ModRESET}
		}
		return []string{ModCEDE}
	case ClassAfterAction:
		return []string{ModARC}
	case ClassRecovery:
		return []string{ModROC}
	case ClassSupport:
		return []string{ModRESET}
	case ClassCorrection:
		corrected := facts["class"]
		if corrected == "" || corrected == ClassCorrection {
			return []string{}
		}
		return modulesFor(corrected, facts)
	default:
		return []string{}
	}
}

// allowedClass reports whether the agency allows this class to cross. A correction is allowed when
// the class it corrects (Facts["class"]) is in the allowed set.
func allowedClass(c Config, e Event) bool {
	class := e.Class
	if class == ClassCorrection {
		class = e.Facts["class"]
		if class == "" {
			return false
		}
	}
	for _, a := range c.Classes {
		if a == class {
			return true
		}
	}
	return false
}

var (
	// digitRun matches a run of 7 or more digits, which can carry a phone number or SSN.
	digitRun = regexp.MustCompile(`\d{7,}`)
)

// looksLikeContent reports whether a fact value looks like it carries content rather than a short
// structured fact. A value carries content when it holds a digit run of 7 or more, contains an '@',
// or has more than 8 words.
func looksLikeContent(v string) bool {
	if strings.Contains(v, "@") {
		return true
	}
	if digitRun.MatchString(v) {
		return true
	}
	if len(strings.Fields(v)) > 8 {
		return true
	}
	return false
}

// isAllowedFact reports whether key is in AllowedFacts.
func isAllowedFact(key string) bool {
	for _, a := range AllowedFacts {
		if a == key {
			return true
		}
	}
	return false
}

// pseudonym returns a stable per-agency pseudonym for a username, keyed by salt.
func pseudonym(username, salt string) string {
	mac := hmac.New(sha256.New, []byte(salt))
	mac.Write([]byte(username))
	return hex.EncodeToString(mac.Sum(nil))
}

// Prepare applies the agency's rules to one event and returns the event to send and whether it may
// be sent. ok is false when the class is not allowed, or when Aggregate mode drops it.
//
// Prepare routes the event, pseudonymises People unless Identified, and strips any Facts key that is
// not in AllowedFacts, any value longer than 64 characters, and any value that looks like it carries
// content. The names of dropped keys are recorded, as a comma-separated list of key names only, in
// Facts["_dropped"].
func Prepare(c Config, e Event) (Event, bool) {
	if !allowedClass(c, e) {
		return Event{}, false
	}
	// In aggregate mode only period summaries ever cross.
	if c.Aggregate && e.Class != ClassPeriod {
		return Event{}, false
	}

	Route(&e)

	// Pseudonymise people unless the agency sends usernames.
	if !c.Identified && len(e.People) > 0 {
		pp := make([]string, len(e.People))
		for i, u := range e.People {
			pp[i] = pseudonym(u, c.Salt)
		}
		e.People = pp
	}

	// Copy and clean the facts so the caller's map is never mutated.
	if len(e.Facts) > 0 {
		clean := make(map[string]string, len(e.Facts))
		var dropped []string
		for k, v := range e.Facts {
			if k == "_dropped" {
				continue
			}
			if !isAllowedFact(k) || len(v) > 64 || looksLikeContent(v) {
				dropped = append(dropped, k)
				continue
			}
			clean[k] = v
		}
		if len(dropped) > 0 {
			sortStrings(dropped)
			clean["_dropped"] = strings.Join(dropped, ",")
		}
		if len(clean) == 0 {
			clean = nil
		}
		e.Facts = clean
	}

	return e, true
}

// sortStrings sorts a small slice of strings in place, so the _dropped list is stable without
// pulling in the sort package for one use. Insertion sort is fine for the handful of fact keys here.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// CheckURL validates the connection URL. https is always allowed. http is allowed only for the
// loopback host 127.0.0.1 or localhost, for testing against a local receiver.
func CheckURL(u string) error {
	if strings.TrimSpace(u) == "" {
		return errors.New("pip: url is empty")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("pip: url is not valid: %w", err)
	}
	host := parsed.Hostname()
	switch parsed.Scheme {
	case "https":
		if host == "" {
			return errors.New("pip: https url has no host")
		}
		return nil
	case "http":
		if host == "127.0.0.1" || host == "localhost" {
			return nil
		}
		return errors.New("pip: http is allowed only for 127.0.0.1 or localhost")
	default:
		return fmt.Errorf("pip: url scheme %q is not allowed, use https", parsed.Scheme)
	}
}

// Client sends prepared events to PSC PIP.
type Client struct {
	Config Config
	HTTP   *http.Client
}

// Send POSTs the events as JSON {"events":[...]} to Config.URL. It sets Content-Type to
// application/json and, when a token is set, an Authorization: Bearer header. It refuses, with an
// error and no request, when the connection is off or the URL is not valid. A 2xx response is a
// success. For any other status it returns the status code and an error that includes up to 200
// bytes of the response body.
func (c *Client) Send(ctx context.Context, evs []Event) (int, error) {
	if !c.Config.On {
		return 0, errors.New("pip: connection is off")
	}
	if err := CheckURL(c.Config.URL); err != nil {
		return 0, err
	}

	body, err := json.Marshal(map[string][]Event{"events": evs})
	if err != nil {
		return 0, fmt.Errorf("pip: cannot encode events: %w", err)
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Config.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("pip: cannot build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Config.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Config.Token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("pip: send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}

	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	return resp.StatusCode, fmt.Errorf("pip: receiver returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
}
