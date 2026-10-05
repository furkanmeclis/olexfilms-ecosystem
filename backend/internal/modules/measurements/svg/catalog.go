// Package svg is the Go port of the legacy hub's NexPTG part map
// (olexfilms app/Services/Nexptg: NexptgBodyModelCatalog,
// NexptgSvgFillService, ServicePdfDetailsBuilder::buildVisualizationCards).
// TEC-298 (F3-02f).
//
// The body shapes are the hub's pointed SVGs and manifests
// (resources/nexptg-models), embedded unchanged under assets/<body type id>
// (design.md: "mevcut SVG'ler korunur"). The interpretation colors and the
// averaging rule are constants identical to NexptgInterpretationEnum. The
// output is byte-for-byte the PHP output (testdata golden files).
package svg

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

//go:embed assets
var assets embed.FS

// ErrBodyTypeUnknown is returned when a body type does not resolve to a
// shipped model.
var ErrBodyTypeUnknown = errors.New("svg: unknown NexPTG body type")

// ErrPartUnknown is returned when a body type has no SVG for a part.
var ErrPartUnknown = errors.New("svg: NexPTG part SVG not found")

// Places in NexptgPlaceIdEnum order.
var Places = []string{"left", "right", "top", "back"}

// Model is one body type of the catalog (index.json models).
type Model struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Folder   string `json:"folder"`
	Bodywork string `json:"bodywork"`
}

// Point is a measurement point of an element part on the canvas.
type Point struct {
	ID    string      `json:"id"`
	Count *int        `json:"count"`
	X     json.Number `json:"x"`
	Y     json.Number `json:"y"`
}

// Asset is one part SVG of a body type (svg_manifest.json assets).
type Asset struct {
	Part   string  `json:"part"`
	Place  string  `json:"place"`
	Kind   string  `json:"kind"`
	Points []Point `json:"points"`
}

// Detail is a body type manifest.
type Detail struct {
	Model
	PointRadius json.Number `json:"pointRadius"`
	Assets      []Asset     `json:"assets"`
}

type catalogFile struct {
	Models []Model `json:"models"`
}

var (
	catalogOnce sync.Once
	catalogErr  error
	models      []Model
	detailMu    sync.Mutex
	details     = map[string]*Detail{}
)

func loadCatalog() ([]Model, error) {
	catalogOnce.Do(func() {
		raw, err := assets.ReadFile("assets/catalog.json")
		if err != nil {
			catalogErr = err
			return
		}
		var f catalogFile
		if err := json.Unmarshal(raw, &f); err != nil {
			catalogErr = err
			return
		}
		models = f.Models
	})
	return models, catalogErr
}

// Models lists the body types in catalog order.
func Models() ([]Model, error) {
	m, err := loadCatalog()
	if err != nil {
		return nil, err
	}
	return append([]Model(nil), m...), nil
}

// ResolveID is NexptgBodyModelCatalog::resolveId: an exact model id, or a
// case-insensitive folder / name / bodywork match (e.g. "SEDAN").
func ResolveID(bodyType string) (string, bool) {
	if bodyType == "" {
		return "", false
	}
	list, err := loadCatalog()
	if err != nil {
		return "", false
	}
	for _, m := range list {
		if m.ID == bodyType {
			return m.ID, true
		}
	}
	for _, m := range list {
		if strings.EqualFold(m.Folder, bodyType) || strings.EqualFold(m.Name, bodyType) || strings.EqualFold(m.Bodywork, bodyType) {
			return m.ID, true
		}
	}
	return "", false
}

// BodyTypeDetail returns the manifest of a model id.
func BodyTypeDetail(id string) (*Detail, error) {
	list, err := loadCatalog()
	if err != nil {
		return nil, err
	}
	var model *Model
	for i := range list {
		if list[i].ID == id {
			model = &list[i]
			break
		}
	}
	if model == nil {
		return nil, fmt.Errorf("%w: %s", ErrBodyTypeUnknown, id)
	}
	detailMu.Lock()
	defer detailMu.Unlock()
	if d, ok := details[id]; ok {
		return d, nil
	}
	raw, err := assets.ReadFile("assets/" + id + "/svg_manifest.json")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBodyTypeUnknown, id)
	}
	d := &Detail{}
	if err := json.Unmarshal(raw, d); err != nil {
		return nil, err
	}
	d.Model = *model
	details[id] = d
	return d, nil
}

// svgContents reads one pointed SVG of a body type.
func svgContents(id, part string) (string, error) {
	if part == "" || strings.ContainsAny(part, "/\\.") {
		return "", ErrPartUnknown
	}
	raw, err := assets.ReadFile("assets/" + id + "/" + part + ".svg")
	if err != nil {
		return "", fmt.Errorf("%w: %s/%s", ErrPartUnknown, id, part)
	}
	return string(raw), nil
}
