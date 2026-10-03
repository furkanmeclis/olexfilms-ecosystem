package db_test

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-285: database-level guards of the contract schema (migration 000083).
// Reuses the service fixture (rolled-back transaction, savepoint per
// failure).

var contractSHA = strings.Repeat("ab", 32)

func (f *serviceFixture) contractTemplate(t *testing.T, kind string, isDefault bool) db.ContractTemplate {
	t.Helper()
	tpl, err := f.q.CreateContractTemplate(f.ctx, db.CreateContractTemplateParams{
		OrganizationID: f.centerID, BrandID: f.brandID, Name: "T285 " + kind,
		Kind: kind, IsDefault: isDefault, OtpRequired: true, SignatureRequired: true, IsActive: true,
	})
	if err != nil {
		t.Fatalf("template %s: %v", kind, err)
	}
	return tpl
}

func (f *serviceFixture) contractInstanceParams(t *testing.T, org db.Organization, svc db.Service, tpl db.ContractTemplate) db.CreateContractInstanceParams {
	t.Helper()
	no, err := f.q.NextContractNo(f.ctx, org.ID)
	if err != nil {
		t.Fatalf("next contract no: %v", err)
	}
	return db.CreateContractInstanceParams{
		OrganizationID: org.ID, BrandID: org.BrandID, ContractNo: no,
		SubjectType: "service", SubjectID: svc.ID,
		TemplateID: tpl.ID, Kind: tpl.Kind, Locale: "tr", TemplateVersion: 1,
		OtpRequired: tpl.OtpRequired, SignatureRequired: tpl.SignatureRequired, Status: "draft",
	}
}

func (f *serviceFixture) contractInstance(t *testing.T, org db.Organization, svc db.Service, tpl db.ContractTemplate) db.ContractInstance {
	t.Helper()
	c, err := f.q.CreateContractInstance(f.ctx, f.contractInstanceParams(t, org, svc, tpl))
	if err != nil {
		t.Fatalf("contract instance: %v", err)
	}
	return c
}

func (f *serviceFixture) contractSigner(t *testing.T, c db.ContractInstance, role string) db.ContractSigner {
	t.Helper()
	s, err := f.q.UpsertContractSigner(f.ctx, db.UpsertContractSignerParams{
		InstanceID: c.ID, OrganizationID: c.OrganizationID, BrandID: c.BrandID,
		Role: role, Name: "T285 " + role,
	})
	if err != nil {
		t.Fatalf("signer %s: %v", role, err)
	}
	return s
}

func (f *serviceFixture) mediaParams(c db.ContractInstance, size int64) db.InsertContractMediaParams {
	return db.InsertContractMediaParams{
		InstanceID: c.ID, OrganizationID: c.OrganizationID, BrandID: c.BrandID,
		StorageKey: "contracts/t285/photo.jpg", MimeType: "image/jpeg", SizeBytes: size, Sha256: contractSHA,
	}
}

// clearContractDefaults drops the brand's default flags inside the test
// transaction so the uniqueness check starts from a known state.
func (f *serviceFixture) clearContractDefaults(t *testing.T) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, `UPDATE contract_templates SET is_default = false WHERE brand_id = $1`, f.brandID); err != nil {
		t.Fatal(err)
	}
}

func TestContractTemplateConstraints(t *testing.T) {
	f := newServiceFixture(t)
	f.clearContractDefaults(t)
	first := f.contractTemplate(t, "vehicle_intake", true)

	// A second default of the same kind in the brand is rejected.
	f.expectConstraint(t, "second default", "23505", "uq_contract_templates_default", func(sp pgx.Tx) error {
		_, err := db.New(sp).CreateContractTemplate(f.ctx, db.CreateContractTemplateParams{
			OrganizationID: f.centerID, BrandID: f.brandID, Name: "T285 second",
			Kind: "vehicle_intake", IsDefault: true, IsActive: true,
		})
		return err
	})
	// Another kind may have its own default; a non-default is free.
	f.contractTemplate(t, "service_sale", true)
	f.contractTemplate(t, "vehicle_intake", false)

	// A default template is active; the kind is closed.
	f.expectConstraint(t, "inactive default", "23514", "chk_contract_templates_default_active", func(sp pgx.Tx) error {
		_, err := db.New(sp).UpdateContractTemplate(f.ctx, db.UpdateContractTemplateParams{
			ID: first.ID, BrandID: f.brandID, Name: first.Name, IsDefault: true, IsActive: false,
		})
		return err
	})
	f.expectConstraint(t, "unknown kind", "23514", "chk_contract_templates_kind", func(sp pgx.Tx) error {
		_, err := db.New(sp).CreateContractTemplate(f.ctx, db.CreateContractTemplateParams{
			OrganizationID: f.centerID, BrandID: f.brandID, Name: "T285 x", Kind: "lease", IsActive: true,
		})
		return err
	})
	// brand_id is the organization's brand.
	f.expectTrigger(t, "brand mismatch", func(sp pgx.Tx) error {
		var other int64
		if err := sp.QueryRow(f.ctx, `SELECT id FROM brands WHERE id <> $1 ORDER BY id LIMIT 1`, f.brandID).Scan(&other); err != nil {
			return err
		}
		_, err := db.New(sp).CreateContractTemplate(f.ctx, db.CreateContractTemplateParams{
			OrganizationID: f.centerID, BrandID: other, Name: "T285 y", Kind: "vehicle_intake", IsActive: true,
		})
		return err
	})

	// Locales: one row per (template, locale), 13 locales only; the upsert
	// bumps the version.
	loc := db.UpsertContractTemplateLocaleParams{
		TemplateID: first.ID, OrganizationID: first.OrganizationID, BrandID: first.BrandID,
		Locale: "tr", Html: "<p>Sözleşme</p>",
	}
	v1, err := f.q.UpsertContractTemplateLocale(f.ctx, loc)
	if err != nil || v1.Version != 1 {
		t.Fatalf("locale v1 = %+v, %v", v1, err)
	}
	v2, err := f.q.UpsertContractTemplateLocale(f.ctx, loc)
	if err != nil || v2.Version != 2 || v2.ID != v1.ID {
		t.Fatalf("locale v2 = %+v, %v", v2, err)
	}
	f.expectConstraint(t, "unknown locale", "23514", "chk_contract_template_locales_locale", func(sp pgx.Tx) error {
		arg := loc
		arg.Locale = "pt"
		_, err := db.New(sp).UpsertContractTemplateLocale(f.ctx, arg)
		return err
	})
}

func TestContractInstanceConstraints(t *testing.T) {
	f := newServiceFixture(t)
	tpl := f.contractTemplate(t, "vehicle_intake", false)
	svc := f.service(t, f.dealer)

	// Numbers are sequential per organization.
	c1 := f.contractInstance(t, f.dealer, svc, tpl)
	c2 := f.contractInstance(t, f.dealer, svc, tpl)
	if c2.ContractNo != c1.ContractNo+1 {
		t.Fatalf("contract numbers %d, %d", c1.ContractNo, c2.ContractNo)
	}
	other := f.contractInstance(t, f.dealer2, f.service(t, f.dealer2), tpl)
	if other.ContractNo != 1 {
		t.Fatalf("first number of another organization = %d", other.ContractNo)
	}
	f.expectConstraint(t, "duplicate number", "23505", "uq_contract_instances_org_no", func(sp pgx.Tx) error {
		arg := f.contractInstanceParams(t, f.dealer, svc, tpl)
		arg.ContractNo = c1.ContractNo
		_, err := db.New(sp).CreateContractInstance(f.ctx, arg)
		return err
	})

	// The subject service belongs to the contract's organization.
	f.expectConstraint(t, "subject of another org", "23503", "fk_contract_instances_subject_service", func(sp pgx.Tx) error {
		arg := f.contractInstanceParams(t, f.dealer2, svc, tpl)
		_, err := db.New(sp).CreateContractInstance(f.ctx, arg)
		return err
	})
	// The kind snapshot equals the template kind.
	f.expectTrigger(t, "kind mismatch", func(sp pgx.Tx) error {
		arg := f.contractInstanceParams(t, f.dealer, svc, tpl)
		arg.Kind = "service_sale"
		_, err := db.New(sp).CreateContractInstance(f.ctx, arg)
		return err
	})

	// services.contract_id: own organization's contract only.
	f.expectOK(t, "link own contract", func(sp pgx.Tx) error {
		_, err := db.New(sp).SetServiceContract(f.ctx, db.SetServiceContractParams{
			ServiceID: svc.ID, ContractID: pgtype.Int8{Int64: c1.ID, Valid: true},
		})
		return err
	})
	f.expectConstraint(t, "link other org contract", "23503", "fk_services_contract", func(sp pgx.Tx) error {
		_, err := db.New(sp).SetServiceContract(f.ctx, db.SetServiceContractParams{
			ServiceID: svc.ID, ContractID: pgtype.Int8{Int64: other.ID, Valid: true},
		})
		return err
	})

	// Signer slots: customer and staff only, once each.
	f.contractSigner(t, c1, "customer")
	f.contractSigner(t, c1, "staff")
	f.expectConstraint(t, "unknown signer role", "23514", "chk_contract_signers_role", func(sp pgx.Tx) error {
		_, err := db.New(sp).UpsertContractSigner(f.ctx, db.UpsertContractSignerParams{
			InstanceID: c1.ID, OrganizationID: c1.OrganizationID, BrandID: c1.BrandID,
			Role: "witness", Name: "T285 witness",
		})
		return err
	})
	signers, err := f.q.ListContractSigners(f.ctx, c1.ID)
	if err != nil || len(signers) != 2 {
		t.Fatalf("signers = %d, %v", len(signers), err)
	}

	// The OTP proof must be a consumed contract_sign code.
	var loginCode int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO otp_codes (phone_e164, code_hash, type, expires_at, consumed_at)
		VALUES ('+905551285001', 'x', 'customer_login', NOW() + interval '5 minutes', NOW()) RETURNING id`).Scan(&loginCode); err != nil {
		t.Fatal(err)
	}
	f.expectTrigger(t, "otp of another purpose", func(sp pgx.Tx) error {
		_, err := db.New(sp).SetContractSignerOTP(f.ctx, db.SetContractSignerOTPParams{
			ID: signers[0].ID, OtpCodeID: pgtype.Int8{Int64: loginCode, Valid: true},
		})
		return err
	})
	var signCode int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO otp_codes (phone_e164, code_hash, type, expires_at, consumed_at)
		VALUES ('+905551285001', 'x', 'contract_sign', NOW() + interval '5 minutes', NOW()) RETURNING id`).Scan(&signCode); err != nil {
		t.Fatal(err)
	}
	f.expectOK(t, "consumed contract_sign otp", func(sp pgx.Tx) error {
		_, err := db.New(sp).SetContractSignerOTP(f.ctx, db.SetContractSignerOTPParams{
			ID: signers[0].ID, OtpCodeID: pgtype.Int8{Int64: signCode, Valid: true},
		})
		return err
	})

	// Media: at most 12 MB.
	f.expectOK(t, "12 MB media", func(sp pgx.Tx) error {
		_, err := db.New(sp).InsertContractMedia(f.ctx, f.mediaParams(c1, 12*1024*1024))
		return err
	})
	f.expectConstraint(t, "media over 12 MB", "23514", "chk_contract_media_size", func(sp pgx.Tx) error {
		_, err := db.New(sp).InsertContractMedia(f.ctx, f.mediaParams(c1, 12*1024*1024+1))
		return err
	})

	// Signatures are append-only.
	sig, err := f.q.InsertContractSignature(f.ctx, db.InsertContractSignatureParams{
		SignerID: signers[0].ID, InstanceID: c1.ID, OrganizationID: c1.OrganizationID, BrandID: c1.BrandID,
		StorageKey: "contracts/t285/sig.png", Sha256: contractSHA,
	})
	if err != nil {
		t.Fatalf("signature: %v", err)
	}
	f.expectCode(t, "update signature", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE contract_signatures SET storage_key = 'x' WHERE id = $1`, sig.ID)
		return err
	}, "23001")
	f.expectCode(t, "delete signature", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `DELETE FROM contract_signatures WHERE id = $1`, sig.ID)
		return err
	}, "23001")
	// A signature belongs to a signer of the same contract.
	f.expectConstraint(t, "signer of another contract", "23503", "fk_contract_signatures_signer", func(sp pgx.Tx) error {
		_, err := db.New(sp).InsertContractSignature(f.ctx, db.InsertContractSignatureParams{
			SignerID: signers[0].ID, InstanceID: c2.ID, OrganizationID: c2.OrganizationID, BrandID: c2.BrandID,
			StorageKey: "contracts/t285/sig2.png", Sha256: contractSHA,
		})
		return err
	})

	// Executing needs the evidence; afterwards content and children are
	// locked, void is the only way out and it is final.
	f.expectConstraint(t, "execute without evidence", "23514", "chk_contract_instances_evidence", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE contract_instances SET status = 'executed', executed_at = NOW() WHERE id = $1`, c1.ID)
		return err
	})
	exec, err := f.q.ExecuteContractInstance(f.ctx, db.ExecuteContractInstanceParams{
		ID: c1.ID, RenderedHtml: "<p>Sözleşme</p>", ContentSha256: contractSHA,
	})
	if err != nil || exec.Status != "executed" {
		t.Fatalf("execute = %+v, %v", exec, err)
	}
	f.expectTrigger(t, "edit executed content", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE contract_instances SET rendered_html = '<p>x</p>' WHERE id = $1`, c1.ID)
		return err
	})
	f.expectTrigger(t, "executed back to draft", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE contract_instances SET status = 'draft', executed_at = NULL WHERE id = $1`, c1.ID)
		return err
	})
	f.expectTrigger(t, "media on executed", func(sp pgx.Tx) error {
		_, err := db.New(sp).InsertContractMedia(f.ctx, f.mediaParams(c1, 1024))
		return err
	})
	f.expectTrigger(t, "signer change on executed", func(sp pgx.Tx) error {
		_, err := db.New(sp).MarkContractSignerSigned(f.ctx, signers[1].ID)
		return err
	})
	f.expectCode(t, "delete executed", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `DELETE FROM contract_instances WHERE id = $1`, c1.ID)
		return err
	}, "23001")
	f.expectOK(t, "pdf key once", func(sp pgx.Tx) error {
		_, err := db.New(sp).SetContractInstancePDFKey(f.ctx, db.SetContractInstancePDFKeyParams{
			ID: c1.ID, PdfKey: "contracts/t285/c1.pdf",
		})
		return err
	})
	f.expectConstraint(t, "void without reason", "23514", "chk_contract_instances_voided", func(sp pgx.Tx) error {
		_, err := db.New(sp).VoidContractInstance(f.ctx, db.VoidContractInstanceParams{ID: c1.ID, VoidReason: " "})
		return err
	})
	voided, err := f.q.VoidContractInstance(f.ctx, db.VoidContractInstanceParams{ID: c1.ID, VoidReason: "wrong vehicle"})
	if err != nil || voided.Status != "voided" {
		t.Fatalf("void = %+v, %v", voided, err)
	}
	f.expectTrigger(t, "voided is final", func(sp pgx.Tx) error {
		_, err := sp.Exec(f.ctx, `UPDATE contract_instances SET void_reason = 'other' WHERE id = $1`, c1.ID)
		return err
	})
}
