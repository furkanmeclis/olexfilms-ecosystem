package i18n

import "testing"

func TestResourceLabel(t *testing.T) {
	if got := ResourceLabel(LocaleTR, "platform.users"); got != "Kullanıcılar" {
		t.Fatalf("tr resource = %q", got)
	}
	if got := ResourceLabel(LocaleEN, "platform.users"); got != "Users" {
		t.Fatalf("en resource = %q", got)
	}
}

func TestExportFormatLabel(t *testing.T) {
	if got := ExportFormatLabel(LocaleTR, "xlsx"); got != "Excel" {
		t.Fatalf("tr format = %q", got)
	}
}
