package usecase

// TEC-190 (F1-06f): vehicle ownership transfer with two codes (TEC-98
// decision 6).
//
//   - Start: the dealer gives the vehicle and the new owner's phone (E.164,
//     K29). Two independent 6 digit codes are generated with crypto/rand,
//     one for the current owner and one for the new owner. Only keyed
//     hashes are stored (HMAC-SHA256 with a per-code random salt, the same
//     server key as the phone OTP); the codes are sent over WhatsApp (K16,
//     SMS fallback when the admin enabled it, K21) synchronously, like the
//     phone OTP, so a code never sits in clear text in the outbox or the
//     notification tables. The transfer row, the vehicle.transfer_started
//     event and the audit row commit only after both codes were handed to
//     the provider.
//   - Verify: the codes can be entered together or one by one. Every wrong
//     code counts one attempt; at the limit the transfer is cancelled. A
//     transfer past expires_at is marked expired (lazy here, and by the
//     vehicle_transfer:expire periodic task) and can no longer complete.
//   - Complete (both codes verified, same transaction): the vehicle owner
//     becomes the new user, the vehicle's active warranties move to the new
//     holder (one warranty.holder_changed event each), the new owner is
//     linked to the transferring organization and vehicle.transfer_completed
//     is written. Services keep their customer snapshot (000050): nothing
//     in services or service items changes.
//   - New owner: when the phone has an account at start, to_user_id is set
//     then. Otherwise to_user_id stays NULL and the user is resolved by the
//     phone at completion, or created there with the TEC-160 rules (phone
//     unverified until the first OTP, customer role, organization link);
//     creation needs the new owner's name in the verify request.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/phone"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Transfer statuses (vehicle_transfers.status).
const (
	TransferPending   = "pending"
	TransferCompleted = "completed"
	TransferCancelled = "cancelled"
	TransferExpired   = "expired"
)

// Audit actions of the transfer flow.
const (
	ActionTransferStarted   = "vehicles.transfer_started"
	ActionTransferCompleted = "vehicles.transfer_completed"
	ActionTransferCancelled = "vehicles.transfer_cancelled"
	ActionTransferFailed    = "vehicles.transfer_code_failed"
)

// Cancel reasons in the vehicle.transfer_cancelled payload.
const (
	CancelReasonManual      = "cancelled"
	CancelReasonTooManyCode = "too_many_attempts"
)

// Transfer defaults (TEC-98 research: 15 minutes, 5 attempts).
const (
	DefaultTransferTTL         = 15 * time.Minute
	DefaultTransferMaxAttempts = 5
	transferCodeLen            = 6
	transferHashVersion        = "v1"
)

var (
	// ErrTransferUnavailable: no message sender is configured.
	ErrTransferUnavailable = errors.New("customers: vehicle transfer is not available")
	// ErrTransferNotFound: unknown transfer or out of scope.
	ErrTransferNotFound = errors.New("customers: vehicle transfer not found")
	// ErrTransferPending: the vehicle already has an open transfer.
	ErrTransferPending = errors.New("customers: vehicle already has a pending transfer")
	// ErrTransferSameOwner: the new owner is the current owner.
	ErrTransferSameOwner = errors.New("customers: new owner is the current owner")
	// ErrTransferOwnerNoPhone: the current owner has no phone for the code.
	ErrTransferOwnerNoPhone = errors.New("customers: current owner has no phone")
	// ErrTransferNotPending: the transfer is completed, cancelled or expired.
	ErrTransferNotPending = errors.New("customers: vehicle transfer is not pending")
	// ErrTransferExpired: the transfer passed expires_at.
	ErrTransferExpired = errors.New("customers: vehicle transfer expired")
	// ErrTransferLocked: too many wrong codes; the transfer was cancelled.
	ErrTransferLocked = errors.New("customers: too many wrong codes, transfer cancelled")
	// ErrTransferDelivery: a code could not be delivered; nothing was saved.
	ErrTransferDelivery = errors.New("customers: transfer code could not be delivered")
	// ErrTransferOwnerChanged: the vehicle owner changed meanwhile.
	ErrTransferOwnerChanged = errors.New("customers: vehicle owner changed")
)

// InvalidTransferCodeError is a wrong code; Remaining attempts are left.
type InvalidTransferCodeError struct {
	Fields    []string
	Remaining int32
}

func (e *InvalidTransferCodeError) Error() string {
	return fmt.Sprintf("customers: invalid transfer code (%d attempts left)", e.Remaining)
}

// TransferSender delivers a code text (whatsapp.Sender: WhatsApp, then SMS).
type TransferSender interface {
	SendText(ctx context.Context, to, body string, opts whatsapp.SendOptions) (whatsapp.Delivery, error)
}

// TransferConfig tunes the transfer flow. Zero values take the defaults.
type TransferConfig struct {
	TTL         time.Duration
	MaxAttempts int32
	// Key is the server secret of the code hashes (otp.DeriveKey).
	Key     []byte
	AppName string
}

type transfers struct {
	sender TransferSender
	cfg    TransferConfig
	now    func() time.Time
}

// SetTransfers enables the vehicle transfer flow.
func (s *Service) SetTransfers(sender TransferSender, cfg TransferConfig) {
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultTransferTTL
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = DefaultTransferMaxAttempts
	}
	if cfg.AppName == "" {
		cfg.AppName = "Olexfilms"
	}
	now := time.Now
	if s.tr != nil && s.tr.now != nil {
		now = s.tr.now
	}
	s.tr = &transfers{sender: sender, cfg: cfg, now: now}
}

// SetTransferClock replaces the clock of the transfer flow (tests).
func (s *Service) SetTransferClock(now func() time.Time) {
	if s.tr == nil {
		s.tr = &transfers{cfg: TransferConfig{TTL: DefaultTransferTTL, MaxAttempts: DefaultTransferMaxAttempts}}
	}
	s.tr.now = now
}

func (s *Service) transferNow() time.Time {
	if s.tr != nil && s.tr.now != nil {
		return s.tr.now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) transferMaxAttempts() int32 {
	if s.tr != nil && s.tr.cfg.MaxAttempts > 0 {
		return s.tr.cfg.MaxAttempts
	}
	return DefaultTransferMaxAttempts
}

// VehicleTransferView is a transfer as the panel sees it. Codes and their
// hashes never leave the server; the new owner's phone is masked.
type VehicleTransferView struct {
	UUID           uuid.UUID  `json:"uuid"`
	VehicleUUID    uuid.UUID  `json:"vehicle_uuid"`
	Status         string     `json:"status"`
	ToPhoneMasked  string     `json:"to_phone_masked"`
	NewOwnerKnown  bool       `json:"new_owner_known"`
	FromVerified   bool       `json:"from_verified"`
	ToVerified     bool       `json:"to_verified"`
	Attempts       int32      `json:"attempts"`
	MaxAttempts    int32      `json:"max_attempts"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	CancelledAt    *time.Time `json:"cancelled_at"`
	WarrantiesMove int        `json:"warranties_moved"`
}

// StartTransferInput is POST /v1/vehicles/{uuid}/transfers.
type StartTransferInput struct {
	Phone string `json:"phone"`
}

// VerifyTransferInput is POST /v1/vehicle-transfers/{uuid}/verify. At least
// one code; name / surname are used only when the new owner has no
// account yet and is created at completion.
type VerifyTransferInput struct {
	FromCode string `json:"from_code"`
	ToCode   string `json:"to_code"`
	Name     string `json:"new_owner_name"`
	Surname  string `json:"new_owner_surname"`
}

func (s *Service) transferView(t db.VehicleTransfer, vehicleUUID uuid.UUID, now time.Time) VehicleTransferView {
	status := t.Status
	if status == TransferPending && !t.ExpiresAt.Time.After(now) {
		status = TransferExpired // the periodic task / next write records it
	}
	return VehicleTransferView{
		UUID: t.Uuid, VehicleUUID: vehicleUUID, Status: status,
		ToPhoneMasked: phone.Mask(t.ToPhone), NewOwnerKnown: t.ToUserID.Valid,
		FromVerified: t.FromVerifiedAt.Valid, ToVerified: t.ToVerifiedAt.Valid,
		Attempts: t.Attempts, MaxAttempts: s.transferMaxAttempts(),
		ExpiresAt: t.ExpiresAt.Time.UTC(), CreatedAt: t.CreatedAt.Time.UTC(),
		CompletedAt: timePtr(t.CompletedAt), CancelledAt: timePtr(t.CancelledAt),
	}
}

// ListVehicleTransfers returns the transfers of a vehicle, newest first.
func (s *Service) ListVehicleTransfers(ctx context.Context, c Caller, vehicleID uuid.UUID) ([]VehicleTransferView, error) {
	v, err := s.scopedVehicle(ctx, s.q, c, vehicleID, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListVehicleTransfersByVehicle(ctx, db.ListVehicleTransfersByVehicleParams{VehicleID: v.ID, BrandID: v.BrandID})
	if err != nil {
		return nil, fmt.Errorf("customers: list transfers: %w", err)
	}
	now := s.transferNow()
	out := make([]VehicleTransferView, 0, len(rows))
	for _, t := range rows {
		out = append(out, s.transferView(t, v.Uuid, now))
	}
	return out, nil
}

// StartTransfer opens a transfer and sends both codes.
func (s *Service) StartTransfer(ctx context.Context, c Caller, vehicleID uuid.UUID, in StartTransferInput, meta activity.Meta) (VehicleTransferView, error) {
	if s.tr == nil || s.tr.sender == nil || len(s.tr.cfg.Key) == 0 {
		return VehicleTransferView{}, ErrTransferUnavailable
	}
	if c.Org.InternalID == 0 || !c.Filter.AllowsOrg(c.Org.InternalID, c.Org.BrandID) {
		return VehicleTransferView{}, ErrForbidden
	}
	if strings.TrimSpace(in.Phone) == "" {
		return VehicleTransferView{}, invalid("phone", "is required")
	}
	iso2, err := s.q.GetOrganizationCountryISO2(ctx, c.Org.InternalID)
	if err != nil {
		return VehicleTransferView{}, fmt.Errorf("customers: organization country: %w", err)
	}
	toPhone, err := phone.NormalizeE164(in.Phone, phone.Region(iso2))
	if err != nil || toPhone == "" {
		return VehicleTransferView{}, invalid("phone", "must be a valid phone number")
	}
	org, err := s.q.GetOrganizationByID(ctx, c.Org.InternalID)
	if err != nil {
		return VehicleTransferView{}, fmt.Errorf("customers: organization: %w", err)
	}
	now := s.transferNow()

	var view VehicleTransferView
	err = s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
		v, err := s.scopedVehicle(ctx, q, c, vehicleID, true)
		if err != nil {
			return err
		}
		from, err := q.GetUserByID(ctx, v.UserID)
		if err != nil {
			return fmt.Errorf("customers: owner: %w", err)
		}
		if err := writableUser(from); err != nil {
			return err
		}
		if !from.PhoneE164.Valid || from.PhoneE164.String == "" {
			return ErrTransferOwnerNoPhone
		}
		if from.PhoneE164.String == toPhone {
			return ErrTransferSameOwner
		}
		var toUserID pgtype.Int8
		toLocale := org.Locale
		to, err := q.GetUserByPhone(ctx, text(toPhone))
		switch {
		case err == nil:
			if to.ID == from.ID {
				return ErrTransferSameOwner
			}
			if err := writableUser(to); err != nil {
				return err
			}
			toUserID = pgtype.Int8{Int64: to.ID, Valid: true}
			if to.Locale.Valid {
				toLocale = to.Locale.String
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("customers: new owner lookup: %w", err)
		}
		// An expired but not yet swept transfer must not block a new one.
		if _, err := s.expireDue(ctx, q, tx, now); err != nil {
			return err
		}
		if _, err := q.GetPendingVehicleTransfer(ctx, v.ID); err == nil {
			return ErrTransferPending
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("customers: pending transfer: %w", err)
		}

		fromCode, err := generateTransferCode()
		if err != nil {
			return err
		}
		toCode, err := generateTransferCode()
		if err != nil {
			return err
		}
		fromHash, err := s.hashTransferCode("from", fromCode)
		if err != nil {
			return err
		}
		toHash, err := s.hashTransferCode("to", toCode)
		if err != nil {
			return err
		}
		actor := pgtype.Int8{Int64: c.UserID, Valid: c.UserID != 0}
		t, err := q.CreateVehicleTransfer(ctx, db.CreateVehicleTransferParams{
			OrganizationID: c.Org.InternalID, BrandID: v.BrandID, VehicleID: v.ID, FromUserID: from.ID,
			ToUserID: toUserID, ToPhone: toPhone, FromCodeHash: fromHash, ToCodeHash: toHash,
			ExpiresAt: pgtype.Timestamptz{Time: now.Add(s.tr.cfg.TTL), Valid: true}, InitiatedByUserID: actor,
		})
		if err != nil {
			if isUniqueViolation(err, "uq_vehicle_transfers_pending") {
				return ErrTransferPending
			}
			return fmt.Errorf("customers: create transfer: %w", err)
		}
		payload := transferPayload(t, v)
		if err := s.enqueue(ctx, tx, c, events.VehicleTransferStarted, t, payload); err != nil {
			return err
		}
		if err := s.auditTransfer(ctx, q, c, t, ActionTransferStarted, payload, meta); err != nil {
			return err
		}
		// Both codes are handed to the provider before the commit: a failed
		// delivery rolls the transfer back, so no unusable pending transfer
		// blocks the vehicle.
		label := vehicleLabel(v)
		minutes := int(s.tr.cfg.TTL / time.Minute)
		fromLocale := org.Locale
		if from.Locale.Valid {
			fromLocale = from.Locale.String
		}
		msgs := []struct{ side, to, body string }{
			{"from", from.PhoneE164.String, RenderTransferMessage(fromLocale, "from", s.tr.cfg.AppName, org.Name, label, fromCode, minutes)},
			{"to", toPhone, RenderTransferMessage(toLocale, "to", s.tr.cfg.AppName, org.Name, label, toCode, minutes)},
		}
		for _, m := range msgs {
			if _, err := s.tr.sender.SendText(ctx, m.to, m.body, whatsapp.SendOptions{ID: transferMessageID(t.Uuid, m.side)}); err != nil {
				return fmt.Errorf("%w: %v", ErrTransferDelivery, err)
			}
		}
		view = s.transferView(t, v.Uuid, now)
		return nil
	})
	if err != nil {
		return VehicleTransferView{}, err
	}
	return view, nil
}

// lockScopedTransfer locks a transfer of the domain brand whose vehicle
// owner (the transfer's current owner) is in the caller's scope.
func (s *Service) lockScopedTransfer(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (db.VehicleTransfer, db.Vehicle, error) {
	if c.Org.BrandID == 0 {
		return db.VehicleTransfer{}, db.Vehicle{}, ErrTransferNotFound
	}
	t, err := q.LockVehicleTransferByUUID(ctx, db.LockVehicleTransferByUUIDParams{Uuid: id, BrandID: c.Org.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.VehicleTransfer{}, db.Vehicle{}, ErrTransferNotFound
	}
	if err != nil {
		return db.VehicleTransfer{}, db.Vehicle{}, fmt.Errorf("customers: transfer: %w", err)
	}
	if err := s.requireInScope(ctx, q, c, t.FromUserID); err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			return db.VehicleTransfer{}, db.Vehicle{}, ErrTransferNotFound
		}
		return db.VehicleTransfer{}, db.Vehicle{}, err
	}
	v, err := q.GetVehicleByID(ctx, t.VehicleID)
	if err != nil {
		return db.VehicleTransfer{}, db.Vehicle{}, fmt.Errorf("customers: transfer vehicle: %w", err)
	}
	return t, v, nil
}

// VerifyTransfer checks the given codes and completes the transfer once
// both sides are verified. Wrong codes and expiry are committed before the
// error is returned (attempt counter, cancellation, expired status).
func (s *Service) VerifyTransfer(ctx context.Context, c Caller, id uuid.UUID, in VerifyTransferInput, meta activity.Meta) (VehicleTransferView, error) {
	if s.tr == nil || len(s.tr.cfg.Key) == 0 {
		return VehicleTransferView{}, ErrTransferUnavailable
	}
	fromCode, toCode := strings.TrimSpace(in.FromCode), strings.TrimSpace(in.ToCode)
	if fromCode == "" && toCode == "" {
		return VehicleTransferView{}, invalid("from_code", "at least one code is required")
	}
	for field, code := range map[string]string{"from_code": fromCode, "to_code": toCode} {
		if code != "" && !validTransferCode(code) {
			return VehicleTransferView{}, invalid(field, "must be 6 digits")
		}
	}
	name, err := normalizeName(in.Name, "new_owner_name")
	if err != nil {
		return VehicleTransferView{}, err
	}
	surname, err := normalizeName(in.Surname, "new_owner_surname")
	if err != nil {
		return VehicleTransferView{}, err
	}
	now := s.transferNow()
	maxAttempts := s.transferMaxAttempts()

	var (
		view   VehicleTransferView
		result error // committed outcome (wrong code, expired, locked)
	)
	err = s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
		t, v, err := s.lockScopedTransfer(ctx, q, c, id)
		if err != nil {
			return err
		}
		if t.Status != TransferPending {
			return ErrTransferNotPending
		}
		if !t.ExpiresAt.Time.After(now) {
			if _, err := s.expireDue(ctx, q, tx, now); err != nil {
				return err
			}
			result = ErrTransferExpired
			return nil
		}
		if t.Attempts >= maxAttempts {
			if err := s.cancelTransfer(ctx, q, tx, c, t, v, CancelReasonTooManyCode, meta); err != nil {
				return err
			}
			result = ErrTransferLocked
			return nil
		}
		var wrong []string
		fromOK, toOK := false, false
		if fromCode != "" && !t.FromVerifiedAt.Valid {
			if s.checkTransferCode(t.FromCodeHash, "from", fromCode) {
				fromOK = true
			} else {
				wrong = append(wrong, "from_code")
			}
		}
		if toCode != "" && !t.ToVerifiedAt.Valid {
			if s.checkTransferCode(t.ToCodeHash, "to", toCode) {
				toOK = true
			} else {
				wrong = append(wrong, "to_code")
			}
		}
		if fromOK || toOK {
			if t, err = q.SetVehicleTransferVerified(ctx, db.SetVehicleTransferVerifiedParams{
				ID: t.ID, FromSide: fromOK, ToSide: toOK,
			}); err != nil {
				return fmt.Errorf("customers: verify transfer: %w", err)
			}
		}
		if len(wrong) > 0 {
			if t, err = q.IncrementVehicleTransferAttempts(ctx, t.ID); err != nil {
				return fmt.Errorf("customers: transfer attempts: %w", err)
			}
			payload := transferPayload(t, v)
			payload["wrong_codes"] = wrong
			payload["attempts"] = t.Attempts
			if err := s.auditTransfer(ctx, q, c, t, ActionTransferFailed, payload, meta); err != nil {
				return err
			}
			if t.Attempts >= maxAttempts {
				if err := s.cancelTransfer(ctx, q, tx, c, t, v, CancelReasonTooManyCode, meta); err != nil {
					return err
				}
				result = ErrTransferLocked
				return nil
			}
			result = &InvalidTransferCodeError{Fields: wrong, Remaining: maxAttempts - t.Attempts}
			view = s.transferView(t, v.Uuid, now)
			return nil
		}
		if !t.FromVerifiedAt.Valid || !t.ToVerifiedAt.Valid {
			view = s.transferView(t, v.Uuid, now) // one side still open
			return nil
		}
		// A new owner without an account is created here and needs a name
		// (TEC-160); a missing name rolls the whole request back, so the
		// codes stay unverified and no attempt is spent.
		moved, t, err := s.completeTransfer(ctx, q, tx, c, t, v, name, surname, meta)
		if err != nil {
			return err
		}
		view = s.transferView(t, v.Uuid, now)
		view.WarrantiesMove = moved
		return nil
	})
	if err != nil {
		return VehicleTransferView{}, err
	}
	if result != nil {
		return view, result
	}
	return view, nil
}

// completeTransfer moves the vehicle and its active warranties to the new
// owner in the caller's transaction. Services are not touched.
func (s *Service) completeTransfer(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, t db.VehicleTransfer,
	v db.Vehicle, name, surname string, meta activity.Meta) (int, db.VehicleTransfer, error) {
	to, err := s.resolveNewOwner(ctx, q, t, name, surname)
	if err != nil {
		return 0, t, err
	}
	if to.ID == t.FromUserID {
		return 0, t, ErrTransferSameOwner
	}
	if _, err := q.SetVehicleOwner(ctx, db.SetVehicleOwnerParams{ID: v.ID, OldUserID: t.FromUserID, NewUserID: to.ID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, t, ErrTransferOwnerChanged
		}
		return 0, t, fmt.Errorf("customers: set vehicle owner: %w", err)
	}
	moved, err := q.ChangeWarrantyHolderByVehicle(ctx, db.ChangeWarrantyHolderByVehicleParams{VehicleID: v.ID, NewHolderUserID: to.ID})
	if err != nil {
		return 0, t, fmt.Errorf("customers: warranty holders: %w", err)
	}
	done, err := q.CompleteVehicleTransfer(ctx, db.CompleteVehicleTransferParams{ID: t.ID, ToUserID: pgtype.Int8{Int64: to.ID, Valid: true}})
	if err != nil {
		return 0, t, fmt.Errorf("customers: complete transfer: %w", err)
	}
	for _, w := range moved {
		id, u := w.ID, w.Uuid
		ev := events.New(events.WarrantyHolderChanged).WithTenant(w.OrganizationID).
			WithEntity("warranty", &id, &u).
			WithPayload(map[string]any{
				"warranty_uuid":        w.Uuid.String(),
				"organization_id":      w.OrganizationID,
				"brand_id":             w.BrandID,
				"vehicle_id":           w.VehicleID,
				"previous_holder_id":   t.FromUserID,
				"holder_user_id":       to.ID,
				"vehicle_transfer_uid": done.Uuid.String(),
			})
		if c.UserID != 0 {
			ev = ev.WithActor(c.UserID)
		}
		if s.out != nil {
			if err := s.out.Enqueue(ctx, tx, ev); err != nil {
				return 0, t, fmt.Errorf("customers: outbox: %w", err)
			}
		}
	}
	payload := transferPayload(done, v)
	payload["to_user_uuid"] = to.Uuid.String()
	payload["warranties_moved"] = len(moved)
	if err := s.enqueue(ctx, tx, c, events.VehicleTransferCompleted, done, payload); err != nil {
		return 0, t, err
	}
	if err := s.auditTransfer(ctx, q, c, done, ActionTransferCompleted, payload, meta); err != nil {
		return 0, t, err
	}
	return len(moved), done, nil
}

// resolveNewOwner returns the transfer's new owner: the user set at start,
// else the phone's account, else a new customer (TEC-160 rules). The user
// gets the customer role and a link to the transferring organization.
func (s *Service) resolveNewOwner(ctx context.Context, q *db.Queries, t db.VehicleTransfer, name, surname string) (db.User, error) {
	var (
		user db.User
		err  error
	)
	if t.ToUserID.Valid {
		user, err = q.GetUserByID(ctx, t.ToUserID.Int64)
	} else {
		user, err = q.GetUserByPhone(ctx, text(t.ToPhone))
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows) && !t.ToUserID.Valid:
		if name == "" {
			return db.User{}, invalid("new_owner_name", "is required for a new customer")
		}
		hash, err := unusablePasswordHash()
		if err != nil {
			return db.User{}, err
		}
		// Phone unverified until the first WhatsApp OTP (K26 claim).
		user, err = q.CreateUser(ctx, db.CreateUserParams{
			PasswordHash: hash, Name: name, Surname: surname, Status: StatusActive, PhoneE164: text(t.ToPhone),
		})
		if err != nil {
			return db.User{}, fmt.Errorf("customers: create new owner: %w", err)
		}
	case err != nil:
		return db.User{}, fmt.Errorf("customers: new owner: %w", err)
	default:
		if err := writableUser(user); err != nil {
			return db.User{}, err
		}
	}
	if err := q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: user.ID, Slug: rbac.RoleCustomer}); err != nil {
		return db.User{}, err
	}
	if _, err := q.LinkCustomerOrganization(ctx, db.LinkCustomerOrganizationParams{
		UserID: user.ID, OrganizationID: t.OrganizationID, BrandID: t.BrandID,
	}); err != nil {
		return db.User{}, fmt.Errorf("customers: link new owner: %w", err)
	}
	return user, nil
}

// CancelTransfer cancels a pending transfer.
func (s *Service) CancelTransfer(ctx context.Context, c Caller, id uuid.UUID, meta activity.Meta) (VehicleTransferView, error) {
	now := s.transferNow()
	var (
		view   VehicleTransferView
		result error
	)
	err := s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
		t, v, err := s.lockScopedTransfer(ctx, q, c, id)
		if err != nil {
			return err
		}
		if t.Status != TransferPending {
			return ErrTransferNotPending
		}
		if !t.ExpiresAt.Time.After(now) {
			if _, err := s.expireDue(ctx, q, tx, now); err != nil {
				return err
			}
			result = ErrTransferExpired
			return nil
		}
		if err := s.cancelTransfer(ctx, q, tx, c, t, v, CancelReasonManual, meta); err != nil {
			return err
		}
		t.Status = TransferCancelled
		t.CancelledAt = pgtype.Timestamptz{Time: now, Valid: true}
		view = s.transferView(t, v.Uuid, now)
		return nil
	})
	if err != nil {
		return VehicleTransferView{}, err
	}
	return view, result
}

func (s *Service) cancelTransfer(ctx context.Context, q *db.Queries, tx pgx.Tx, c Caller, t db.VehicleTransfer,
	v db.Vehicle, reason string, meta activity.Meta) error {
	done, err := q.CancelVehicleTransfer(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("customers: cancel transfer: %w", err)
	}
	payload := transferPayload(done, v)
	payload["reason"] = reason
	if err := s.enqueue(ctx, tx, c, events.VehicleTransferCancelled, done, payload); err != nil {
		return err
	}
	return s.auditTransfer(ctx, q, c, done, ActionTransferCancelled, payload, meta)
}

// ExpireTransfersTask is the vehicle_transfer:expire handler.
func (s *Service) ExpireTransfersTask(ctx context.Context) error {
	_, err := s.ExpireTransfers(ctx, s.transferNow())
	return err
}

// ExpireTransfers marks pending transfers past expires_at as expired and
// writes one vehicle.transfer_expired event each; a second run writes
// nothing.
func (s *Service) ExpireTransfers(ctx context.Context, now time.Time) (int, error) {
	var n int
	err := s.inTxRaw(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var err error
		n, err = s.expireDue(ctx, q, tx, now)
		return err
	})
	return n, err
}

func (s *Service) expireDue(ctx context.Context, q *db.Queries, tx pgx.Tx, now time.Time) (int, error) {
	rows, err := q.ExpireDueVehicleTransfers(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("customers: expire transfers: %w", err)
	}
	for _, t := range rows {
		payload := map[string]any{
			"transfer_uuid":   t.Uuid.String(),
			"organization_id": t.OrganizationID,
			"brand_id":        t.BrandID,
			"vehicle_id":      t.VehicleID,
			"from_user_id":    t.FromUserID,
			"expires_at":      t.ExpiresAt.Time.UTC().Format(time.RFC3339),
		}
		if t.ToUserID.Valid {
			payload["to_user_id"] = t.ToUserID.Int64
		}
		if err := s.enqueue(ctx, tx, Caller{}, events.VehicleTransferExpired, t, payload); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

func (s *Service) enqueue(ctx context.Context, tx pgx.Tx, c Caller, name string, t db.VehicleTransfer, payload map[string]any) error {
	if s.out == nil {
		return nil
	}
	id, u := t.ID, t.Uuid
	ev := events.New(name).WithTenant(t.OrganizationID).
		WithEntity("vehicle_transfer", &id, &u).WithPayload(payload)
	if c.UserID != 0 {
		ev = ev.WithActor(c.UserID)
	}
	if err := s.out.Enqueue(ctx, tx, ev); err != nil {
		return fmt.Errorf("customers: outbox: %w", err)
	}
	return nil
}

func (s *Service) auditTransfer(ctx context.Context, q *db.Queries, c Caller, t db.VehicleTransfer, action string,
	payload map[string]any, meta activity.Meta) error {
	var actor *int64
	if c.UserID != 0 {
		a := c.UserID
		actor = &a
	}
	body := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		body[k] = v
	}
	if c.Org.UUID != uuid.Nil {
		body["organization_uuid"] = c.Org.UUID.String()
	}
	if err := activity.Write(ctx, q, actor, action, "vehicle_transfers", &t.Uuid, body, meta); err != nil {
		return fmt.Errorf("customers: audit: %w", err)
	}
	return nil
}

func transferPayload(t db.VehicleTransfer, v db.Vehicle) map[string]any {
	p := map[string]any{
		"transfer_uuid":   t.Uuid.String(),
		"organization_id": t.OrganizationID,
		"brand_id":        t.BrandID,
		"vehicle_id":      v.ID,
		"vehicle_uuid":    v.Uuid.String(),
		"from_user_id":    t.FromUserID,
		"to_phone_masked": phone.Mask(t.ToPhone),
		"status":          t.Status,
		"expires_at":      t.ExpiresAt.Time.UTC().Format(time.RFC3339),
	}
	if t.ToUserID.Valid {
		p["to_user_id"] = t.ToUserID.Int64
	}
	return p
}

func vehicleLabel(v db.Vehicle) string {
	switch {
	case v.Plate.Valid && v.Plate.String != "":
		return v.Plate.String
	case v.Vin.Valid && v.Vin.String != "":
		return v.Vin.String
	}
	return "-"
}

// --- Codes ---------------------------------------------------------------

func generateTransferCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("customers: transfer code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func validTransferCode(code string) bool {
	if len(code) != transferCodeLen {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// hashTransferCode is "v1$<salt hex>$<HMAC-SHA256(key, salt:side:code) hex>".
// The random salt keeps equal codes of different transfers apart; side
// keeps the two codes of one transfer from being swapped.
func (s *Service) hashTransferCode(side, code string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("customers: transfer salt: %w", err)
	}
	saltHex := hex.EncodeToString(salt)
	return transferHashVersion + "$" + saltHex + "$" + transferMAC(s.tr.cfg.Key, saltHex, side, code), nil
}

func (s *Service) checkTransferCode(stored, side, code string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 || parts[0] != transferHashVersion {
		return false
	}
	want := transferMAC(s.tr.cfg.Key, parts[1], side, code)
	return subtle.ConstantTimeCompare([]byte(want), []byte(parts[2])) == 1
}

func transferMAC(key []byte, saltHex, side, code string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(saltHex + ":" + side + ":" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

// transferMessageID is an idempotent provider message id per transfer side.
func transferMessageID(id uuid.UUID, side string) string {
	return strings.ToUpper("VT" + side + strings.ReplaceAll(id.String(), "-", ""))
}

// --- Messages ------------------------------------------------------------

// transferMessages holds the code texts per locale (K10, 13 locales):
// [0] goes to the current owner, [1] to the new owner. Placeholders:
// {app} {org} {vehicle} {code} {minutes}.
var transferMessages = map[i18n.Locale][2]string{
	"tr": {
		"{app} – {org}: {vehicle} plakalı aracınızın devri için onay kodunuz: {code}\nKod {minutes} dakika geçerlidir. Devri onaylıyorsanız kodu bayiye iletin; aksi halde kimseyle paylaşmayın.",
		"{app} – {org}: {vehicle} plakalı aracın size devri için onay kodunuz: {code}\nKod {minutes} dakika geçerlidir. Devri onaylıyorsanız kodu bayiye iletin; aksi halde kimseyle paylaşmayın.",
	},
	"en": {
		"{app} – {org}: your code to approve the transfer of your vehicle {vehicle}: {code}\nValid for {minutes} minutes. Give it to the dealer only if you approve the transfer; otherwise do not share it.",
		"{app} – {org}: your code to take over the vehicle {vehicle}: {code}\nValid for {minutes} minutes. Give it to the dealer only if you approve the transfer; otherwise do not share it.",
	},
	"bg": {
		"{app} – {org}: вашият код за потвърждаване на прехвърлянето на автомобил {vehicle}: {code}\nВалиден е {minutes} минути. Дайте го на дилъра само ако одобрявате прехвърлянето; иначе не го споделяйте.",
		"{app} – {org}: вашият код за приемане на автомобил {vehicle}: {code}\nВалиден е {minutes} минути. Дайте го на дилъра само ако одобрявате прехвърлянето; иначе не го споделяйте.",
	},
	"de": {
		"{app} – {org}: Ihr Code zur Bestätigung der Übertragung Ihres Fahrzeugs {vehicle}: {code}\nGültig für {minutes} Minuten. Geben Sie ihn nur an den Händler weiter, wenn Sie der Übertragung zustimmen; teilen Sie ihn sonst mit niemandem.",
		"{app} – {org}: Ihr Code zur Übernahme des Fahrzeugs {vehicle}: {code}\nGültig für {minutes} Minuten. Geben Sie ihn nur an den Händler weiter, wenn Sie der Übertragung zustimmen; teilen Sie ihn sonst mit niemandem.",
	},
	"el": {
		"{app} – {org}: ο κωδικός σας για την έγκριση της μεταβίβασης του οχήματός σας {vehicle}: {code}\nΙσχύει για {minutes} λεπτά. Δώστε τον στον αντιπρόσωπο μόνο αν εγκρίνετε τη μεταβίβαση· διαφορετικά μην τον κοινοποιήσετε.",
		"{app} – {org}: ο κωδικός σας για την παραλαβή του οχήματος {vehicle}: {code}\nΙσχύει για {minutes} λεπτά. Δώστε τον στον αντιπρόσωπο μόνο αν εγκρίνετε τη μεταβίβαση· διαφορετικά μην τον κοινοποιήσετε.",
	},
	"uk": {
		"{app} – {org}: ваш код для підтвердження передачі автомобіля {vehicle}: {code}\nДійсний {minutes} хв. Передайте його дилеру, лише якщо погоджуєтеся на передачу; інакше нікому не повідомляйте.",
		"{app} – {org}: ваш код для прийняття автомобіля {vehicle}: {code}\nДійсний {minutes} хв. Передайте його дилеру, лише якщо погоджуєтеся на передачу; інакше нікому не повідомляйте.",
	},
	"ru": {
		"{app} – {org}: ваш код для подтверждения передачи автомобиля {vehicle}: {code}\nДействует {minutes} мин. Сообщите его дилеру, только если согласны на передачу; иначе никому не сообщайте.",
		"{app} – {org}: ваш код для принятия автомобиля {vehicle}: {code}\nДействует {minutes} мин. Сообщите его дилеру, только если согласны на передачу; иначе никому не сообщайте.",
	},
	"fr": {
		"{app} – {org} : votre code pour approuver le transfert de votre véhicule {vehicle} : {code}\nValable {minutes} minutes. Communiquez-le au concessionnaire uniquement si vous approuvez le transfert ; sinon, ne le partagez pas.",
		"{app} – {org} : votre code pour reprendre le véhicule {vehicle} : {code}\nValable {minutes} minutes. Communiquez-le au concessionnaire uniquement si vous approuvez le transfert ; sinon, ne le partagez pas.",
	},
	"es": {
		"{app} – {org}: su código para aprobar la transferencia de su vehículo {vehicle}: {code}\nVálido durante {minutes} minutos. Entréguelo al concesionario solo si aprueba la transferencia; de lo contrario, no lo comparta.",
		"{app} – {org}: su código para recibir el vehículo {vehicle}: {code}\nVálido durante {minutes} minutos. Entréguelo al concesionario solo si aprueba la transferencia; de lo contrario, no lo comparta.",
	},
	"it": {
		"{app} – {org}: il suo codice per approvare il trasferimento del veicolo {vehicle}: {code}\nValido per {minutes} minuti. Lo comunichi al rivenditore solo se approva il trasferimento; altrimenti non lo condivida.",
		"{app} – {org}: il suo codice per rilevare il veicolo {vehicle}: {code}\nValido per {minutes} minuti. Lo comunichi al rivenditore solo se approva il trasferimento; altrimenti non lo condivida.",
	},
	"zh-CN": {
		"{app} – {org}：您的车辆 {vehicle} 过户确认码：{code}\n{minutes} 分钟内有效。仅在您同意过户时将其告知经销商，否则请勿分享。",
		"{app} – {org}：您接收车辆 {vehicle} 的确认码：{code}\n{minutes} 分钟内有效。仅在您同意过户时将其告知经销商，否则请勿分享。",
	},
	"az": {
		"{app} – {org}: {vehicle} nömrəli avtomobilinizin təhvili üçün təsdiq kodunuz: {code}\nKod {minutes} dəqiqə etibarlıdır. Təhvili təsdiqləyirsinizsə kodu dilerə verin; əks halda heç kimlə paylaşmayın.",
		"{app} – {org}: {vehicle} nömrəli avtomobilin sizə təhvili üçün təsdiq kodunuz: {code}\nKod {minutes} dəqiqə etibarlıdır. Təhvili təsdiqləyirsinizsə kodu dilerə verin; əks halda heç kimlə paylaşmayın.",
	},
	"ar": {
		"{app} – {org}: رمز الموافقة على نقل ملكية مركبتك {vehicle}: {code}\nصالح لمدة {minutes} دقيقة. أعطه للوكيل فقط إذا كنت توافق على النقل، وإلا فلا تشاركه مع أحد.",
		"{app} – {org}: رمز استلام ملكية المركبة {vehicle}: {code}\nصالح لمدة {minutes} دقيقة. أعطه للوكيل فقط إذا كنت توافق على النقل، وإلا فلا تشاركه مع أحد.",
	},
}

// RenderTransferMessage builds the code text for side "from" (current
// owner) or "to" (new owner) in the recipient's locale (tr fallback).
func RenderTransferMessage(locale, side, app, org, vehicle, code string, minutes int) string {
	tpl, ok := transferMessages[i18n.Normalize(locale)]
	if !ok {
		tpl = transferMessages[i18n.DefaultLocale]
	}
	body := tpl[0]
	if side == "to" {
		body = tpl[1]
	}
	return strings.NewReplacer("{app}", app, "{org}", org, "{vehicle}", vehicle, "{code}", code,
		"{minutes}", fmt.Sprint(minutes)).Replace(body)
}
