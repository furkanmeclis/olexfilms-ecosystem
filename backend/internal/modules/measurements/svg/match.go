package svg

import (
	"strconv"
	"strings"
)

// bodyTypeAliases maps a NexPTG typeOfBody the catalog's bodywork does not
// spell the same way to the bodywork it means (TEC-1010). The device sends
// "ESTATE (STATION WAGON)"; the hub's catalog names it "ESTATE". Keys are
// upper case.
var bodyTypeAliases = map[string]string{
	"ESTATE (STATION WAGON)": "ESTATE",
	"STATION WAGON":          "ESTATE",
}

// aliasOf is the catalog bodywork of a body type alias ("" when none).
func aliasOf(bodyType string) string {
	return bodyTypeAliases[strings.ToUpper(strings.TrimSpace(bodyType))]
}

// Unmatched is a reading that cannot be placed on the part map.
type Unmatched struct {
	Reason   string // "body_type", "part" or "position"
	PartType string
	Position *int
}

// Unmatched reasons.
const (
	UnmatchedBodyType = "body_type"
	UnmatchedPart     = "part"
	UnmatchedPosition = "position"
)

// UnmatchedReadings lists the readings of a report that do not land on a
// numbered point of its body type: the body type does not resolve, the
// body type has no such part, or the part has no point of the position.
// A reading without a position is not a point reading and is not listed.
func UnmatchedReadings(r Report) []Unmatched {
	var out []Unmatched
	id, ok := ResolveID(r.BodyType)
	if !ok {
		for _, rd := range r.Readings {
			out = append(out, Unmatched{Reason: UnmatchedBodyType, PartType: rd.PartType, Position: rd.Position})
		}
		return out
	}
	d, err := BodyTypeDetail(id)
	if err != nil {
		return nil
	}
	points := map[string]map[int]bool{}
	for _, a := range d.Assets {
		if a.Kind != "element" {
			continue
		}
		set := map[int]bool{}
		for _, p := range a.Points {
			if p.Count != nil {
				set[*p.Count] = true
			}
		}
		points[a.Part] = set
	}
	for _, rd := range r.Readings {
		set, ok := points[rd.PartType]
		switch {
		case !ok:
			out = append(out, Unmatched{Reason: UnmatchedPart, PartType: rd.PartType, Position: rd.Position})
		case rd.Position != nil && !set[*rd.Position]:
			out = append(out, Unmatched{Reason: UnmatchedPosition, PartType: rd.PartType, Position: rd.Position})
		}
	}
	return out
}

// String is "reason:PART#position" for reports.
func (u Unmatched) String() string {
	s := u.Reason + ":" + u.PartType
	if u.Position != nil {
		s += "#" + strconv.Itoa(*u.Position)
	}
	return s
}
