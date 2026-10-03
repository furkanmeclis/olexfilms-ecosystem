// Package migrations embeds the golang-migrate files of this directory so a
// binary knows the schema version it was built for (TEC-275 cutover
// preflight compares it with schema_migrations). golang-migrate and sqlc
// read only the *.sql files; this Go file is ignored by both.
package migrations

import (
	"embed"
	"errors"
	"io/fs"
	"regexp"
	"strconv"
)

//go:embed *.up.sql
var files embed.FS

// upFile matches golang-migrate up files: <version>_<title>.up.sql.
var upFile = regexp.MustCompile(`^([0-9]+)_.+\.up\.sql$`)

// LatestVersion is the highest migration version embedded in the binary.
func LatestVersion() (uint64, error) {
	return latestVersion(files)
}

func latestVersion(fsys fs.FS) (uint64, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return 0, err
	}
	var latest uint64
	for _, e := range entries {
		m := upFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return 0, err
		}
		if v > latest {
			latest = v
		}
	}
	if latest == 0 {
		return 0, errors.New("migrations: no up migration embedded")
	}
	return latest, nil
}
