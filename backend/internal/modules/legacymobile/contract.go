package legacymobile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FixtureDir is the contract fixture folder, relative to this package.
const FixtureDir = "testdata/legacy_mobile"

// Fixture is one contract fixture (testdata/legacy_mobile/<name>.json): the
// alias request and the answer the old app relies on. {{var}} placeholders
// are filled by LoadFixture.
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
		Status   int      `json:"status"`
		DataKeys []string `json:"data_keys"`
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
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return f.Response.DataKeys
	}
	var out []string
	for _, k := range f.Response.DataKeys {
		if _, ok := obj[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}
