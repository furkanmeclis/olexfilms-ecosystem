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
