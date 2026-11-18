package emailpreview

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// seedRow is one notification_templates row of a migration seed.
type seedRow struct {
	code, role, channel, language, subject, body string
}

// migrationSeeds parses every `INSERT INTO notification_templates (...)
// VALUES` statement of the embedded up migrations, in file order. Rows
// without a role column are generic.
func migrationSeeds(fsys fs.FS) ([]seedRow, error) {
	names, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var out []seedRow
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		rows, err := parseSeeds(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, rows...)
	}
	return out, nil
}

func parseSeeds(sql string) ([]seedRow, error) {
	const head = "INSERT INTO notification_templates ("
	var out []seedRow
	for {
		i := strings.Index(sql, head)
		if i < 0 {
			return out, nil
		}
		sql = sql[i+len(head):]
		end := strings.Index(sql, ")")
		if end < 0 {
			return nil, fmt.Errorf("column list not closed")
		}
		cols := strings.Split(sql[:end], ",")
		for j := range cols {
			cols[j] = strings.TrimSpace(cols[j])
		}
		v := strings.Index(sql, "VALUES")
		if v < 0 {
			return nil, fmt.Errorf("VALUES missing")
		}
		tuples, rest := parseTuples(sql[v+len("VALUES"):])
		sql = rest
		for _, tu := range tuples {
			if len(tu) != len(cols) {
				return nil, fmt.Errorf("tuple %v does not match columns %v", tu, cols)
			}
			r := seedRow{role: "generic"}
			for j, c := range cols {
				switch c {
				case "code":
					r.code = tu[j]
				case "role":
					r.role = tu[j]
				case "channel":
					r.channel = tu[j]
				case "language":
					r.language = tu[j]
				case "subject":
					r.subject = tu[j]
				case "body":
					r.body = tu[j]
				}
			}
			out = append(out, r)
		}
	}
}

// parseTuples reads `(v, v, ...), (...)` up to the statement end (`;` at
// depth 0). String literals are unquoted; bare tokens are kept as is.
func parseTuples(s string) ([][]string, string) {
	var (
		out   [][]string
		cur   []string
		tok   strings.Builder
		depth int
	)
	flush := func() {
		if v := strings.TrimSpace(tok.String()); v != "" {
			cur = append(cur, v)
		}
		tok.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			var lit strings.Builder
			for i++; i < len(s); i++ {
				if s[i] == '\'' {
					if i+1 < len(s) && s[i+1] == '\'' {
						lit.WriteByte('\'')
						i++
						continue
					}
					break
				}
				lit.WriteByte(s[i])
			}
			cur = append(cur, lit.String())
		case c == '(':
			depth++
			cur = nil
		case c == ')' && depth > 0:
			flush()
			depth--
			out = append(out, cur)
		case c == ',' && depth > 0:
			flush()
		case c == ';' && depth == 0:
			return out, s[i+1:]
		case depth > 0:
			tok.WriteByte(c)
		}
	}
	return out, ""
}
