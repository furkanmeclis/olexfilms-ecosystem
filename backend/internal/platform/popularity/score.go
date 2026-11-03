// Package popularity is the lookup popularity engine (TEC-1043): it orders
// the picker lists (car brands, car models) by how much they are used,
// mixing the caller organization's usage with the usage of every
// organization of the same domain brand (K20). Port of the legacy
// LookupPopularityService. Scores only ever change the order of a list; no
// count or score of another organization leaves this package's callers.
package popularity

import (
	"slices"
	"sort"
	"strings"
)

// Weights of the mixed score (legacy lookups.popularity.dealer_weight /
// global_weight, fixed here). A caller without an organization gets the
// general score only (weight 1).
const (
	OrgWeight    = 0.6
	GlobalWeight = 0.4
)

// Counts maps a row id (car brand / model id) to its usage count.
type Counts map[int64]int64

// Scores mixes the counts into a 0..1 score per id: OrgWeight × org count /
// max org count + GlobalWeight × general count / max general count, each max
// at least 1. hasOrg false (no organization) scores the general count alone.
// Ids missing from both maps have no entry (score 0). Pure function.
func Scores(org, global Counts, hasOrg bool) map[int64]float64 {
	orgW, globalW := OrgWeight, GlobalWeight
	if !hasOrg {
		orgW, globalW, org = 0, 1, nil
	}
	out := make(map[int64]float64, len(global)+len(org))
	maxOrg, maxGlobal := maxCount(org), maxCount(global)
	for id := range global {
		out[id] = 0
	}
	for id := range org {
		out[id] = 0
	}
	for id := range out {
		out[id] = orgW*float64(org[id])/maxOrg + globalW*float64(global[id])/maxGlobal
	}
	return out
}

func maxCount(c Counts) float64 {
	m := int64(1)
	for _, n := range c {
		if n > m {
			m = n
		}
	}
	return float64(m)
}

// Entry is one rankable row.
type Entry struct {
	ID   int64
	Name string
}

// Rank orders entries by score (highest first), then case-insensitive name,
// then id (docs/list-contract.md tiebreak); the input is left untouched.
// Mirrors the SQL popularity sort of the catalog lists (score DESC,
// lower(name), id).
func Rank(entries []Entry, scores map[int64]float64) []Entry {
	out := append([]Entry(nil), entries...)
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := scores[out[i].ID], scores[out[j].ID]
		if si != sj {
			return si > sj
		}
		ni, nj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if ni != nj {
			return ni < nj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Arrays flattens scores into the parallel id / score arrays the catalog
// list queries take (pop_ids, pop_scores), ids ascending so the arguments are
// stable. Empty scores give nil arrays (every row scores 0).
func Arrays(scores map[int64]float64) ([]int64, []float64) {
	if len(scores) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	vals := make([]float64, len(ids))
	for i, id := range ids {
		vals[i] = scores[id]
	}
	return ids, vals
}
