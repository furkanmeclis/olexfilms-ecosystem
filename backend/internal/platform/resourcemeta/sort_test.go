package resourcemeta

import (
	"testing"

	authmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// TEC-363: for resources whose list applies sort in SQL, meta must publish
// exactly the endpoint whitelist and a default that resolves against it, and
// every column marked sortable must be accepted by the endpoint.
func TestMetaSortMatchesListWhitelist(t *testing.T) {
	cases := []struct {
		meta ResourceMeta
		spec apiquery.SortSpec
	}{
		{PlatformUsers(), apiquery.UsersSortSpec},
		{PlatformOrganizations(), apiquery.TenantsSortSpec},
		{PlatformNotifications(), apiquery.NotificationsSortSpec},
		{PlatformLogs(), apiquery.LogsSortSpec},
		// TEC-365
		{PlatformRoles(), authmodel.RolesSortSpec},
		{Activity(), activity.SortSpec},
	}
	for _, c := range cases {
		if _, err := apiquery.ResolveSort(apiquery.ParseSort(c.meta.DefaultSort), c.spec); err != nil {
			t.Fatalf("%s default_sort %q: %v", c.meta.Resource, c.meta.DefaultSort, err)
		}
		if len(c.meta.SortableFields) != len(c.spec.Columns) {
			t.Fatalf("%s sortable_fields %v != whitelist", c.meta.Resource, c.meta.SortableFields)
		}
		for _, col := range c.meta.Columns {
			if _, ok := c.spec.Columns[col.Key]; col.Sortable && !ok {
				t.Fatalf("%s column %q is marked sortable but not whitelisted", c.meta.Resource, col.Key)
			}
		}
	}
	for _, col := range PlatformLogs().Columns {
		if col.Key == "message" && col.Sortable {
			t.Fatal("logs message must not be sortable")
		}
	}
}
