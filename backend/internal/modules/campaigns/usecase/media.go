package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Media limits: images (JPEG, PNG, WebP) up to 5 MB, PDF documents up to
// 16 MB (WhatsApp document limit). The type is sniffed from the bytes; the
// file name and the declared content type are not trusted.
const (
	MaxImageBytes    = 5 << 20
	MaxDocumentBytes = 16 << 20
	// MaxUploadBytes bounds the multipart body read by the handler.
	MaxUploadBytes = MaxDocumentBytes + (1 << 20)

	MediaImage    = "image"
	MediaDocument = "document"

	maxMediaPerContent = 5
	maxFileName        = 255
)

// MediaInput is an uploaded file.
type MediaInput struct {
	Body     io.Reader
	Filename string
}

// AddMedia stores an image or PDF on the content of a locale of a draft;
// the content is created empty when the locale has none yet.
func (s *Service) AddMedia(ctx context.Context, c Caller, id uuid.UUID, locale string, in MediaInput) (Media, error) {
	if !i18n.IsSupported(locale) {
		return Media{}, invalid("locale", "must be one of "+i18n.SupportedList())
	}
	data, kind, mimeType, ext, err := readMedia(in.Body)
	if err != nil {
		return Media{}, err
	}
	name := cleanFileName(in.Filename)
	if s.storage == nil {
		return Media{}, errors.New("campaigns: storage is not configured")
	}
	// Scope and draft check before the upload; repeated under lock below.
	row, err := s.load(ctx, s.q, c, id)
	if err != nil {
		return Media{}, err
	}
	if row.Status != StatusDraft {
		return Media{}, ErrNotDraft
	}
	org, err := s.q.GetOrganizationByID(ctx, row.OrganizationID)
	if err != nil {
		return Media{}, err
	}
	key := platstorage.CampaignMediaObjectKey(org.Uuid, row.Uuid, locale, uuid.New(), ext)
	if err := s.storage.Upload(ctx, platstorage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: mimeType, Filename: name,
	}, key); err != nil {
		return Media{}, fmt.Errorf("campaigns: upload media: %w", err)
	}
	var out Media
	err = s.inTx(ctx, func(q *db.Queries) error {
		row, err := s.lockDraft(ctx, q, c, id)
		if err != nil {
			return err
		}
		ct, err := q.EnsureCampaignContent(ctx, db.EnsureCampaignContentParams{CampaignID: row.ID, Locale: locale})
		if err != nil {
			return err
		}
		n, err := q.CountCampaignContentMedia(ctx, ct.ID)
		if err != nil {
			return err
		}
		if n >= maxMediaPerContent {
			return invalid("file", fmt.Sprintf("at most %d files per locale", maxMediaPerContent))
		}
		var fileName pgtype.Text
		if name != "" {
			fileName = pgtype.Text{String: name, Valid: true}
		}
		m, err := q.InsertCampaignMedia(ctx, db.InsertCampaignMediaParams{
			ContentID: ct.ID, Kind: kind, StorageKey: key, MimeType: mimeType, SizeBytes: int64(len(data)),
			FileName: fileName, SortOrder: int32(n), CreatedByUserID: pgInt8(c.UserID),
		})
		if err != nil {
			return err
		}
		out = mediaView(m.Uuid, locale, m.Kind, m.MimeType, m.SizeBytes, m.FileName, m.CreatedAt)
		return nil
	})
	if err != nil {
		s.removeObjects(ctx, key)
		return Media{}, err
	}
	return out, nil
}

// DeleteMedia removes a medium of the locale's content of a draft.
func (s *Service) DeleteMedia(ctx context.Context, c Caller, id uuid.UUID, locale string, mediaID uuid.UUID) error {
	var key string
	err := s.inTx(ctx, func(q *db.Queries) error {
		row, err := s.lockDraft(ctx, q, c, id)
		if err != nil {
			return err
		}
		m, err := q.GetCampaignMediaByUUID(ctx, db.GetCampaignMediaByUUIDParams{Uuid: mediaID, CampaignID: row.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		ct, err := q.GetCampaignContent(ctx, db.GetCampaignContentParams{CampaignID: row.ID, Locale: locale})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && ct.ID != m.ContentID) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := q.DeleteCampaignMedia(ctx, m.ID); err != nil {
			return err
		}
		key = m.StorageKey
		return nil
	})
	if err != nil {
		return err
	}
	s.removeObjects(ctx, key)
	return nil
}

// readMedia reads and sniffs an upload: kind, MIME type and extension.
func readMedia(body io.Reader) ([]byte, string, string, string, error) {
	if body == nil {
		return nil, "", "", "", invalid("file", "is required")
	}
	data, err := io.ReadAll(io.LimitReader(body, MaxDocumentBytes+1))
	if err != nil {
		return nil, "", "", "", err
	}
	if len(data) == 0 {
		return nil, "", "", "", invalid("file", "is empty")
	}
	mimeType := http.DetectContentType(data)
	var kind, ext string
	limit := MaxImageBytes
	switch mimeType {
	case "image/jpeg":
		kind, ext = MediaImage, "jpg"
	case "image/png":
		kind, ext = MediaImage, "png"
	case "image/webp":
		kind, ext = MediaImage, "webp"
	case "application/pdf":
		kind, ext, limit = MediaDocument, "pdf", MaxDocumentBytes
	default:
		return nil, "", "", "", invalid("file", "must be a JPEG, PNG or WebP image or a PDF document")
	}
	if len(data) > limit {
		if kind == MediaImage {
			return nil, "", "", "", invalid("file", "images must be at most 5 MB")
		}
		return nil, "", "", "", invalid("file", "documents must be at most 16 MB")
	}
	return data, kind, mimeType, ext, nil
}

func cleanFileName(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, `\`, "/")))
	if name == "." || name == "/" {
		return ""
	}
	if utf8.RuneCountInString(name) > maxFileName {
		r := []rune(name)
		name = string(r[:maxFileName])
	}
	return name
}
