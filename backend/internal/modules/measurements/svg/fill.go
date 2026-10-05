package svg

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// Interpretation levels (NexptgInterpretationEnum, NexPTG
// Interpretation.getInterpretation()).
const (
	Unknown        = -1
	TooThin        = 0
	Original       = 1
	SecondLayer    = 2
	ThinPutty      = 3
	ThickPutty     = 4
	ThickPuttyHigh = 5
)

// colors are NexptgInterpretationEnum::fillColor (APK color_interpretations).
var colors = map[int]string{
	Unknown:        "#9CA3AF",
	TooThin:        "#e9df28",
	Original:       "#55d37a",
	SecondLayer:    "#deb50a",
	ThinPutty:      "#ec4a08",
	ThickPutty:     "#af0025",
	ThickPuttyHigh: "#af0025",
}

// FromValue is NexptgInterpretationEnum::fromValue: nil and unknown codes
// are Unknown.
func FromValue(v *int) int {
	if v == nil {
		return Unknown
	}
	if _, ok := colors[*v]; ok {
		return *v
	}
	return Unknown
}

// FillColor is the color of an interpretation level.
func FillColor(level int) string {
	if c, ok := colors[level]; ok {
		return c
	}
	return colors[Unknown]
}

// Reading is one measurement point of a report (measurement_values row).
// Readings are passed in insertion order: a later reading of the same
// part and position wins, like the PHP mapWithKeys.
type Reading struct {
	PartType       string
	Position       *int
	Interpretation *int
}

// Report is the data the part map needs.
type Report struct {
	BodyType string // measurement_results.body_type (model id or bodywork)
	Readings []Reading
}

// Filler colors the SVGs of one report.
type Filler struct {
	detail   *Detail
	readings []Reading
}

// NewFiller resolves the body type of a report.
func NewFiller(r Report) (*Filler, error) {
	id, ok := ResolveID(r.BodyType)
	if !ok {
		return nil, ErrBodyTypeUnknown
	}
	d, err := BodyTypeDetail(id)
	if err != nil {
		return nil, err
	}
	return &Filler{detail: d, readings: r.Readings}, nil
}

// Detail returns the resolved body type manifest.
func (f *Filler) Detail() *Detail { return f.detail }

func (f *Filler) partReadings(part string) []Reading {
	var out []Reading
	for _, r := range f.readings {
		if r.PartType == part {
			out = append(out, r)
		}
	}
	return out
}

// AveragePartInterpretation is the rounded mean level of a part's readings
// (null and -1 excluded), clamped to 0..5; ok is false without readings.
func (f *Filler) AveragePartInterpretation(part string) (int, bool) {
	sum, n := 0, 0
	for _, r := range f.partReadings(part) {
		if r.Interpretation == nil || FromValue(r.Interpretation) == Unknown {
			continue
		}
		sum += *r.Interpretation
		n++
	}
	if n == 0 {
		return 0, false
	}
	rounded := int(math.Round(float64(sum) / float64(n)))
	rounded = max(rounded, TooThin)
	rounded = min(rounded, ThickPuttyHigh)
	return FromValue(&rounded), true
}

type positionLevel struct {
	position int
	level    int
}

// measurementsByPosition keeps the PHP array order: first insertion of a
// position fixes its place, a later reading overwrites the level.
func (f *Filler) measurementsByPosition(part string) []positionLevel {
	var out []positionLevel
	index := map[int]int{}
	for _, r := range f.partReadings(part) {
		if r.Position == nil {
			continue
		}
		level := FromValue(r.Interpretation)
		if i, ok := index[*r.Position]; ok {
			out[i].level = level
			continue
		}
		index[*r.Position] = len(out)
		out = append(out, positionLevel{position: *r.Position, level: level})
	}
	return out
}

func levelAt(list []positionLevel, position int) (int, bool) {
	for _, p := range list {
		if p.position == position {
			return p.level, true
		}
	}
	return 0, false
}

// FillPart is NexptgSvgFillService::fillPart: the part SVG with the point
// circles in their interpretation color and the part body in the average
// color. A part without readings is returned unchanged.
func (f *Filler) FillPart(part string) (string, error) {
	src, err := svgContents(f.detail.ID, part)
	if err != nil {
		return "", err
	}
	byPosition := f.measurementsByPosition(part)
	bg := ""
	if level, ok := f.AveragePartInterpretation(part); ok {
		bg = FillColor(level)
	}
	return applyFills(src, part, byPosition, bg)
}

func applyFills(src, part string, byPosition []positionLevel, bg string) (string, error) {
	if len(byPosition) == 0 && bg == "" {
		return src, nil
	}
	doc, err := parseXML(src)
	if err != nil {
		// The PHP regex fallback is never reached by the shipped SVGs.
		return "", err
	}
	if bg != "" {
		applyBackgroundFill(doc, part, bg)
	}
	for _, p := range byPosition {
		group := doc.byID(part + "_point_" + strconv.Itoa(p.position))
		if group == nil {
			continue
		}
		color := FillColor(p.level)
		group.walk(func(c *node) {
			if c.name == "circle" {
				c.setAttr("fill", color)
				c.setAttr("stroke", color)
			}
		})
	}
	return serialize(doc.root), nil
}

func isWhiteFill(v string) bool {
	switch v {
	case "#FFFFFF", "#ffffff", "#FFF", "#fff", "white":
		return true
	}
	return false
}

// applyBackgroundFill is //*[@id=part]//*[@fill=white...] outside the
// point groups.
func applyBackgroundFill(doc *document, part, color string) {
	seen := map[*node]bool{}
	for _, holder := range doc.allByID(part) {
		holder.walk(func(c *node) {
			if seen[c] {
				return
			}
			seen[c] = true
			if v, ok := c.attr("fill"); !ok || !isWhiteFill(v) {
				return
			}
			if isInsidePointsGroup(c, part) {
				return
			}
			c.setAttr("fill", color)
		})
	}
}

func isInsidePointsGroup(n *node, part string) bool {
	for cur := n.parent; cur != nil; cur = cur.parent {
		id, _ := cur.attr("id")
		if id == part+"_points" || strings.Contains(id, "_point_") {
			return true
		}
		if id == part {
			return false
		}
	}
	return false
}

// extractPartBodyMarkup is the part body of a filled part SVG without the
// point groups and metadata.
func extractPartBodyMarkup(filled, part string) (string, error) {
	doc, err := parseXML(filled)
	if err != nil {
		return "", err
	}
	holder := doc.byID(part)
	if holder == nil {
		return "", nil
	}
	var b strings.Builder
	for _, c := range holder.children {
		if c.kind != elementNode {
			continue
		}
		id, _ := c.attr("id")
		if id == part+"_points" || strings.Contains(id, "_point_") {
			continue
		}
		if strings.ToLower(c.name) == "metadata" {
			continue
		}
		b.WriteString(serialize(c))
		b.WriteString("\n")
	}
	return b.String(), nil
}

// phpFloat is PHP's (string) cast of a float (precision=14).
func phpFloat(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	s := strconv.FormatFloat(v, 'g', 14, 64)
	if i := strings.IndexByte(s, 'e'); i >= 0 {
		mant, exp := s[:i], s[i+1:]
		if !strings.Contains(mant, ".") {
			mant += ".0"
		}
		sign := "+"
		if exp[0] == '-' || exp[0] == '+' {
			if exp[0] == '-' {
				sign = "-"
			}
			exp = exp[1:]
		}
		exp = strings.TrimLeft(exp, "0")
		if exp == "" {
			exp = "0"
		}
		return mant + "E" + sign + exp
	}
	return s
}

// numberString is (string) of a JSON number decoded by PHP (int stays int,
// a fraction becomes a float).
func numberString(n string) (string, bool) {
	if n == "" {
		return "", false
	}
	if i, err := strconv.ParseInt(n, 10, 64); err == nil {
		return strconv.FormatInt(i, 10), true
	}
	f, err := strconv.ParseFloat(n, 64)
	if err != nil {
		return "", false
	}
	return phpFloat(f), true
}

// esc is htmlspecialchars($s, ENT_QUOTES).
var phpEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#039;")

func esc(s string) string { return phpEscaper.Replace(s) }

// ComposePlaceView is NexptgSvgFillService::composePlaceView: the main
// view of a place with every element part body in its average color
// (opacity 0.72) and the measurement points on top. ok is false when the
// body type has no main view for the place.
func (f *Filler) ComposePlaceView(place string) (string, bool, error) {
	var main *Asset
	for i := range f.detail.Assets {
		a := &f.detail.Assets[i]
		if a.Place == place && a.Kind == "main" {
			main = a
			break
		}
	}
	if main == nil || main.Part == "" {
		return "", false, nil
	}
	base, err := svgContents(f.detail.ID, main.Part)
	if err != nil {
		return "", false, nil //nolint:nilerr // PHP returns null for a missing main SVG
	}
	radius := 22.0
	if f.detail.PointRadius != "" {
		if r, err := strconv.ParseFloat(string(f.detail.PointRadius), 64); err == nil {
			radius = r
		} else {
			radius = 0
		}
	}
	var overlays, points strings.Builder
	for _, a := range f.detail.Assets {
		if a.Place != place || a.Kind != "element" || a.Part == "" {
			continue
		}
		filled, err := f.FillPart(a.Part)
		switch {
		case err == nil:
			body, err := extractPartBodyMarkup(filled, a.Part)
			if err != nil {
				return "", false, err
			}
			if body != "" {
				p := esc(a.Part)
				overlays.WriteString(`    <g id="nexptg_part_body_` + p + `" data-part="` + p + `" opacity="0.72">` + "\n")
				overlays.WriteString(body)
				overlays.WriteString("\n    </g>\n")
			}
		case errors.Is(err, ErrPartUnknown):
			// no part SVG: points only
		default:
			return "", false, err
		}
		byPosition := f.measurementsByPosition(a.Part)
		for _, pt := range a.Points {
			if pt.Count == nil || *pt.Count < 1 {
				continue
			}
			cx, okX := numberString(string(pt.X))
			cy, okY := numberString(string(pt.Y))
			if !okX || !okY {
				continue
			}
			count := *pt.Count
			level, ok := levelAt(byPosition, count)
			if !ok {
				level = Unknown
			}
			fill := esc(FillColor(level))
			label := strconv.Itoa(count)
			points.WriteString(`    <g id="` + esc(a.Part+"_point_"+label) + `" data-part="` + esc(a.Part) + `" data-count="` + label + `">` + "\n")
			points.WriteString(`      <circle cx="` + esc(cx) + `" cy="` + esc(cy) + `" r="` + esc(phpFloat(radius)) + `" fill="` + fill + `" stroke="` + fill + `" stroke-width="2"/>` + "\n")
			points.WriteString(`      <text x="` + esc(cx) + `" y="` + esc(cy) + `" text-anchor="middle" dominant-baseline="central" font-family="Arial, Helvetica, sans-serif" font-size="19" font-weight="700" fill="#111827">` + label + `</text>` + "\n")
			points.WriteString("    </g>\n")
		}
	}
	if overlays.Len() == 0 && points.Len() == 0 {
		return base, true, nil
	}
	overlay := `<g id="nexptg_measurement_overlay">` + "\n" + overlays.String()
	if points.Len() > 0 {
		overlay += `    <g id="nexptg_measurement_points">` + "\n" + points.String() + "    </g>\n"
	}
	overlay += "  </g>"
	if strings.Contains(base, "</svg>") {
		return strings.ReplaceAll(base, "</svg>", overlay+"</svg>"), true, nil
	}
	return base + overlay, true, nil
}

// Card is one picture of the PDF part map.
type Card struct {
	Type  string // "main" (composite view) or "part"
	Place string
	Part  string
	SVG   string
}

// Cards is ServicePdfDetailsBuilder::buildVisualizationCards: the
// composite views in place order, then the element parts of each place in
// manifest order. An unresolvable body type gives no cards.
func Cards(r Report) ([]Card, error) {
	f, err := NewFiller(r)
	if errors.Is(err, ErrBodyTypeUnknown) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Card
	for _, place := range Places {
		var main *Asset
		for i := range f.detail.Assets {
			if a := &f.detail.Assets[i]; a.Place == place && a.Kind == "main" {
				main = a
				break
			}
		}
		if main == nil {
			continue
		}
		svg, ok, err := f.ComposePlaceView(place)
		if err != nil {
			return nil, err
		}
		if !ok || svg == "" {
			continue
		}
		out = append(out, Card{Type: "main", Place: place, Part: main.Part, SVG: svg})
	}
	for _, place := range Places {
		for _, a := range f.detail.Assets {
			if a.Place != place || a.Kind != "element" || a.Part == "" {
				continue
			}
			svg, err := f.FillPart(a.Part)
			if errors.Is(err, ErrPartUnknown) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out = append(out, Card{Type: "part", Place: place, Part: a.Part, SVG: svg})
		}
	}
	return out, nil
}
