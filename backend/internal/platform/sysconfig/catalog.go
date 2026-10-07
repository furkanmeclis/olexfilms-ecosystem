// Package sysconfig is the global system settings store (TEC-215, design
// §4 "Sistem ayarları"): a typed catalog of keys with defaults, JSONB rows
// in system_settings for overrides, and a short Redis cache in front.
//
// Adding a setting is one catalog entry: the catalog is the schema. The
// value type, bounds and default are checked here, so handlers and callers
// never see an out-of-range value.
package sysconfig

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/appversion"
)

// Kind is the JSON type a setting holds.
type Kind string

const (
	KindInt    Kind = "int"
	KindBool   Kind = "bool"
	KindString Kind = "string"
)

// Group names a settings page section; new areas (warehouse, scanning,
// ...) add their own group constant with their keys.
type Group string

const (
	GroupGeneral   Group = "general"
	GroupContracts Group = "contracts"
	GroupForecast  Group = "forecast"
	GroupServices  Group = "services"
	GroupSMTP      Group = "smtp"
	GroupWarehouse Group = "warehouse"
	GroupScanning  Group = "scanning"
	GroupMobile    Group = "mobile"
	GroupLeads     Group = "leads"
	// GroupWarrantyClaims: warranty claim accounting (TEC-337).
	GroupWarrantyClaims Group = "warranty_claims"
	// GroupWhatsApp: WhatsApp conversation messaging (TEC-395).
	GroupWhatsApp Group = "whatsapp"
	// GroupMCP: MCP endpoints (TEC-402).
	GroupMCP Group = "mcp"
	// GroupCampaigns: campaign sending (TEC-407).
	GroupCampaigns Group = "campaigns"
)

// SchemaVersion is stored with every row; bump it when a key's shape
// changes so stale rows can be recognized.
const SchemaVersion = 1

// Setting keys.
const (
	// KeyForecastMinDays is the minimum data window (days) before the stock
	// forecast proposes anything (K15).
	KeyForecastMinDays = "forecast_min_days"
	// KeyContractGraceDays is the read-only grace after a contract expires
	// (design §4). Default 0 keeps K23 ("grace yok"); raising it is a product
	// decision.
	KeyContractGraceDays = "contract_grace_days"
	// KeyContractsIntakeRequired blocks intake status moves until the linked
	// contract is executed, when the intake_contracts module is also enabled.
	KeyContractsIntakeRequired = "contracts.intake_required"
	// KeyPhotoStandardEnabled switches the vehicle intake photo standard
	// (design §4, default off).
	KeyPhotoStandardEnabled = "photo_standard_enabled"
	// KeyBulkUndoWindowHours is how long a bulk operation stays undoable
	// after it ran (TEC-212).
	KeyBulkUndoWindowHours = "bulk_undo_window_hours"

	KeySMTPHost     = "smtp.host"
	KeySMTPPort     = "smtp.port"
	KeySMTPUsername = "smtp.username"
	KeySMTPPassword = "smtp.password"
	KeySMTPFrom     = "smtp.from"
	KeySMTPFromName = "smtp.from_name"

	// Scanner (TEC-203): which inputs the universal scan resolver accepts
	// besides location QR payloads and unit barcodes, which always resolve.
	KeyScanSKUEnabled              = "scan.sku_enabled"
	KeyScanShortCodeEnabled        = "scan.short_code_enabled"
	KeyScanShortCodePrefix         = "scan.short_code_prefix"
	KeyScanBareLocationCodeEnabled = "scan.bare_location_code_enabled"

	// Mobile app version gate (TEC-236): apps below the minimum release get
	// 426 UPDATE_REQUIRED with the store links. Empty values fall back to
	// the environment (MOBILE_APP_MIN_VERSION, MOBILE_APP_STORE_URL_*).
	KeyMobileAppMinVersion      = "mobile.app_min_version"
	KeyMobileAppStoreURLIOS     = "mobile.app_store_url_ios"
	KeyMobileAppStoreURLAndroid = "mobile.app_store_url_android"
	// KeyMobileAppVersionRequired refuses a /v1/mobile/* request whose app
	// version cannot be read (no X-App-Version, no known User-Agent token)
	// while a minimum is set. Off: such requests pass.
	KeyMobileAppVersionRequired = "mobile.app_version_required"

	// KeyLeadsDealerApplicationEnabled opens the public dealer application
	// form (TEC-317). Conservative default: closed. It can only be switched
	// on while the leads module is open system wide (guarded at write time).
	KeyLeadsDealerApplicationEnabled = "leads.dealer_application_enabled"

	// Warranty claim labor rule (TEC-337, F3-06d): who bears the labor of a
	// re-application. dealer (default, conservative): the labor stays with
	// the dealer, the center pays none; center: the center credits the
	// dealer labor_amount down the chain; shared: the center credits
	// labor_share_percent percent of it. labor_amount is a decimal in the
	// center's currency.
	KeyWarrantyClaimsLaborRule         = "warranty_claims.labor_rule"
	KeyWarrantyClaimsLaborAmount       = "warranty_claims.labor_amount"
	KeyWarrantyClaimsLaborSharePercent = "warranty_claims.labor_share_percent"

	// KeyWhatsAppSendPerMinute caps outgoing conversation messages (AI,
	// staff, system) per recipient number per minute (TEC-395); campaigns
	// have their own limit.
	KeyWhatsAppSendPerMinute = "whatsapp.send_per_minute"
	// KeyWhatsAppAIStaffPauseMinutes: the AI does not answer a conversation
	// a staff member wrote in within this many minutes (TEC-396).
	KeyWhatsAppAIStaffPauseMinutes = "whatsapp.ai_staff_pause_minutes"
	// KeyWhatsAppAIGuidelinesURL is the AI guidelines link of the WhatsApp
	// consent question; empty = the guidelines text itself is sent (TEC-396).
	KeyWhatsAppAIGuidelinesURL = "whatsapp.ai_guidelines_url"

	// KeyMCPRequestsPerHourPerOrg caps the MCP requests of one organization
	// per hour over every connected app and endpoint (TEC-402).
	KeyMCPRequestsPerHourPerOrg = "mcp.requests_per_hour_per_org"

	// KeyCampaignsWhatsAppPerMinute caps the campaign WhatsApp messages over
	// all recipients per minute (TEC-407); conversation messages keep
	// KeyWhatsAppSendPerMinute.
	KeyCampaignsWhatsAppPerMinute = "campaigns.whatsapp_per_minute"
	// Campaign WhatsApp quiet hours in the recipient's time zone (TEC-407):
	// from quiet_hours_start:00 to quiet_hours_end:00 messages wait; equal
	// values switch the quiet hours off.
	KeyCampaignsQuietHoursStart = "campaigns.quiet_hours_start"
	KeyCampaignsQuietHoursEnd   = "campaigns.quiet_hours_end"
)

// Catalog defaults of the campaign keys (F4 QUESTIONS S14).
const (
	DefaultCampaignsWhatsAppPerMinute = 20
	DefaultCampaignsQuietHoursStart   = 21
	DefaultCampaignsQuietHoursEnd     = 9
)

// DefaultMCPRequestsPerHourPerOrg is the catalog default of
// KeyMCPRequestsPerHourPerOrg.
const DefaultMCPRequestsPerHourPerOrg = 600

// DefaultWhatsAppSendPerMinute is the catalog default of
// KeyWhatsAppSendPerMinute.
const DefaultWhatsAppSendPerMinute = 30

// DefaultWhatsAppAIStaffPauseMinutes is the catalog default of
// KeyWhatsAppAIStaffPauseMinutes.
const DefaultWhatsAppAIStaffPauseMinutes = 30

// Values of KeyWarrantyClaimsLaborRule.
const (
	LaborRuleDealer = "dealer"
	LaborRuleCenter = "center"
	LaborRuleShared = "shared"
)

func checkLaborRule(s string) string {
	switch s {
	case LaborRuleDealer, LaborRuleCenter, LaborRuleShared:
		return ""
	}
	return "must be dealer, center or shared"
}

var laborAmountRe = regexp.MustCompile(`^[0-9]{1,14}(\.[0-9]{1,2})?$`)

func checkLaborAmount(s string) string {
	if !laborAmountRe.MatchString(s) {
		return "must be a non-negative amount with at most 2 decimals, e.g. 150.00"
	}
	return ""
}

// DefaultBulkUndoWindowHours is the catalog default of KeyBulkUndoWindowHours.
const DefaultBulkUndoWindowHours = 24

// SecretMask replaces a secret value on read. Writing the mask back keeps
// the stored value, so a form can round-trip without revealing it.
const SecretMask = "********"

// Definition is one catalog entry.
type Definition struct {
	Key         string `json:"key"`
	Group       Group  `json:"group"`
	Kind        Kind   `json:"kind"`
	Default     any    `json:"default"`
	Description string `json:"description"`
	// Min/Max bound KindInt values (inclusive); MaxLen bounds KindString.
	Min    *int64 `json:"min,omitempty"`
	Max    *int64 `json:"max,omitempty"`
	MaxLen int    `json:"max_len,omitempty"`
	// Secret values are masked on read (SecretMask).
	Secret bool `json:"secret,omitempty"`
	// check further validates a non-empty KindString value; it returns the
	// error message or "".
	check func(string) string
}

func checkAppVersion(s string) string {
	if s != "" && !appversion.Valid(s) {
		return "must be a version such as 2.4.0"
	}
	return ""
}

func checkHTTPSURL(s string) string {
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "must be an https:// URL"
	}
	return ""
}

func i64(v int64) *int64 { return &v }

var catalog = []Definition{
	{Key: KeyForecastMinDays, Group: GroupForecast, Kind: KindInt, Default: int64(30), Min: i64(1), Max: i64(3650),
		Description: "Minimum days of movement history before the stock forecast proposes anything (K15)"},
	{Key: KeyContractGraceDays, Group: GroupContracts, Kind: KindInt, Default: int64(0), Min: i64(0), Max: i64(365),
		Description: "Read-only grace period after a contract expires; 0 = no grace (K23)"},
	{Key: KeyContractsIntakeRequired, Group: GroupContracts, Kind: KindBool, Default: false,
		Description: "Require an executed intake contract before a service can start processing or be completed directly"},
	{Key: KeyPhotoStandardEnabled, Group: GroupServices, Kind: KindBool, Default: false,
		Description: "Require the vehicle intake photo standard"},
	{Key: KeyBulkUndoWindowHours, Group: GroupGeneral, Kind: KindInt, Default: int64(DefaultBulkUndoWindowHours), Min: i64(1), Max: i64(720),
		Description: "Hours a bulk operation stays undoable after it ran (TEC-212)"},
	{Key: KeySMTPHost, Group: GroupSMTP, Kind: KindString, Default: "", MaxLen: 253,
		Description: "SMTP host; empty = use environment configuration"},
	{Key: KeySMTPPort, Group: GroupSMTP, Kind: KindInt, Default: int64(0), Min: i64(0), Max: i64(65535),
		Description: "SMTP port; 0 = use environment configuration"},
	{Key: KeySMTPUsername, Group: GroupSMTP, Kind: KindString, Default: "", MaxLen: 255,
		Description: "SMTP username"},
	{Key: KeySMTPPassword, Group: GroupSMTP, Kind: KindString, Default: "", MaxLen: 255, Secret: true,
		Description: "SMTP password (masked on read)"},
	{Key: KeySMTPFrom, Group: GroupSMTP, Kind: KindString, Default: "", MaxLen: 255,
		Description: "Sender address for outbound email"},
	{Key: KeySMTPFromName, Group: GroupSMTP, Kind: KindString, Default: "", MaxLen: 255,
		Description: "Sender display name for outbound email"},
	{Key: KeyScanSKUEnabled, Group: GroupScanning, Kind: KindBool, Default: true,
		Description: "Scanner resolves a product SKU to the product"},
	{Key: KeyScanShortCodeEnabled, Group: GroupScanning, Kind: KindBool, Default: true,
		Description: "Scanner resolves a digits-only short code (1-8 digits) to the generated barcode <PREFIX>-<8 digits>"},
	{Key: KeyScanShortCodePrefix, Group: GroupScanning, Kind: KindString, Default: "", MaxLen: 8,
		Description: "Barcode prefix of short codes (A-Z0-9, 2-8 characters); empty = the brand's default prefix"},
	{Key: KeyScanBareLocationCodeEnabled, Group: GroupScanning, Kind: KindBool, Default: false,
		Description: "Scanner also resolves a location full_code typed without the OFW:LOC: prefix"},
	{Key: KeyMobileAppMinVersion, Group: GroupMobile, Kind: KindString, Default: "", MaxLen: 32, check: checkAppVersion,
		Description: "Minimum mobile app version (e.g. 2.4.0); older apps get 426 UPDATE_REQUIRED. Empty = MOBILE_APP_MIN_VERSION from the environment; both empty = no gate"},
	{Key: KeyMobileAppStoreURLIOS, Group: GroupMobile, Kind: KindString, Default: "", MaxLen: 500, check: checkHTTPSURL,
		Description: "App Store link sent with UPDATE_REQUIRED; empty = MOBILE_APP_STORE_URL_IOS from the environment"},
	{Key: KeyMobileAppStoreURLAndroid, Group: GroupMobile, Kind: KindString, Default: "", MaxLen: 500, check: checkHTTPSURL,
		Description: "Google Play link sent with UPDATE_REQUIRED; empty = MOBILE_APP_STORE_URL_ANDROID from the environment"},
	{Key: KeyMobileAppVersionRequired, Group: GroupMobile, Kind: KindBool, Default: false,
		Description: "While a minimum version is set, also refuse mobile requests whose app version is unknown (no X-App-Version header or app User-Agent token)"},
	{Key: KeyLeadsDealerApplicationEnabled, Group: GroupLeads, Kind: KindBool, Default: false,
		Description: "Accept public dealer applications (/bayi-basvuru); only while the leads module is open system wide (TEC-317)"},
	{Key: KeyWarrantyClaimsLaborRule, Group: GroupWarrantyClaims, Kind: KindString, Default: LaborRuleDealer, MaxLen: 16, check: checkLaborRule,
		Description: "Who bears the labor of a warranty re-application: dealer (the center pays no labor), center (the center credits labor_amount to the dealer through the chain) or shared (labor_share_percent percent of it) (TEC-337)"},
	{Key: KeyWarrantyClaimsLaborAmount, Group: GroupWarrantyClaims, Kind: KindString, Default: "0.00", MaxLen: 18, check: checkLaborAmount,
		Description: "Labor amount of one warranty re-application in the center's currency; 0 = no labor entry (TEC-337)"},
	{Key: KeyWarrantyClaimsLaborSharePercent, Group: GroupWarrantyClaims, Kind: KindInt, Default: int64(50), Min: i64(0), Max: i64(100),
		Description: "Percent of the labor amount the center credits under the shared rule (TEC-337)"},
	{Key: KeyWhatsAppSendPerMinute, Group: GroupWhatsApp, Kind: KindInt, Default: int64(DefaultWhatsAppSendPerMinute), Min: i64(1), Max: i64(600),
		Description: "Outgoing WhatsApp conversation messages per recipient number per minute; further messages wait in the queue (TEC-395)"},
	{Key: KeyWhatsAppAIStaffPauseMinutes, Group: GroupWhatsApp, Kind: KindInt, Default: int64(DefaultWhatsAppAIStaffPauseMinutes), Min: i64(0), Max: i64(1440),
		Description: "The WhatsApp AI does not answer a conversation in which a staff member wrote within this many minutes; 0 = no pause (TEC-396)"},
	{Key: KeyWhatsAppAIGuidelinesURL, Group: GroupWhatsApp, Kind: KindString, Default: "", MaxLen: 500, check: checkHTTPSURL,
		Description: "Link to the AI guidelines sent with the WhatsApp consent question; empty = the guidelines text is sent in the message (TEC-396)"},
	{Key: KeyMCPRequestsPerHourPerOrg, Group: GroupMCP, Kind: KindInt, Default: int64(DefaultMCPRequestsPerHourPerOrg), Min: i64(1), Max: i64(100000),
		Description: "MCP requests one organization may make per hour over all connected apps; further requests get 429 (TEC-402)"},
	{Key: KeyCampaignsWhatsAppPerMinute, Group: GroupCampaigns, Kind: KindInt, Default: int64(DefaultCampaignsWhatsAppPerMinute), Min: i64(1), Max: i64(600),
		Description: "Campaign WhatsApp messages per minute over all recipients; further recipients wait (TEC-407)"},
	{Key: KeyCampaignsQuietHoursStart, Group: GroupCampaigns, Kind: KindInt, Default: int64(DefaultCampaignsQuietHoursStart), Min: i64(0), Max: i64(23),
		Description: "Hour (recipient's time zone) from which campaign WhatsApp messages wait until the quiet hours end (TEC-407)"},
	{Key: KeyCampaignsQuietHoursEnd, Group: GroupCampaigns, Kind: KindInt, Default: int64(DefaultCampaignsQuietHoursEnd), Min: i64(0), Max: i64(23),
		Description: "Hour (recipient's time zone) at which campaign WhatsApp quiet hours end; equal to the start = no quiet hours (TEC-407)"},
}

var byKey = func() map[string]Definition {
	m := make(map[string]Definition, len(catalog))
	for _, d := range catalog {
		if _, dup := m[d.Key]; dup {
			panic("sysconfig: duplicate key " + d.Key)
		}
		if _, err := d.Validate(mustJSON(d.Default)); err != nil {
			panic("sysconfig: default of " + d.Key + " fails its own schema: " + err.Error())
		}
		m[d.Key] = d
	}
	return m
}()

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// Catalog returns every definition, sorted by key.
func Catalog() []Definition {
	out := make([]Definition, len(catalog))
	copy(out, catalog)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Lookup returns the definition of key.
func Lookup(key string) (Definition, bool) {
	d, ok := byKey[key]
	return d, ok
}

// ValidationError is a value that does not match its definition.
type ValidationError struct {
	Key     string
	Message string
}

func (e *ValidationError) Error() string { return e.Key + ": " + e.Message }

// Validate checks raw against the definition and returns the canonical
// JSON encoding (an integer without fraction, a bare bool, a string).
func (d Definition) Validate(raw json.RawMessage) (json.RawMessage, error) {
	fail := func(msg string) (json.RawMessage, error) {
		return nil, &ValidationError{Key: d.Key, Message: msg}
	}
	// json.Unmarshal accepts null as the zero value for every kind, so it
	// is refused up front: "no value" is a reset (DELETE), not a write.
	if t := strings.TrimSpace(string(raw)); t == "" || t == "null" {
		return fail("value is required")
	}
	switch d.Kind {
	case KindInt:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return fail("must be an integer")
		}
		if f != math.Trunc(f) || math.IsInf(f, 0) || math.IsNaN(f) || math.Abs(f) > math.MaxInt32 {
			return fail("must be an integer")
		}
		n := int64(f)
		if d.Min != nil && n < *d.Min {
			return fail(fmt.Sprintf("must be at least %d", *d.Min))
		}
		if d.Max != nil && n > *d.Max {
			return fail(fmt.Sprintf("must be at most %d", *d.Max))
		}
		return mustJSON(n), nil
	case KindBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return fail("must be a boolean")
		}
		return mustJSON(b), nil
	case KindString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fail("must be a string")
		}
		if d.MaxLen > 0 && len([]rune(s)) > d.MaxLen {
			return fail(fmt.Sprintf("must be at most %d characters", d.MaxLen))
		}
		if d.check != nil {
			if msg := d.check(strings.TrimSpace(s)); msg != "" {
				return fail(msg)
			}
			s = strings.TrimSpace(s)
		}
		return mustJSON(s), nil
	}
	return fail("unknown kind")
}
