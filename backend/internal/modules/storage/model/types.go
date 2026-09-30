package model

import "io"

type Object struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Key                string            `json:"key"`
	Prefix             string            `json:"prefix"`
	Bucket             string            `json:"bucket"`
	Kind               string            `json:"kind"`
	FileKind           string            `json:"file_kind"`
	Size               int64             `json:"size"`
	MimeType           string            `json:"mime_type"`
	ETag               string            `json:"etag,omitempty"`
	VersionID          string            `json:"version_id,omitempty"`
	Version            string            `json:"version,omitempty"`
	IsPublic           bool              `json:"is_public"`
	IsStarred          bool              `json:"is_starred"`
	IsShared           bool              `json:"is_shared"`
	Access             string            `json:"access"`
	Owner              string            `json:"owner,omitempty"`
	StorageClass       string            `json:"storage_class,omitempty"`
	ContentDisposition string            `json:"content_disposition,omitempty"`
	CacheControl       string            `json:"cache_control,omitempty"`
	Metadata           map[string]string `json:"metadata"`
	CreatedAt          string            `json:"created_at"`
	UpdatedAt          string            `json:"updated_at"`
	TrashUUID          string            `json:"trash_uuid,omitempty"`
	TrashExpiresAt     string            `json:"trash_expires_at,omitempty"`
}

type ListInput struct {
	Prefix       string
	View         string
	Q            string
	Kind         string
	Access       string
	Sort         string
	Limit        int32
	Offset       int32
	ModifiedFrom string
	ModifiedTo   string
	Recursive    bool
}

type ListResult struct {
	Items  []Object `json:"items"`
	Total  int64    `json:"total"`
	Limit  int32    `json:"limit"`
	Offset int32    `json:"offset"`
	Prefix string   `json:"prefix"`
	View   string   `json:"view"`
}

type CreateFolderInput struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
}

type CopyMoveInput struct {
	SourceKey string `json:"source_key"`
	DestKey   string `json:"dest_key"`
}

type RenameInput struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type KeysInput struct {
	Keys []string `json:"keys"`
}

type UploadMeta struct {
	Prefix      string
	Key         string
	Filename    string
	ContentType string
	Size        int64
}

type UploadSession struct {
	SessionID    string `json:"session_id"`
	Key          string `json:"key"`
	Method       string `json:"method"`
	UploadURL    string `json:"upload_url"`
	PresignedURL string `json:"presigned_url,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
}

type Version struct {
	VersionID string `json:"version_id"`
	Label     string `json:"label"`
	Size      int64  `json:"size"`
	ETag      string `json:"etag,omitempty"`
	IsLatest  bool   `json:"is_latest"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	CreatedBy string `json:"created_by,omitempty"`
}

type RestoreVersionInput struct {
	Key       string `json:"key"`
	VersionID string `json:"version_id"`
}

type Activity struct {
	UUID      string         `json:"uuid"`
	ObjectKey string         `json:"object_key"`
	Action    string         `json:"action"`
	Actor     string         `json:"actor"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"created_at"`
}

type Link struct {
	UUID        string `json:"uuid"`
	ObjectKey   string `json:"object_key"`
	Kind        string `json:"kind"`
	Slug        string `json:"slug,omitempty"`
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	CanView     bool   `json:"can_view"`
	CanDownload bool   `json:"can_download"`
	CanUpload   bool   `json:"can_upload"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type CreateLinkInput struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Slug        string `json:"slug"`
	ExpiresIn   int    `json:"expires_in"`
	CanView     *bool  `json:"can_view"`
	CanDownload *bool  `json:"can_download"`
	CanUpload   bool   `json:"can_upload"`
}

type Share struct {
	UUID      string `json:"uuid"`
	ObjectKey string `json:"object_key"`
	UserUUID  string `json:"user_uuid"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

type CreateShareInput struct {
	Key      string `json:"key"`
	UserUUID string `json:"user_uuid"`
	Role     string `json:"role"`
}

type UsageKind struct {
	Kind  string `json:"kind"`
	Bytes int64  `json:"bytes"`
	Count int64  `json:"count"`
}

type Usage struct {
	TotalBytes     int64       `json:"total_bytes"`
	UsedBytes      int64       `json:"used_bytes"`
	AvailableBytes int64       `json:"available_bytes"`
	QuotaBytes     int64       `json:"quota_bytes"`
	TotalFiles     int64       `json:"total_files"`
	TotalFolders   int64       `json:"total_folders"`
	ByKind         []UsageKind `json:"by_kind"`
	Largest        []Object    `json:"largest"`
	Recent         []Object    `json:"recent"`
}

type Actor struct {
	UserID int64
	Name   string
}

type OpenObject struct {
	Object   Object
	Body     io.ReadCloser
	Size     int64
	Download bool
}
