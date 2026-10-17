package usecase

import (
	"testing"

	"github.com/google/uuid"
)

func TestBulkAssigneeParamUUIDPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		params  map[string]string
		wantErr bool
	}{
		{"uuid overrides invalid numeric", map[string]string{ParamAssigneeUserUUID: uuid.NewString(), ParamAssigneeUserID: "invalid"}, false},
		{"legacy numeric", map[string]string{ParamAssigneeUserID: "42"}, false},
		{"invalid uuid overrides numeric", map[string]string{ParamAssigneeUserUUID: "invalid", ParamAssigneeUserID: "42"}, true},
		{"empty uuid overrides numeric", map[string]string{ParamAssigneeUserUUID: "", ParamAssigneeUserID: "42"}, true},
		{"missing", map[string]string{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := assigneeParam(tc.params)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
