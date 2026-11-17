package refs

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestKindsUniqueAndKnown(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range Kinds {
		if seen[k] {
			t.Fatalf("duplicate kind %q", k)
		}
		seen[k] = true
		if !IsKind(k) {
			t.Fatalf("IsKind(%q) = false", k)
		}
	}
	if len(Kinds) != 21 {
		t.Fatalf("kinds = %d, want 21 (docs/refs.md)", len(Kinds))
	}
	if IsKind("report") || IsKind("") {
		t.Fatal("report / empty must not be a record kind")
	}
}

func TestRefJSON(t *testing.T) {
	id := uuid.MustParse("6f1c2a52-6d0e-4a52-9a43-0f3f3a1b2c3d")
	b, _ := json.Marshal(UserRef(id, "Ayşe Yılmaz"))
	if got, want := string(b), `{"kind":"user","uuid":"6f1c2a52-6d0e-4a52-9a43-0f3f3a1b2c3d","name":"Ayşe Yılmaz"}`; got != want {
		t.Fatalf("user ref = %s, want %s", got, want)
	}
	b, _ = json.Marshal(ProductRef(id, "PPF Mat", "PPF-01"))
	if got, want := string(b), `{"kind":"product","uuid":"6f1c2a52-6d0e-4a52-9a43-0f3f3a1b2c3d","name":"PPF Mat","sub":"PPF-01"}`; got != want {
		t.Fatalf("product ref = %s, want %s", got, want)
	}
	if OrgRef(id, "Bayi", "").Sub != nil {
		t.Fatal("empty sub must be omitted")
	}
	if Ptr(KindUser, uuid.Nil, "x") != nil || UserRefPtr(id, "x") == nil {
		t.Fatal("Ptr nil handling")
	}
	if FullName("Ayşe", "") != "Ayşe" || FullName("", "Y") != "Y" || FullName("A", "B") != "A B" {
		t.Fatal("FullName")
	}
}
