package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// IntakePhotosPlaceholder is the vehicle intake photo grid of the contract
// PDF (TEC-499, F5-07b).
const IntakePhotosPlaceholder = "{{intake_photos_html}}"

// EnsureContractIntakePhotos adds {{intake_photos_html}} to the platform
// default contract templates (brand NULL) without a migration. Only the
// system seed is touched: a language whose active version was published by a
// person (created_by set), that already uses the placeholder or that has an
// open draft is left alone, so admin edits are never overwritten. The
// placeholder goes in a new published version (before the footer), so cached
// PDFs of the old version stop matching. Idempotent; returns the number of
// languages updated.
func (s *Service) EnsureContractIntakePhotos(ctx context.Context) (int, error) {
	updated := 0
	none := pgtype.Int8{}
	for _, lang := range model.Languages {
		active, err := s.q.GetActiveDocumentTemplate(ctx, db.GetActiveDocumentTemplateParams{
			Kind: model.KindContract, Language: lang, BrandID: none,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return updated, fmt.Errorf("documents: contract template %s: %w", lang, err)
		}
		if active.CreatedBy.Valid || strings.Contains(active.Html, "intake_photos_html") {
			continue
		}
		if _, err := s.q.GetDraftDocumentTemplate(ctx, db.GetDraftDocumentTemplateParams{
			Kind: model.KindContract, Language: lang, BrandID: none,
		}); err == nil {
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return updated, fmt.Errorf("documents: contract draft %s: %w", lang, err)
		}
		spec, _ := model.Spec(model.KindContract)
		html, vars, hash, err := prepareHTML(spec, withIntakePhotos(active.Html))
		if err != nil {
			return updated, fmt.Errorf("documents: contract template %s: %w", lang, err)
		}
		if err := s.publishSystemVersion(ctx, active, html, vars, hash); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

// withIntakePhotos inserts the placeholder before the footer paragraph, or
// appends it.
func withIntakePhotos(tpl string) string {
	if i := strings.LastIndex(tpl, `<p class="doc-footer">`); i >= 0 {
		return tpl[:i] + IntakePhotosPlaceholder + "\n" + tpl[i:]
	}
	return tpl + "\n" + IntakePhotosPlaceholder
}

func (s *Service) publishSystemVersion(ctx context.Context, active db.DocumentTemplate, html string, vars []byte, hash string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)
	next, err := qtx.NextDocumentTemplateVersion(ctx, db.NextDocumentTemplateVersionParams{
		Kind: active.Kind, Language: active.Language, BrandID: active.BrandID,
	})
	if err != nil {
		return err
	}
	row, err := qtx.CreateDocumentTemplate(ctx, db.CreateDocumentTemplateParams{
		Kind: active.Kind, BrandID: active.BrandID, Language: active.Language, Name: active.Name,
		Version: next, LexicalJson: nil, Html: html, Variables: vars, ContentHash: hash,
	})
	if err != nil {
		return fmt.Errorf("documents: contract template version: %w", err)
	}
	if err := qtx.DeactivateDocumentTemplates(ctx, db.DeactivateDocumentTemplatesParams{
		Kind: active.Kind, Language: active.Language, BrandID: active.BrandID,
	}); err != nil {
		return err
	}
	if _, err := qtx.PublishDocumentTemplate(ctx, row.ID); err != nil {
		return fmt.Errorf("documents: publish contract template: %w", err)
	}
	return tx.Commit(ctx)
}
