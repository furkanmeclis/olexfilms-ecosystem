package apiquery

// Resource sort/search whitelists for list endpoints.

var (
	// TenantMembersSort columns (current tenant members).
	TenantMembersSort = SortColumns{
		"email":      "email",
		"name":       "name",
		"surname":    "surname",
		"role":       "role",
		"created_at": "created_at",
	}
	TenantMembersSearchable = []string{"email", "name", "surname"}

	// TenantsSort columns (platform catalog).
	TenantsSort = SortColumns{
		"name":       "name",
		"slug":       "slug",
		"status":     "status",
		"created_at": "created_at",
		"updated_at": "updated_at",
		// TEC-363: the platform organizations table sorts these too.
		"city":           "city",
		"access_ends_at": "access_ends_at",
	}
	TenantsSearchable = []string{"name", "slug"}

	// UsersSort columns (platform catalog).
	UsersSort = SortColumns{
		"email":      "email",
		"name":       "name",
		"surname":    "surname",
		"status":     "status",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}
	UsersSearchable = []string{"email", "name", "surname"}

	// NotificationsSort columns.
	NotificationsSort = SortColumns{
		"channel":    "channel",
		"status":     "status",
		"priority":   "priority",
		"created_at": "created_at",
		"updated_at": "updated_at",
		"sent_at":    "sent_at",
	}
	NotificationsSearchable = []string{"title", "body", "template_code", "recipient"}

	// LogsSort columns (application log viewer).
	LogsSort = SortColumns{
		"created_at": "created_at",
		"level":      "level",
		"source":     "source",
	}
	LogsSearchable = []string{"message", "source", "request_id"}

	// StorageSort columns (in-memory object listing).
	StorageSort = SortColumns{
		"name":       "name",
		"size":       "size",
		"updated_at": "updated_at",
		"type":       "type",
	}
	StorageSearchable = []string{"name", "key", "mime_type"}
)

// Sort specs (whitelist + default) for list endpoints that apply sort in
// SQL (TEC-363). The default is also published as resource meta
// default_sort. New endpoints should keep their spec next to their module
// instead of growing this file.
var (
	UsersSortSpec         = SortSpec{Columns: UsersSort, Default: SortField{Field: "created_at", Desc: true}}
	TenantsSortSpec       = SortSpec{Columns: TenantsSort, Default: SortField{Field: "created_at", Desc: true}}
	NotificationsSortSpec = SortSpec{Columns: NotificationsSort, Default: SortField{Field: "created_at", Desc: true}}
	LogsSortSpec          = SortSpec{Columns: LogsSort, Default: SortField{Field: "created_at", Desc: true}}
)
