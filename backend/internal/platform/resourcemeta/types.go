package resourcemeta

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine/adapters"
)

// ColumnType is the admin DataTable / meta column type contract.
type ColumnType string

const (
	ColumnTypeString   ColumnType = "string"
	ColumnTypeEnum     ColumnType = "enum"
	ColumnTypeBoolean  ColumnType = "boolean"
	ColumnTypeDatetime ColumnType = "datetime"
	ColumnTypeUUID     ColumnType = "uuid"
)

// FilterVariant describes how admin should render a filter control.
type FilterVariant string

const (
	FilterVariantText    FilterVariant = "text"
	FilterVariantFaceted FilterVariant = "faceted"
)

// Capabilities declares which list/resource operations the API actually supports.
type Capabilities struct {
	Create bool `json:"create"`
	Read   bool `json:"read"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
	Search bool `json:"search"`
	Filter bool `json:"filter"`
	Sort   bool `json:"sort"`
	Export bool `json:"export"`
	Import bool `json:"import"`
	Bulk   bool `json:"bulk"`
}

// Column describes one list-table column the API supports.
type Column struct {
	Key            string        `json:"key"`
	LabelKey       string        `json:"label_key"`
	Type           ColumnType    `json:"type"`
	Sortable       bool          `json:"sortable"`
	Filterable     bool          `json:"filterable"`
	FilterVariant  FilterVariant `json:"filter_variant,omitempty"`
	DefaultVisible bool          `json:"default_visible"`
}

// Filter describes one list filter the API supports.
type Filter struct {
	Key        string        `json:"key"`
	LabelKey   string        `json:"label_key"`
	Variant    FilterVariant `json:"variant"`
	EnumLookup string        `json:"enum_lookup,omitempty"`
}

// ResourceMeta is the table-metadata contract for a primary list resource.
type ResourceMeta struct {
	Resource         string                     `json:"resource"`
	DefaultSort      string                     `json:"default_sort"`
	DefaultFields    []string                   `json:"default_fields"`
	Capabilities     Capabilities               `json:"capabilities"`
	SearchableFields []string                   `json:"searchable_fields"`
	SortableFields   []string                   `json:"sortable_fields"`
	FilterableFields []string                   `json:"filterable_fields"`
	Columns          []Column                   `json:"columns"`
	Filters          []Filter                   `json:"filters"`
	Includes         []string                   `json:"includes"`
	BulkActions      []bulkengine.BulkActionDef `json:"bulk_actions"`
}

// PlatformUsers returns meta for GET /v1/platform/users.
func PlatformUsers() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.users",
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "email", "name", "surname", "status"},
		Capabilities: Capabilities{
			Create: true, Read: true, Update: true, Delete: false,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"email", "name", "surname"},
		SortableFields:   []string{"email", "name", "surname", "status", "created_at", "updated_at"},
		FilterableFields: []string{"status", "role"},
		Columns: []Column{
			{Key: "uuid", LabelKey: "users.uuid", Type: ColumnTypeUUID, DefaultVisible: true},
			{Key: "email", LabelKey: "users.email", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "name", LabelKey: "users.name", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "surname", LabelKey: "users.surname", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "status", LabelKey: "users.status", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "users.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters: []Filter{
			{Key: "status", LabelKey: "users.status", Variant: FilterVariantFaceted, EnumLookup: "user_status"},
			{Key: "role", LabelKey: "users.role", Variant: FilterVariantFaceted, EnumLookup: "role_slug"},
		},
		Includes:    []string{"roles"},
		BulkActions: adapters.NewUsers(nil).BulkActions(),
	}
}

// PlatformRoles returns meta for GET /v1/platform/roles.
func PlatformRoles() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.roles",
		DefaultSort:   "name",
		DefaultFields: []string{"uuid", "name", "slug", "is_system"},
		Capabilities: Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: false, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"name", "slug"},
		SortableFields:   []string{"name", "slug", "created_at", "updated_at"},
		FilterableFields: []string{},
		Columns: []Column{
			{Key: "uuid", LabelKey: "roles.uuid", Type: ColumnTypeUUID, DefaultVisible: true},
			{Key: "name", LabelKey: "roles.name", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "slug", LabelKey: "roles.slug", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "is_system", LabelKey: "roles.is_system", Type: ColumnTypeBoolean, DefaultVisible: true},
			{Key: "created_at", LabelKey: "roles.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters:     []Filter{},
		Includes:    []string{"permission_slugs"},
		BulkActions: adapters.NewRoles(nil).BulkActions(),
	}
}

// Notifications returns meta for GET /v1/notifications (inbox).
func Notifications() ResourceMeta {
	return ResourceMeta{
		Resource:      "notifications",
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "channel", "status", "priority", "title", "created_at", "read_at"},
		Capabilities: Capabilities{
			Create: false, Read: true, Update: false, Delete: false,
			Search: true, Filter: true, Sort: true, Export: false, Import: false, Bulk: false,
		},
		SearchableFields: []string{"title", "body", "template_code", "recipient"},
		SortableFields:   []string{"channel", "status", "priority", "created_at", "updated_at", "sent_at"},
		FilterableFields: []string{"status", "channel", "unread"},
		Columns: []Column{
			{Key: "uuid", LabelKey: "notifications.uuid", Type: ColumnTypeUUID, DefaultVisible: true},
			{Key: "channel", LabelKey: "notifications.channel", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "status", LabelKey: "notifications.status", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "priority", LabelKey: "notifications.priority", Type: ColumnTypeEnum, Sortable: true, DefaultVisible: true},
			{Key: "title", LabelKey: "notifications.title", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "created_at", LabelKey: "notifications.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "read_at", LabelKey: "notifications.read_at", Type: ColumnTypeDatetime, DefaultVisible: true},
		},
		Filters: []Filter{
			{Key: "status", LabelKey: "notifications.status", Variant: FilterVariantFaceted},
			{Key: "channel", LabelKey: "notifications.channel", Variant: FilterVariantFaceted},
			{Key: "unread", LabelKey: "notifications.unread", Variant: FilterVariantFaceted},
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// PlatformNotifications returns meta for GET /v1/platform/notifications.
func PlatformNotifications() ResourceMeta {
	m := Notifications()
	m.Resource = "platform.notifications"
	m.Capabilities.Export = true
	return m
}

// Activity returns meta for GET /v1/platform/activity.
func Activity() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.activity",
		DefaultSort:   "-created_at",
		DefaultFields: []string{"action", "resource", "actor_user_id", "created_at"},
		Capabilities: Capabilities{
			Read: true, Search: true, Filter: true, Sort: true, Export: true,
		},
		SearchableFields: []string{"action", "resource"},
		SortableFields:   []string{"created_at", "action", "resource"},
		Columns: []Column{
			{Key: "action", LabelKey: "activity.action", Sortable: true, DefaultVisible: true},
			{Key: "resource", LabelKey: "activity.resource", Sortable: true, DefaultVisible: true},
			{Key: "actor_user_id", LabelKey: "activity.actor", DefaultVisible: true},
			{Key: "created_at", LabelKey: "activity.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
		},
	}
}

// PlatformStorage returns meta for GET /v1/platform/storage/objects.
func PlatformStorage() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.storage",
		DefaultSort:   "name",
		DefaultFields: []string{"name", "kind", "size", "updated_at", "access"},
		Capabilities: Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: false, Import: false, Bulk: false,
		},
		SearchableFields: []string{"name", "key", "mime_type"},
		SortableFields:   []string{"name", "size", "updated_at", "type"},
		FilterableFields: []string{"kind", "access"},
		Columns: []Column{
			{Key: "name", LabelKey: "storage.col_name", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "kind", LabelKey: "storage.col_type", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "size", LabelKey: "storage.col_size", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "updated_at", LabelKey: "storage.col_modified", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "access", LabelKey: "storage.col_access", Type: ColumnTypeEnum, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
		},
		Filters: []Filter{
			{Key: "kind", LabelKey: "storage.col_type", Variant: FilterVariantFaceted},
			{Key: "access", LabelKey: "storage.col_access", Variant: FilterVariantFaceted},
		},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// PlatformLogs returns meta for GET /v1/platform/logs.
func PlatformLogs() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.logs",
		DefaultSort:   "-created_at",
		DefaultFields: []string{"level", "message", "source", "created_at"},
		Capabilities: Capabilities{
			Read: true, Delete: true, Search: true, Filter: true, Sort: true, Bulk: false,
		},
		SearchableFields: []string{"message", "source", "request_id"},
		SortableFields:   []string{"created_at", "level", "source"},
		FilterableFields: []string{"level", "source", "created_from", "created_to"},
		Columns: []Column{
			{Key: "level", LabelKey: "logs.columns.level", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "message", LabelKey: "logs.columns.message", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "source", LabelKey: "logs.columns.source", Type: ColumnTypeString, Sortable: true, Filterable: true, DefaultVisible: true},
			{Key: "created_at", LabelKey: "logs.columns.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
		},
		Filters: []Filter{
			{Key: "level", LabelKey: "logs.columns.level", Variant: FilterVariantFaceted},
			{Key: "source", LabelKey: "logs.columns.source", Variant: FilterVariantText},
		},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// PlatformLogRules returns meta for GET /v1/platform/log-rules.
func PlatformLogRules() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.log_rules",
		DefaultSort:   "name",
		DefaultFields: []string{"name", "enabled", "levels", "older_than_hours", "interval_minutes"},
		Capabilities: Capabilities{
			Create: true, Read: true, Update: true, Delete: true, Sort: true,
		},
		SortableFields: []string{"name", "created_at", "updated_at"},
		Columns: []Column{
			{Key: "name", LabelKey: "logs.rules.columns.name", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "enabled", LabelKey: "logs.rules.columns.enabled", Type: ColumnTypeBoolean, DefaultVisible: true},
			{Key: "older_than_hours", LabelKey: "logs.rules.columns.older_than", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "interval_minutes", LabelKey: "logs.rules.columns.schedule", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "last_run_at", LabelKey: "logs.rules.columns.last_run", Type: ColumnTypeDatetime, DefaultVisible: true},
		},
	}
}

// PlatformOrganizations returns meta for GET /v1/platform/organizations.
func PlatformOrganizations() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.organizations",
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "slug", "name", "city", "phone", "status", "plan_code", "access_ends_at"},
		Capabilities: Capabilities{
			Create: true, Read: true, Update: true, Delete: false,
			Search: true, Filter: true, Sort: true, Export: false, Import: false, Bulk: false,
		},
		SearchableFields: []string{"name", "slug", "city", "phone"},
		SortableFields:   []string{"name", "slug", "city", "status", "created_at", "access_ends_at"},
		FilterableFields: []string{"status"},
		Columns: []Column{
			{Key: "uuid", LabelKey: "organizations.uuid", Type: ColumnTypeUUID, DefaultVisible: true},
			{Key: "slug", LabelKey: "organizations.slug", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "name", LabelKey: "organizations.name", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "city", LabelKey: "organizations.city", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "phone", LabelKey: "organizations.phone", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "status", LabelKey: "organizations.status", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "plan_code", LabelKey: "organizations.plan_code", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "access_ends_at", LabelKey: "organizations.access_ends_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "created_at", LabelKey: "organizations.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters: []Filter{
			{Key: "status", LabelKey: "organizations.status", Variant: FilterVariantFaceted},
		},
	}
}
