package usecase

// TEC-1208: phone book of the mobile caller ID. The app keeps a copy of the
// customers in scope (phone + label) on the device so the phone shows who is
// calling. The whole list is one snapshot with a version hash: the app sends
// the version it holds and gets an empty answer while nothing changed, so
// removed, anonymized and unlinked customers drop out without tombstones.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// CallerIDMaxEntries caps one snapshot; a larger scope is cut (lowest phone
// numbers first) and reported with Truncated.
const CallerIDMaxEntries = 50000

// callerIDLabelMax is the longest label, in characters.
const callerIDLabelMax = 80

// CallerIDEntry is one phone (E.164 with the plus sign) and what to show.
type CallerIDEntry struct {
	Phone string `json:"phone"`
	Label string `json:"label"`
}

// CallerIDSnapshot is the answer of GET /v1/customers/caller-id.
type CallerIDSnapshot struct {
	Version   string          `json:"version"`
	Unchanged bool            `json:"unchanged"`
	Truncated bool            `json:"truncated"`
	Count     int             `json:"count"`
	Entries   []CallerIDEntry `json:"entries"`
}

// CallerIDSnapshot lists the customers of the scope for the caller ID.
// knownVersion is the version the device holds; when it still matches, the
// entries are left out (Unchanged).
func (s *Service) CallerIDSnapshot(ctx context.Context, c Caller, knownVersion string) (CallerIDSnapshot, error) {
	rows, err := s.q.ListCallerIDEntries(ctx, db.ListCallerIDEntriesParams{
		OrgIds: c.orgIDs(), BrandID: c.brand(), LimitCount: CallerIDMaxEntries + 1,
	})
	if err != nil {
		return CallerIDSnapshot{}, fmt.Errorf("customers: caller id: %w", err)
	}
	return buildCallerIDSnapshot(rows, knownVersion, CallerIDMaxEntries), nil
}

func buildCallerIDSnapshot(rows []db.ListCallerIDEntriesRow, knownVersion string, limit int) CallerIDSnapshot {
	out := CallerIDSnapshot{Entries: []CallerIDEntry{}}
	if len(rows) > limit {
		rows, out.Truncated = rows[:limit], true
	}
	entries := make([]CallerIDEntry, 0, len(rows))
	h := sha256.New()
	for _, r := range rows {
		label := callerIDLabel(r.Name, r.Surname, r.CompanyName)
		if r.Phone == "" || label == "" {
			continue
		}
		entries = append(entries, CallerIDEntry{Phone: r.Phone, Label: label})
		h.Write([]byte(r.Phone))
		h.Write([]byte{'\t'})
		h.Write([]byte(label))
		h.Write([]byte{'\n'})
	}
	out.Count = len(entries)
	out.Version = hex.EncodeToString(h.Sum(nil))[:32]
	if knownVersion != "" && knownVersion == out.Version {
		out.Unchanged = true
		return out
	}
	out.Entries = entries
	return out
}

// callerIDLabel is "Name Surname", with the company in brackets when there
// is one ("Company" alone when the person has no name). Whitespace is
// collapsed (the app stores one entry per line) and the label is cut to
// callerIDLabelMax characters.
func callerIDLabel(name, surname, company string) string {
	person := strings.Join(strings.Fields(name+" "+surname), " ")
	company = strings.Join(strings.Fields(company), " ")
	label := person
	switch {
	case person == "":
		label = company
	case company != "" && !strings.EqualFold(company, person):
		label = person + " (" + company + ")"
	}
	if r := []rune(label); len(r) > callerIDLabelMax {
		label = strings.TrimSpace(string(r[:callerIDLabelMax-1])) + "…"
	}
	return label
}
