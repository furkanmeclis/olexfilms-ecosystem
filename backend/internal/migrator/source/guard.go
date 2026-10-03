package source

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotReadOnly is returned for any statement the guard refuses. Legacy
// databases are read-only sources (AGENTS §4): only a single SELECT (or a
// WITH ... SELECT) may reach them.
var ErrNotReadOnly = errors.New("source: only read-only SELECT statements are allowed")

// forbidden keywords, matched as whole unquoted words anywhere in the
// statement. They cover DML, DDL, privilege and session changes, locking
// reads and MySQL/MariaDB side doors (SELECT ... INTO OUTFILE, LOAD DATA,
// HANDLER, CALL). The list is deliberately broad: a step that needs one of
// these words as a function (e.g. REPLACE()) does it in Go instead.
var forbidden = map[string]struct{}{
	"INSERT": {}, "UPDATE": {}, "DELETE": {}, "MERGE": {}, "REPLACE": {}, "UPSERT": {},
	"CREATE": {}, "ALTER": {}, "DROP": {}, "TRUNCATE": {}, "RENAME": {}, "COMMENT": {},
	"GRANT": {}, "REVOKE": {}, "SET": {}, "RESET": {}, "LOCK": {}, "UNLOCK": {},
	"CALL": {}, "DO": {}, "EXECUTE": {}, "PREPARE": {}, "DEALLOCATE": {}, "HANDLER": {},
	"LOAD": {}, "COPY": {}, "INTO": {}, "OUTFILE": {}, "DUMPFILE": {},
	"VACUUM": {}, "ANALYZE": {}, "OPTIMIZE": {}, "REPAIR": {}, "FLUSH": {}, "KILL": {},
	"BEGIN": {}, "COMMIT": {}, "ROLLBACK": {}, "SAVEPOINT": {}, "START": {},
	"SHUTDOWN": {}, "INSTALL": {}, "UNINSTALL": {}, "REFRESH": {}, "NOTIFY": {}, "LISTEN": {},
}

// CheckReadOnly accepts a single SELECT / WITH statement and rejects
// everything else: DML, DDL, multiple statements, locking reads (FOR UPDATE
// contains UPDATE; LOCK IN SHARE MODE contains LOCK) and SELECT ... INTO.
// Quoted strings, quoted identifiers and comments are skipped, so a literal
// such as 'DELETE' does not trip it and a keyword hidden in a comment does
// not slip past it as a separate statement.
func CheckReadOnly(query string) error {
	words, err := scanWords(query)
	if err != nil {
		return err
	}
	if len(words) == 0 {
		return fmt.Errorf("%w: empty statement", ErrNotReadOnly)
	}
	if first := words[0]; first != "SELECT" && first != "WITH" {
		return fmt.Errorf("%w: statement starts with %s", ErrNotReadOnly, first)
	}
	for _, w := range words {
		if _, bad := forbidden[w]; bad {
			return fmt.Errorf("%w: keyword %s", ErrNotReadOnly, w)
		}
	}
	return nil
}

// scanWords returns the upper-cased unquoted words of query. A ';' followed
// by anything but whitespace or comments is a second statement and refused.
func scanWords(q string) ([]string, error) {
	var words []string
	terminated := false
	i, n := 0, len(q)
	for i < n {
		c := q[i]
		switch {
		case c == '-' && i+1 < n && q[i+1] == '-', c == '#':
			for i < n && q[i] != '\n' {
				i++
			}
			continue
		case c == '/' && i+1 < n && q[i+1] == '*':
			end := strings.Index(q[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("%w: unterminated comment", ErrNotReadOnly)
			}
			// MySQL executes /*! ... */ comments as code; refuse them.
			if i+2 < n && (q[i+2] == '!' || q[i+2] == '+') {
				return nil, fmt.Errorf("%w: executable comment", ErrNotReadOnly)
			}
			i += end + 4
			continue
		case c == '\'' || c == '"' || c == '`':
			j := i + 1
			for ; j < n; j++ {
				// MariaDB reads \' as an escaped quote, Postgres does not: the
				// two engines would split the statement differently. Values go
				// in as args, so a backslash in a literal is refused.
				if q[j] == '\\' {
					return nil, fmt.Errorf("%w: backslash in quoted text", ErrNotReadOnly)
				}
				if q[j] == c {
					if j+1 < n && q[j+1] == c { // doubled quote escape
						j++
						continue
					}
					break
				}
			}
			if j >= n {
				return nil, fmt.Errorf("%w: unterminated quote", ErrNotReadOnly)
			}
			if terminated {
				return nil, fmt.Errorf("%w: multiple statements", ErrNotReadOnly)
			}
			i = j + 1
			continue
		case c == ';':
			terminated = true
			i++
			continue
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
			continue
		}
		if terminated {
			return nil, fmt.Errorf("%w: multiple statements", ErrNotReadOnly)
		}
		if isWordByte(c) {
			j := i
			for j < n && isWordByte(q[j]) {
				j++
			}
			words = append(words, strings.ToUpper(q[i:j]))
			i = j
			continue
		}
		i++
	}
	return words, nil
}

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c >= 0x80
}
