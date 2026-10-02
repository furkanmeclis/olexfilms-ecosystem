package catalog

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
)

// seedRow is one notification_templates row of a migration seed.
type seedRow struct {
	code, role, channel, language, subject, body string
}

// TEC-148: every default template seeded by 000008 (tr + en only) exists in
// all 13 locales once the migrations ran (000036 adds the notifications.test
// ar rows, 000054 the rest), with the same placeholder set as tr/en. The
// migration SQL is parsed, so this runs without a database.
func TestMigrationSeedTemplatesCoverEveryLocale(t *testing.T) {
	if len(msgtemplate.Locales) != 13 {
		t.Fatalf("locales = %d, want 13", len(msgtemplate.Locales))
	}
	base := seedRows(t, "000008_create_notifications.up.sql")
	added := seedRows(t, "000054_notification_template_translations.up.sql")
	all := append(append(append([]seedRow{}, base...),
		seedRows(t, "000036_notification_center.up.sql")...), added...)

	// 000008 source keys and their placeholder set (tr and en must agree).
	want := map[string][]string{}
	for _, r := range base {
		if r.language != "tr" && r.language != "en" {
			t.Fatalf("000008 seeds %s %s/%s", r.code, r.channel, r.language)
		}
		k := r.code + "|" + r.channel
		ph := placeholders(r)
		if prev, ok := want[k]; ok && !slices.Equal(prev, ph) {
			t.Fatalf("000008 %s: tr/en placeholders differ %v vs %v", k, prev, ph)
		}
		want[k] = ph
	}
	if len(want) != 7 {
		t.Fatalf("000008 keys = %d, want 7", len(want))
	}

	seen := map[string]bool{}
	for _, r := range all {
		k := r.code + "|" + r.channel
		ph, ok := want[k]
		if !ok || r.role != RoleGeneric {
			continue
		}
		seen[k+"|"+r.language] = true
		if strings.TrimSpace(r.subject) == "" || strings.TrimSpace(r.body) == "" {
			t.Errorf("%s/%s: empty text", k, r.language)
		}
		if got := placeholders(r); !slices.Equal(got, ph) {
			t.Errorf("%s/%s: placeholders %v, want %v", k, r.language, got, ph)
		}
	}
	for k := range want {
		for _, l := range msgtemplate.Locales {
			if !seen[k+"|"+l] {
				t.Errorf("%s: no template in %s", k, l)
			}
		}
	}

	// 000054 only completes 000008 keys in the 11 non tr/en locales
	// (7 keys x 11, minus the two 000036 ar rows).
	if len(added) != 7*11-2 {
		t.Errorf("000054 rows = %d, want %d", len(added), 7*11-2)
	}
	for _, r := range added {
		if _, ok := want[r.code+"|"+r.channel]; !ok || r.role != RoleGeneric ||
			r.language == "tr" || r.language == "en" || !slices.Contains(msgtemplate.Locales, r.language) {
			t.Errorf("000054 seeds unexpected row %s %s %s/%s", r.code, r.role, r.channel, r.language)
		}
	}
	up := readMigration(t, "000054_notification_template_translations.up.sql")
	if !strings.Contains(up, "ON CONFLICT DO NOTHING") {
		t.Error("000054 up must not overwrite existing (admin edited) rows")
	}
	down := readMigration(t, "000054_notification_template_translations.down.sql")
	for _, r := range added {
		if !strings.Contains(down, sqlQuote(r.body)) {
			t.Errorf("000054 down does not cover %s %s/%s", r.code, r.channel, r.language)
		}
	}
}

func placeholders(r seedRow) []string {
	return msgtemplate.Placeholders(r.subject + "\n" + r.body)
}

func sqlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func readMigration(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// seedRows parses every `INSERT INTO notification_templates (...) VALUES`
// statement of a migration. Rows without a role column are generic.
func seedRows(t *testing.T, name string) []seedRow {
	t.Helper()
	sql := readMigration(t, name)
	const head = "INSERT INTO notification_templates ("
	var out []seedRow
	for {
		i := strings.Index(sql, head)
		if i < 0 {
			return out
		}
		sql = sql[i+len(head):]
		end := strings.Index(sql, ")")
		cols := strings.Split(sql[:end], ",")
		for j := range cols {
			cols[j] = strings.TrimSpace(cols[j])
		}
		v := strings.Index(sql, "VALUES")
		if v < 0 {
			t.Fatalf("%s: VALUES missing", name)
		}
		tuples, rest := parseTuples(t, name, sql[v+len("VALUES"):])
		sql = rest
		for _, tu := range tuples {
			if len(tu) != len(cols) {
				t.Fatalf("%s: tuple %v does not match columns %v", name, tu, cols)
			}
			r := seedRow{role: RoleGeneric}
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

// parseTuples reads `(v, v, ...), (...)` up to the statement end. String
// literals are unquoted (a doubled quote becomes one); bare tokens (TRUE, NULL)
// are kept as is.
func parseTuples(t *testing.T, name, s string) ([][]string, string) {
	t.Helper()
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
		case depth > 0:
			tok.WriteByte(c)
		case c == ';' || (depth == 0 && strings.HasPrefix(s[i:], "ON CONFLICT")):
			return out, s[i:]
		}
	}
	t.Fatalf("%s: unterminated VALUES", name)
	return nil, ""
}
