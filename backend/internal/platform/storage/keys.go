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

// VehicleBrandLogoObjectKey builds vehicle-brands/{uuid}/logo.{ext}.
func VehicleBrandLogoObjectKey(brandUUID uuid.UUID, ext string) string {
	return fmt.Sprintf("vehicle-brands/%s/logo.%s", brandUUID.String(), trimExt(ext))
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
