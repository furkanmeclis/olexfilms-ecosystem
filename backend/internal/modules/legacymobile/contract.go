package legacymobile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FixtureDir is the contract fixture folder, relative to this package.
const FixtureDir = "testdata/legacy_mobile"

// Fixture is one contract fixture (testdata/legacy_mobile/<name>.json): the
// request the old app sends and the answer it relies on, both taken from
// the old hub (source names the controller, request and resource the shape
// comes from). {{var}} placeholders are filled by LoadFixture.
type Fixture struct {
	Name    string `json:"name"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Adapts  string `json:"adapts"`
	Source  string `json:"source"`
	Request struct {
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	} `json:"request"`
	Response struct {
		Status int `json:"status"`
		// Message is the old envelope's message (a string, or null).
		Message json.RawMessage `json:"message"`
		// DataKeys are the keys of the envelope's data object; DataNull
		// says data is null (push token answers).
		DataKeys []string `json:"data_keys"`
		DataNull bool     `json:"data_null"`
		// NestedKeys maps a dotted path inside data ("user", "data.0",
		// "meta") to the keys the object there must have.
		NestedKeys map[string][]string `json:"nested_keys"`
	} `json:"response"`
}

// LoadFixture reads dir/<name>.json and replaces every {{key}} with vars.
// An unfilled placeholder is an error, so a fixture never reaches the
// server half filled.
func LoadFixture(dir, name string, vars map[string]string) (Fixture, error) {
	var f Fixture
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return f, err
	}
	s := string(b)
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	if i := strings.Index(s, "{{"); i >= 0 && vars != nil {
		end := strings.Index(s[i:], "}}")
		if end < 0 {
			end = len(s) - i - 2
		}
		return f, fmt.Errorf("fixture %s: unfilled placeholder %s", name, s[i:i+end+2])
	}
	if err := json.Unmarshal([]byte(s), &f); err != nil {
		return f, fmt.Errorf("fixture %s: %w", name, err)
	}
	return f, nil
}

// MissingKeys returns the fixture's data_keys absent from the top level of
// data (the envelope's "data" object).
func (f Fixture) MissingKeys(data json.RawMessage) []string {
	return missing(data, f.Response.DataKeys)
}

func missing(obj json.RawMessage, keys []string) []string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(obj, &m); err != nil {
		return keys
	}
	var out []string
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}

// at walks a dotted path ("data.0.customer") through objects and arrays.
func at(v json.RawMessage, path string) (json.RawMessage, bool) {
	for _, seg := range strings.Split(path, ".") {
		if i, err := strconv.Atoi(seg); err == nil {
			var arr []json.RawMessage
			if json.Unmarshal(v, &arr) != nil || i >= len(arr) {
				return nil, false
			}
			v = arr[i]
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(v, &m) != nil {
			return nil, false
		}
		next, ok := m[seg]
		if !ok {
			return nil, false
		}
		v = next
	}
	return v, true
}

// LegacyBody is the old envelope as the app reads it.
type LegacyBody struct {
	Success *bool               `json:"success"`
	Message json.RawMessage     `json:"message"`
	Data    json.RawMessage     `json:"data"`
	Errors  map[string][]string `json:"errors"`
	Code    string              `json:"code"`
}

// Check verifies an answer against the fixture: the status, the old success
// envelope, the message, the data keys and the nested keys.
func (f Fixture) Check(status int, body []byte) (LegacyBody, error) {
	var b LegacyBody
	if err := json.Unmarshal(body, &b); err != nil {
		return b, fmt.Errorf("%s: body is not JSON: %s", f.Name, body)
	}
	if status != f.Response.Status {
		return b, fmt.Errorf("%s %s = %d, want %d: %s", f.Method, f.Path, status, f.Response.Status, body)
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(body, &raw)
	for _, k := range []string{"success", "message", "data"} {
		if _, ok := raw[k]; !ok {
			return b, fmt.Errorf("%s: envelope misses %q: %s", f.Name, k, body)
		}
	}
	if _, ok := raw["error"]; ok {
		return b, fmt.Errorf("%s: new error envelope in a legacy answer: %s", f.Name, body)
	}
	if b.Success == nil || !*b.Success {
		return b, fmt.Errorf("%s: success is not true: %s", f.Name, body)
	}
	if len(f.Response.Message) > 0 && !bytes.Equal(bytes.TrimSpace(b.Message), bytes.TrimSpace(f.Response.Message)) {
		return b, fmt.Errorf("%s: message = %s, want %s", f.Name, b.Message, f.Response.Message)
	}
	if f.Response.DataNull {
		if string(bytes.TrimSpace(b.Data)) != "null" {
			return b, fmt.Errorf("%s: data = %s, want null", f.Name, b.Data)
		}
		return b, nil
	}
	if m := f.MissingKeys(b.Data); len(m) > 0 {
		return b, fmt.Errorf("%s: data misses %v: %s", f.Name, m, b.Data)
	}
	for path, keys := range f.Response.NestedKeys {
		v, ok := at(b.Data, path)
		if !ok {
			return b, fmt.Errorf("%s: data has no %s: %s", f.Name, path, b.Data)
		}
		if m := missing(v, keys); len(m) > 0 {
			return b, fmt.Errorf("%s: data.%s misses %v: %s", f.Name, path, m, v)
		}
	}
	return b, nil
}
