package storage

import (
	"fmt"

	"github.com/google/uuid"
)

// AppLogoObjectKey builds app/branding/logo.{ext} for system letterhead.
func AppLogoObjectKey(ext string) string {
	return fmt.Sprintf("app/branding/logo.%s", trimExt(ext))
}

// ExportObjectKey builds exports/{jobUUID}/export.{ext}.
func ExportObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("exports/%s/export.%s", jobUUID, trimExt(ext))
}

// ImportSourceObjectKey builds imports/{jobUUID}/source.{ext}.
func ImportSourceObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("imports/%s/source.%s", jobUUID, trimExt(ext))
}

// VehicleBrandLogoObjectKey builds vehicle-brands/{uuid}/logo-{version}.{ext}.
// The version changes on every upload, so the key (and the ETag derived from
// it) changes whenever the logo changes (TEC-149).
func VehicleBrandLogoObjectKey(brandUUID uuid.UUID, version, ext string) string {
	return fmt.Sprintf("vehicle-brands/%s/logo-%s.%s", brandUUID.String(), version, trimExt(ext))
}

// VehicleBrandHeroObjectKey builds vehicle-brands/{uuid}/hero-{version}.{ext}.
func VehicleBrandHeroObjectKey(brandUUID uuid.UUID, version, ext string) string {
	return fmt.Sprintf("vehicle-brands/%s/hero-%s.%s", brandUUID.String(), version, trimExt(ext))
}

// VehicleModelHeroObjectKey builds vehicle-models/{uuid}/hero-{version}.{ext}.
func VehicleModelHeroObjectKey(modelUUID uuid.UUID, version, ext string) string {
	return fmt.Sprintf("vehicle-models/%s/hero-%s.%s", modelUUID.String(), version, trimExt(ext))
}

// ProductImageObjectKey builds products/{uuid}/images/{imageKey} (TEC-152).
// imageKey is the flat, random image id stored in products.images.
func ProductImageObjectKey(productUUID uuid.UUID, imageKey string) string {
	return fmt.Sprintf("products/%s/images/%s", productUUID.String(), imageKey)
}

// ServiceImageObjectKey builds services/{org}/{service}/images/{image}.{ext}
// (TEC-179). The image uuid is random, so a key is never reused for other
// bytes.
func ServiceImageObjectKey(orgUUID, serviceUUID, imageUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("services/%s/%s/images/%s.%s", orgUUID.String(), serviceUUID.String(), imageUUID.String(), trimExt(ext))
}

// IntakePhotoObjectKey builds services/{org}/{service}/intake/{angle}/{photo}.{ext}
// (TEC-498). Intake photos live separately from service_images.
func IntakePhotoObjectKey(orgUUID, serviceUUID uuid.UUID, angleKey string, photoUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("services/%s/%s/intake/%s/%s.%s", orgUUID.String(), serviceUUID.String(), angleKey, photoUUID.String(), trimExt(ext))
}

// DocumentObjectKey builds documents/{org}/{kind}/{render}.pdf (TEC-88).
func DocumentObjectKey(orgUUID uuid.UUID, kind string, renderUUID uuid.UUID) string {
	return fmt.Sprintf("documents/%s/%s/%s.pdf", orgUUID.String(), kind, renderUUID.String())
}

// ContractExecutedPDFObjectKey builds contracts/{org}/{instance}/executed.pdf.
func ContractExecutedPDFObjectKey(orgUUID, instanceUUID uuid.UUID) string {
	return fmt.Sprintf("contracts/%s/%s/executed.pdf", orgUUID.String(), instanceUUID.String())
}

// ContractSignatureObjectKey builds contracts/{org}/{instance}/signatures/{signer}.png.
func ContractSignatureObjectKey(orgUUID, instanceUUID, signerUUID uuid.UUID) string {
	return fmt.Sprintf(
		"contracts/%s/%s/signatures/%s.png",
		orgUUID.String(),
		instanceUUID.String(),
		signerUUID.String(),
	)
}

// ContractMediaObjectKey builds contracts/{org}/{instance}/media/{media}.{ext}.
func ContractMediaObjectKey(orgUUID, instanceUUID, mediaUUID uuid.UUID, ext string) string {
	return fmt.Sprintf(
		"contracts/%s/%s/media/%s.%s",
		orgUUID.String(),
		instanceUUID.String(),
		mediaUUID.String(),
		trimExt(ext),
	)
}

func trimExt(ext string) string {
	for len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return ext
}

// MeasurementPDFObjectKey builds measurements/{org}/{result}/report.pdf
// (TEC-298: rendered once, the result is immutable).
func MeasurementPDFObjectKey(orgUUID, resultUUID uuid.UUID) string {
	return fmt.Sprintf("measurements/%s/%s/report.pdf", orgUUID.String(), resultUUID.String())
}

// WhatsAppMediaObjectKey builds whatsapp/{conversation}/{message} (TEC-395):
// inbound media copied from the gateway and outgoing documents/images. The
// object is private; the sniffed content type is stored with it.
func WhatsAppMediaObjectKey(conversationUUID, messageUUID uuid.UUID) string {
	return fmt.Sprintf("whatsapp/%s/%s", conversationUUID.String(), messageUUID.String())
}

// CampaignMediaObjectKey builds campaigns/{org}/{campaign}/{locale}/{media}.{ext}
// (TEC-405). The object is private; e-mails link to it instead of attaching.
func CampaignMediaObjectKey(orgUUID, campaignUUID uuid.UUID, locale string, mediaUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("campaigns/%s/%s/%s/%s.%s", orgUUID.String(), campaignUUID.String(), locale, mediaUUID.String(), trimExt(ext))
}

// ShowcasePhotoObjectKey builds showcases/{org}/{photo}.{ext} (TEC-467).
// The photo uuid is random, so a key is never reused for other bytes.
func ShowcasePhotoObjectKey(orgUUID, photoUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("showcases/%s/%s.%s", orgUUID.String(), photoUUID.String(), trimExt(ext))
}
