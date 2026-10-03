package source

import (
	"context"
	"errors"
	"testing"
)

func TestCheckReadOnlyAccepts(t *testing.T) {
	ok := []string{
		"SELECT id, name FROM customers WHERE id > ? ORDER BY id LIMIT 500",
		"select * from dealers",
		"  SELECT 1;  ",
		"SELECT 1; -- trailing comment",
		"WITH x AS (SELECT id FROM users) SELECT id FROM x",
		"SELECT 'DELETE FROM users' AS note FROM dealers",
		"SELECT `update`, \"insert\" FROM t",
		"SELECT c.id FROM customers c /* drop table */ JOIN dealers d ON d.id = c.dealer_id",
		"SELECT updated_at, deleted_at, created_by FROM services",
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?",
		"SELECT 'it''s' FROM t",
	}
	for _, q := range ok {
		if err := CheckReadOnly(q); err != nil {
			t.Errorf("CheckReadOnly(%q) = %v, want nil", q, err)
		}
	}
}

func TestCheckReadOnlyRejects(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"-- only a comment",
		"INSERT INTO customers (id) VALUES (1)",
		"insert into customers values (1)",
		"UPDATE customers SET name = 'x'",
		"DELETE FROM customers",
		"REPLACE INTO customers VALUES (1)",
		"MERGE INTO t USING s ON true WHEN MATCHED THEN DELETE",
		"CREATE TABLE x (id int)",
		"ALTER TABLE customers ADD COLUMN x int",
		"DROP TABLE customers",
		"TRUNCATE customers",
		"RENAME TABLE a TO b",
		"GRANT ALL ON *.* TO someone",
		"SET SESSION TRANSACTION READ WRITE",
		"CALL some_proc()",
		"LOAD DATA INFILE 'x' INTO TABLE t",
		"LOCK TABLES customers WRITE",
		"COPY customers FROM '/tmp/x'",
		"BEGIN",
		"COMMIT",
		"SELECT 1; DELETE FROM customers",
		"SELECT 1;DROP TABLE x",
		"SELECT 1; 'x'",
		"SELECT * FROM customers FOR UPDATE",
		"SELECT * FROM customers LOCK IN SHARE MODE",
		"SELECT * INTO OUTFILE '/tmp/x' FROM customers",
		"SELECT * FROM customers INTO DUMPFILE '/tmp/x'",
		"SELECT * INTO new_table FROM customers",
		"WITH d AS (DELETE FROM customers RETURNING id) SELECT * FROM d",
		"WITH u AS (UPDATE customers SET name = '' RETURNING id) SELECT 1",
		"SELECT 1 /*! ; DROP TABLE x */",
		"SELECT 'unterminated",
		"SELECT 'a\\'; DELETE FROM customers; --' FROM t",
		"SELECT 1 /* unterminated",
		"EXPLAIN ANALYZE DELETE FROM customers",
		"SHOW TABLES",
		"VACUUM",
	}
	for _, q := range bad {
		err := CheckReadOnly(q)
		if !errors.Is(err, ErrNotReadOnly) {
			t.Errorf("CheckReadOnly(%q) = %v, want ErrNotReadOnly", q, err)
		}
	}
}

// The MariaDB adapter refuses a write before any connection is used (db is
// nil here: reaching it would panic).
func TestMariaDBQueryRefusesWrites(t *testing.T) {
	m := &MariaDB{name: "hub"}
	for _, q := range []string{
		"INSERT INTO customers (id) VALUES (1)",
		"UPDATE customers SET name = 'x'",
		"DELETE FROM customers",
		"DROP TABLE customers",
	} {
		if _, err := m.Query(context.Background(), q); !errors.Is(err, ErrNotReadOnly) {
			t.Errorf("MariaDB.Query(%q) = %v, want ErrNotReadOnly", q, err)
		}
	}
}

func TestRewritePlaceholders(t *testing.T) {
	cases := map[string]string{
		"SELECT * FROM t WHERE a = ? AND b > ?":   "SELECT * FROM t WHERE a = $1 AND b > $2",
		"SELECT '?' FROM t WHERE a = ?":           "SELECT '?' FROM t WHERE a = $1",
		"SELECT \"a?\" FROM t -- why?\nWHERE a=?": "SELECT \"a?\" FROM t -- why?\nWHERE a=$1",
		"SELECT 1": "SELECT 1",
	}
	for in, want := range cases {
		if got := rewritePlaceholders(in); got != want {
			t.Errorf("rewritePlaceholders(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidTableName(t *testing.T) {
	for _, n := range []string{"customers", "model_has_roles", "T1"} {
		if !validTableName(n) {
			t.Errorf("validTableName(%q) = false", n)
		}
	}
	for _, n := range []string{"", "a.b", "a;b", "a b", "a'b"} {
		if validTableName(n) {
			t.Errorf("validTableName(%q) = true", n)
		}
	}
}
