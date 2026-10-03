package response

// Error codes of the mobile API and QR web sign-in (TEC-91).
const (
	CodeMobileAPIVersionUnsupported = "MOBILE_API_VERSION_UNSUPPORTED"
	CodeRefreshTokenReused          = "REFRESH_TOKEN_REUSED"
	CodeNotMobileSession            = "NOT_MOBILE_SESSION"
	CodeNotImplemented              = "NOT_IMPLEMENTED"
	CodeQRLoginNotFound             = "QR_LOGIN_NOT_FOUND"
	CodeQRLoginExpired              = "QR_LOGIN_EXPIRED"
	CodeQRLoginRejected             = "QR_LOGIN_REJECTED"
	CodeQRLoginPending              = "QR_LOGIN_PENDING"
	CodeQRLoginClosed               = "QR_LOGIN_CLOSED"
	CodeQRLoginInvalidSecret        = "QR_LOGIN_INVALID_SECRET"
	CodeQRLoginDisabled             = "QR_LOGIN_DISABLED"

	// CodeUpdateRequired is the 426 of an app release below the minimum
	// (TEC-236).
	CodeUpdateRequired = "UPDATE_REQUIRED"
)
