// Package source is the read-only view of a legacy database (TEC-252).
//
// Legacy systems are read-only sources (AGENTS §4, design §7): the migrator
// reads them and writes only to the new Postgres. The guarantee is enforced
// twice:
//
//  1. CheckReadOnly refuses anything but a single SELECT / WITH statement
//     before it is sent.
//  2. Every query runs inside a READ ONLY transaction on the server (MariaDB
//     START TRANSACTION READ ONLY, Postgres BEGIN READ ONLY), so even a
//     statement that slipped past the guard is rejected by the database.
//
// Step queries use the portable SQL subset both engines understand: '?'
// placeholders (rewritten to $n for Postgres), unqualified table names (the
// Postgres adapter pins search_path to the fixture schema), standard joins,
// no engine-specific functions or operators.
package source

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// LegacySource reads one legacy database.
type LegacySource interface {
	// Name is the source system key stored in migration_map ("hub", "wh").
	Name() string
	// Query runs a guarded, read-only SELECT. args bind to '?' placeholders.
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	// TableExists reports whether table exists in the source database.
	TableExists(ctx context.Context, table string) (bool, error)
	Close() error
}

// Rows is a forward-only result set. Close must always be called; it ends the
// read-only transaction the query ran in.
type Rows interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

// ErrBadTableName is returned by TableExists for a name that is not a plain
// identifier.
var ErrBadTableName = errors.New("source: invalid table name")

func validTableName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'); !ok {
			return false
		}
	}
	return true
}

// rewritePlaceholders turns '?' placeholders into $1..$n, leaving quoted
// strings, quoted identifiers and comments untouched.
func rewritePlaceholders(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			j := i + 1
			for ; j < len(q); j++ {
				if q[j] == '\\' && c == '\'' {
					j++
					continue
				}
				if q[j] == c {
					break
				}
			}
			if j >= len(q) {
				j = len(q) - 1
			}
			b.WriteString(q[i : j+1])
			i = j
		case c == '-' && i+1 < len(q) && q[i+1] == '-':
			j := strings.IndexByte(q[i:], '\n')
			if j < 0 {
				b.WriteString(q[i:])
				return b.String()
			}
			b.WriteString(q[i : i+j])
			i += j - 1
		case c == '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
