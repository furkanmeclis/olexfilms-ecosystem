package usecase

// TEC-296 (F3-02d, K28): VIN based before/after matching of a service,
// dealer confirmation and manual selection.
//
// The rule itself is Match (match.go). The Linker applies it inside a
// transaction that locks the service row, so the auto rule, a confirmation
// and a manual selection of the same service never interleave. A confirmed
// link is never touched by the auto rule; an auto link waits for the dealer
// (confirmed_at NULL) until confirmed or replaced by a manual choice.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	serviceStatusCancelled = "cancelled"
	serviceStatusCompleted = "completed"
	orgTypeCenter          = "center"
)

var (
	// ErrMeasurementNotExpected: the service answered "no measurement"
	// (has_measurement = false); 422 MEASUREMENT_NOT_EXPECTED.
	ErrMeasurementNotExpected = errors.New("measurements: service expects no measurement")
	// ErrMeasurementVINPending: the measurement has no VIN yet; 422.
	ErrMeasurementVINPending = errors.New("measurements: measurement vin pending")
	// ErrMeasurementVINMismatch: the measurement's VIN is not the service's; 422.
	ErrMeasurementVINMismatch = errors.New("measurements: measurement vin differs from the service")
	// ErrPhaseTaken: the phase already has a confirmed (or manual) link; 409.
	ErrPhaseTaken = errors.New("measurements: phase already linked")
	// ErrMeasurementLinked: the measurement is linked elsewhere; 409.
	ErrMeasurementLinked = errors.New("measurements: measurement already linked")
	// ErrServiceCancelled: a cancelled service takes no link change; 409.
	ErrServiceCancelled = errors.New("measurements: service cancelled")
	// ErrLinkLocked: after completion only the center changes links; 403.
	ErrLinkLocked = errors.New("measurements: link of a completed service is center only")
	// ErrLinkNotFound: the phase has no link; 404.
	ErrLinkNotFound = errors.New("measurements: link not found")
)

// Linker matches, confirms and selects the before/after measurements of a
// service.
type Linker struct {
	pool TxBeginner
	q    *db.Queries
	out  outbox.Enqueuer
	log  *slog.Logger
}

// NewLinker creates the linker; out receives measurement.match_suggested.
func NewLinker(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, log *slog.Logger) *Linker {
	if log == nil {
		log = slog.Default()
	}
	return &Linker{pool: pool, q: q, out: out, log: log}
}

// LinkCaller is the panel caller of the service measurement routes.
type LinkCaller struct {
	UserID int64
	Org    orgctx.Scope
	Filter scopefilter.Filter
}

// LinkInput is POST /v1/services/{uuid}/measurements.
type LinkInput struct {
	MeasurementUUID uuid.UUID
	Phase           string
}

// MeasurementBrief is a measurement in the service measurement view.
type MeasurementBrief struct {
	UUID         uuid.UUID  `json:"uuid"`
	VIN          *string    `json:"vin"`
	Status       string     `json:"status"`
	Source       string     `json:"source"`
	DeviceSerial *string    `json:"device_serial"`
	MeasuredAt   *time.Time `json:"measured_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// UserRef is the confirming user.
type UserRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ServiceMeasurementLink is one linked phase.
type ServiceMeasurementLink struct {
	Phase       string           `json:"phase"`
	LinkSource  string           `json:"link_source"`
	Confirmed   bool             `json:"confirmed"`
	ConfirmedAt *time.Time       `json:"confirmed_at"`
	ConfirmedBy *UserRef         `json:"confirmed_by"`
	LinkedAt    time.Time        `json:"linked_at"`
	Measurement MeasurementBrief `json:"measurement"`
}

// ServiceMeasurementSuggestion is a window candidate of a phase.
type ServiceMeasurementSuggestion struct {
	Phase       string           `json:"phase"`
	Measurement MeasurementBrief `json:"measurement"`
}

// ServiceMeasurementsView is GET /v1/services/{uuid}/measurements.
type ServiceMeasurementsView struct {
	ServiceUUID    uuid.UUID                      `json:"service_uuid"`
	VIN            *string                        `json:"vin"`
	HasMeasurement bool                           `json:"has_measurement"`
	Status         string                         `json:"status"`
	Links          []ServiceMeasurementLink       `json:"links"`
	Suggestions    []ServiceMeasurementSuggestion `json:"suggestions"`
	Candidates     []MeasurementBrief             `json:"candidates"`
}

type matchSvc struct {
	ID             int64
	UUID           uuid.UUID
	OrganizationID int64
	BrandID        int64
	VIN            string
	HasMeasurement bool
	Status         string
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

func fromLockRow(r db.GetServiceForMeasurementMatchRow) matchSvc {
	return matchSvc{ID: r.ID, UUID: r.Uuid, OrganizationID: r.OrganizationID, BrandID: r.BrandID,
		VIN: r.Vin.String, HasMeasurement: r.HasMeasurement, Status: r.Status,
		CreatedAt: r.CreatedAt.Time, CompletedAt: timeOut(r.CompletedAt)}
}

// eligible reports whether the auto rule runs for the service.
func (s matchSvc) eligible() bool {
	return s.HasMeasurement && s.VIN != "" && s.Status != serviceStatusCancelled
}

func candidateAt(measured, created pgtype.Timestamptz) time.Time {
	if measured.Valid {
		return measured.Time
	}
	return created.Time
}

func phaseValid(p string) bool { return p == PhaseBefore || p == PhaseAfter }

func (l *Linker) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("measurements: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(l.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("measurements: commit: %w", err)
	}
	return nil
}

// MatchService runs the auto rule for one service: an empty phase with a
// single window candidate is linked (auto, waiting for confirmation) and
// measurement.match_suggested is written when something was linked or
// suggested. A service without a measurement answer, without VIN or
// cancelled is skipped.
func (l *Linker) MatchService(ctx context.Context, serviceID int64) (MatchPlan, error) {
	var plan MatchPlan
	err := l.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		row, err := q.GetServiceForMeasurementMatch(ctx, serviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		svc := fromLockRow(row)
		if !svc.eligible() {
			return nil
		}
		plan, err = l.autoLink(ctx, q, tx, svc)
		return err
	})
	return plan, err
}

// MatchVIN runs MatchService for the services of the organization a newly
// accepted measurement with the VIN may belong to. Failures are logged only:
// the measurement is stored whatever the matching does.
func (l *Linker) MatchVIN(ctx context.Context, organizationID int64, vin string) error {
	vin = strings.ToUpper(strings.TrimSpace(vin))
	if vin == "" {
		return nil
	}
	ids, err := l.q.ListServicesForMeasurementMatch(ctx, db.ListServicesForMeasurementMatchParams{
		OrganizationID: organizationID, Vin: pgtype.Text{String: vin, Valid: true},
	})
	if err != nil {
		l.log.Warn("measurement_match_failed", "organization_id", organizationID, "error", err)
		return err
	}
	var first error
	for _, id := range ids {
		if _, err := l.MatchService(ctx, id); err != nil {
			l.log.Warn("measurement_match_failed", "service_id", id, "error", err)
			if first == nil {
				first = err
			}
			continue
		}
		if err := l.RecalculateServiceDiff(ctx, id); err != nil {
			l.log.Warn("measurement_diff_failed", "service_id", id, "error", err)
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// HandleServiceEvent is the bus consumer of the service events: a service
// whose VIN was set or that moved to processing / ready / completed gets
// its suggestions computed. Idempotent.
func (l *Linker) HandleServiceEvent(ctx context.Context, ev events.Event) error {
	if ev.EntityID == nil || *ev.EntityID <= 0 {
		return nil
	}
	if _, err := l.MatchService(ctx, *ev.EntityID); err != nil {
		return fmt.Errorf("measurements: match service %d: %w", *ev.EntityID, err)
	}
	if err := l.RecalculateServiceDiff(ctx, *ev.EntityID); err != nil {
		return fmt.Errorf("measurements: diff service %d: %w", *ev.EntityID, err)
	}
	return nil
}

func (l *Linker) autoLink(ctx context.Context, q *db.Queries, tx pgx.Tx, svc matchSvc) (MatchPlan, error) {
	links, err := q.ListServiceMeasurements(ctx, db.ListServiceMeasurementsParams{
		ServiceID: svc.ID, OrganizationID: svc.OrganizationID,
	})
	if err != nil {
		return MatchPlan{}, err
	}
	cands, err := q.ListMeasurementMatchCandidates(ctx, db.ListMeasurementMatchCandidatesParams{
		OrganizationID: svc.OrganizationID, Vin: pgtype.Text{String: svc.VIN, Valid: true},
	})
	if err != nil {
		return MatchPlan{}, err
	}
	ms := MatchService{Status: svc.Status, StartedAt: svc.CreatedAt, CompletedAt: svc.CompletedAt, Linked: map[string]bool{}}
	confirmed := map[string]bool{}
	for _, lk := range links {
		ms.Linked[lk.Phase] = true
		confirmed[lk.Phase] = lk.ConfirmedAt.Valid
	}
	in := make([]MatchCandidate, 0, len(cands))
	uuids := map[int64]uuid.UUID{}
	for _, c := range cands {
		in = append(in, MatchCandidate{ID: c.ID, At: candidateAt(c.MeasuredAt, c.CreatedAt)})
		uuids[c.ID] = c.Uuid
	}
	plan := Match(ms, in)

	auto := []map[string]any{}
	for _, phase := range []string{PhaseBefore, PhaseAfter} {
		c, ok := plan.Auto[phase]
		if !ok {
			continue
		}
		linked, err := linkAuto(ctx, q, tx, svc, c.ID, phase)
		if err != nil {
			return MatchPlan{}, err
		}
		if !linked {
			delete(plan.Auto, phase)
			continue
		}
		auto = append(auto, map[string]any{"phase": phase, "measurement_uuid": uuids[c.ID].String()})
	}
	if len(auto) > 0 {
		if err := l.recalculateDiffLocked(ctx, q, tx, svc); err != nil {
			return MatchPlan{}, err
		}
	}

	suggested := map[string]any{}
	for _, phase := range []string{PhaseBefore, PhaseAfter} {
		if confirmed[phase] || len(plan.Suggestions[phase]) == 0 {
			continue
		}
		if _, ok := plan.Auto[phase]; ok {
			continue
		}
		list := make([]string, 0, len(plan.Suggestions[phase]))
		for _, c := range plan.Suggestions[phase] {
			list = append(list, uuids[c.ID].String())
		}
		suggested[phase] = list
	}
	if len(auto) == 0 && len(suggested) == 0 {
		return plan, nil
	}
	id, uid := svc.ID, svc.UUID
	ev := events.New(events.MeasurementMatchSuggested).WithTenant(svc.OrganizationID).
		WithEntity("service", &id, &uid).WithPayload(map[string]any{
		"service_uuid":    svc.UUID.String(),
		"organization_id": svc.OrganizationID,
		"brand_id":        svc.BrandID,
		"vin":             svc.VIN,
		"auto_linked":     auto,
		"suggestions":     suggested,
	})
	if err := l.out.Enqueue(ctx, tx, ev); err != nil {
		return MatchPlan{}, fmt.Errorf("measurements: outbox: %w", err)
	}
	return plan, nil
}

// linkAuto inserts an auto link under a savepoint: a measurement another
// service claimed meanwhile (23505) is skipped instead of failing the run.
func linkAuto(ctx context.Context, q *db.Queries, tx pgx.Tx, svc matchSvc, resultID int64, phase string) (bool, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	_, err = q.WithTx(sp).LinkServiceMeasurement(ctx, db.LinkServiceMeasurementParams{
		OrganizationID: svc.OrganizationID, BrandID: svc.BrandID, ServiceID: svc.ID,
		MeasurementResultID: resultID, Phase: phase, LinkSource: LinkSourceAuto,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		_ = sp.Rollback(ctx)
		return false, nil
	}
	if err != nil {
		_ = sp.Rollback(ctx)
		return false, err
	}
	return true, sp.Commit(ctx)
}

// visibleService loads the service of the caller's brand and reach.
func (l *Linker) visibleService(ctx context.Context, q *db.Queries, c LinkCaller, id uuid.UUID) (matchSvc, error) {
	row, err := q.GetServiceForMeasurementMatchByUUID(ctx, db.GetServiceForMeasurementMatchByUUIDParams{
		Uuid: id, BrandID: c.Org.BrandID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return matchSvc{}, ErrServiceNotFound
	}
	if err != nil {
		return matchSvc{}, err
	}
	if !c.Filter.AllowsOrg(row.OrganizationID, row.BrandID) {
		return matchSvc{}, ErrServiceNotFound
	}
	return matchSvc{ID: row.ID, UUID: row.Uuid, OrganizationID: row.OrganizationID, BrandID: row.BrandID,
		VIN: row.Vin.String, HasMeasurement: row.HasMeasurement, Status: row.Status,
		CreatedAt: row.CreatedAt.Time, CompletedAt: timeOut(row.CompletedAt)}, nil
}

// lockService loads the visible service and locks its row in the tx.
func (l *Linker) lockService(ctx context.Context, q *db.Queries, c LinkCaller, id uuid.UUID) (matchSvc, error) {
	svc, err := l.visibleService(ctx, q, c, id)
	if err != nil {
		return matchSvc{}, err
	}
	row, err := q.GetServiceForMeasurementMatch(ctx, svc.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return matchSvc{}, ErrServiceNotFound
	}
	if err != nil {
		return matchSvc{}, err
	}
	return fromLockRow(row), nil
}

// ServiceMeasurements is GET /v1/services/{uuid}/measurements: the links,
// the window suggestions of the phases without a confirmed link and the
// other unlinked accepted measurements of the same VIN.
func (l *Linker) ServiceMeasurements(ctx context.Context, c LinkCaller, id uuid.UUID) (ServiceMeasurementsView, error) {
	svc, err := l.visibleService(ctx, l.q, c, id)
	if err != nil {
		return ServiceMeasurementsView{}, err
	}
	return l.view(ctx, l.q, svc)
}

func (l *Linker) view(ctx context.Context, q *db.Queries, svc matchSvc) (ServiceMeasurementsView, error) {
	out := ServiceMeasurementsView{
		ServiceUUID: svc.UUID, HasMeasurement: svc.HasMeasurement, Status: svc.Status,
		Links: []ServiceMeasurementLink{}, Suggestions: []ServiceMeasurementSuggestion{}, Candidates: []MeasurementBrief{},
	}
	if svc.VIN != "" {
		v := svc.VIN
		out.VIN = &v
	}
	links, err := q.ListServiceMeasurementLinks(ctx, db.ListServiceMeasurementLinksParams{
		ServiceID: svc.ID, OrganizationID: svc.OrganizationID,
	})
	if err != nil {
		return out, err
	}
	ms := MatchService{Status: svc.Status, StartedAt: svc.CreatedAt, CompletedAt: svc.CompletedAt, Linked: map[string]bool{}}
	confirmed := map[string]bool{}
	for _, lk := range links {
		ms.Linked[lk.Phase] = true
		confirmed[lk.Phase] = lk.ConfirmedAt.Valid
		link := ServiceMeasurementLink{
			Phase: lk.Phase, LinkSource: lk.LinkSource, Confirmed: lk.ConfirmedAt.Valid,
			ConfirmedAt: timeOut(lk.ConfirmedAt), LinkedAt: lk.LinkedAt.Time,
			Measurement: MeasurementBrief{
				UUID: lk.MeasurementUuid, VIN: textOut(lk.Vin), Status: lk.Status, Source: lk.Source,
				DeviceSerial: textOut(lk.DeviceSerial), MeasuredAt: timeOut(lk.MeasuredAt),
				CreatedAt: lk.MeasurementCreatedAt.Time,
			},
		}
		if lk.ConfirmedByUuid.Valid {
			link.ConfirmedBy = &UserRef{
				UUID: uuid.UUID(lk.ConfirmedByUuid.Bytes),
				Name: strings.TrimSpace(lk.ConfirmedByName.String + " " + lk.ConfirmedBySurname.String),
			}
		}
		out.Links = append(out.Links, link)
	}
	if svc.VIN == "" {
		return out, nil
	}
	cands, err := q.ListMeasurementMatchCandidates(ctx, db.ListMeasurementMatchCandidatesParams{
		OrganizationID: svc.OrganizationID, Vin: pgtype.Text{String: svc.VIN, Valid: true},
	})
	if err != nil {
		return out, err
	}
	briefs := map[int64]MeasurementBrief{}
	in := make([]MatchCandidate, 0, len(cands))
	for _, c := range cands {
		briefs[c.ID] = MeasurementBrief{
			UUID: c.Uuid, VIN: textOut(c.Vin), Status: c.Status, Source: c.Source,
			DeviceSerial: textOut(c.DeviceSerial), MeasuredAt: timeOut(c.MeasuredAt), CreatedAt: c.CreatedAt.Time,
		}
		in = append(in, MatchCandidate{ID: c.ID, At: candidateAt(c.MeasuredAt, c.CreatedAt)})
	}
	plan := Match(ms, in)
	suggested := map[int64]bool{}
	if svc.Status != serviceStatusCancelled {
		for _, phase := range []string{PhaseBefore, PhaseAfter} {
			if confirmed[phase] {
				continue
			}
			for _, c := range plan.Suggestions[phase] {
				suggested[c.ID] = true
				out.Suggestions = append(out.Suggestions, ServiceMeasurementSuggestion{Phase: phase, Measurement: briefs[c.ID]})
			}
		}
	}
	for _, c := range cands {
		if !suggested[c.ID] {
			out.Candidates = append(out.Candidates, briefs[c.ID])
		}
	}
	return out, nil
}

// LinkMeasurement is POST /v1/services/{uuid}/measurements: the linked
// measurement of the phase is confirmed; an unlinked measurement is linked
// manually (confirmed by the caller) into an empty phase or in place of an
// unconfirmed auto link.
func (l *Linker) LinkMeasurement(ctx context.Context, c LinkCaller, id uuid.UUID, in LinkInput) (ServiceMeasurementsView, error) {
	if !phaseValid(in.Phase) {
		return ServiceMeasurementsView{}, invalid("phase", "must be before or after")
	}
	if in.MeasurementUUID == uuid.Nil {
		return ServiceMeasurementsView{}, invalid("measurement_uuid", "is required")
	}
	var out ServiceMeasurementsView
	err := l.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := l.lockService(ctx, q, c, id)
		if err != nil {
			return err
		}
		if !svc.HasMeasurement {
			return ErrMeasurementNotExpected
		}
		if svc.Status == serviceStatusCancelled {
			return ErrServiceCancelled
		}
		m, err := q.GetMeasurementResultForLink(ctx, db.GetMeasurementResultForLinkParams{
			Uuid: in.MeasurementUUID, OrganizationID: svc.OrganizationID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if m.Status != StatusAccepted || !m.Vin.Valid {
			return ErrMeasurementVINPending
		}
		if m.Vin.String != svc.VIN {
			return ErrMeasurementVINMismatch
		}
		if err := l.applyLink(ctx, q, c, svc, m.ID, in.Phase); err != nil {
			return err
		}
		if err := l.recalculateDiffLocked(ctx, q, tx, svc); err != nil {
			return err
		}
		out, err = l.view(ctx, q, svc)
		return err
	})
	return out, err
}

func (l *Linker) applyLink(ctx context.Context, q *db.Queries, c LinkCaller, svc matchSvc, resultID int64, phase string) error {
	confirmer := pgtype.Int8{Int64: c.UserID, Valid: c.UserID != 0}
	if own, err := q.GetServiceMeasurementByResult(ctx, db.GetServiceMeasurementByResultParams{
		MeasurementResultID: resultID, OrganizationID: svc.OrganizationID,
	}); err == nil {
		if own.ServiceID != svc.ID || own.Phase != phase {
			return ErrMeasurementLinked
		}
		if own.ConfirmedAt.Valid {
			return nil // already confirmed: idempotent
		}
		_, err := q.ConfirmServiceMeasurement(ctx, db.ConfirmServiceMeasurementParams{
			ConfirmedBy: confirmer, ServiceID: svc.ID, Phase: phase, OrganizationID: svc.OrganizationID,
		})
		return err
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	links, err := q.ListServiceMeasurements(ctx, db.ListServiceMeasurementsParams{
		ServiceID: svc.ID, OrganizationID: svc.OrganizationID,
	})
	if err != nil {
		return err
	}
	for _, lk := range links {
		if lk.Phase != phase {
			continue
		}
		if lk.ConfirmedAt.Valid {
			return ErrPhaseTaken
		}
		if !l.mayChangeLocked(c, svc) {
			return ErrLinkLocked
		}
		_, err := q.ReplaceServiceMeasurement(ctx, db.ReplaceServiceMeasurementParams{
			MeasurementResultID: resultID, ConfirmedBy: confirmer, ServiceID: svc.ID,
			Phase: phase, OrganizationID: svc.OrganizationID,
		})
		return mapLinkDBError(err)
	}
	_, err = q.LinkServiceMeasurement(ctx, db.LinkServiceMeasurementParams{
		OrganizationID: svc.OrganizationID, BrandID: svc.BrandID, ServiceID: svc.ID,
		MeasurementResultID: resultID, Phase: phase, LinkSource: LinkSourceManual, ConfirmedBy: confirmer,
	})
	return mapLinkDBError(err)
}

// mayChangeLocked: an existing link of a completed service is removed or
// replaced by the center only.
func (l *Linker) mayChangeLocked(c LinkCaller, svc matchSvc) bool {
	return svc.Status != serviceStatusCompleted || c.Org.OrgType == orgTypeCenter
}

func mapLinkDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "uq_service_measurements_service_phase" {
			return ErrPhaseTaken
		}
		return ErrMeasurementLinked
	}
	return err
}

// UnlinkMeasurement is DELETE /v1/services/{uuid}/measurements/{phase}.
// After completion only the center removes a link.
func (l *Linker) UnlinkMeasurement(ctx context.Context, c LinkCaller, id uuid.UUID, phase string) error {
	if !phaseValid(phase) {
		return invalid("phase", "must be before or after")
	}
	return l.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := l.lockService(ctx, q, c, id)
		if err != nil {
			return err
		}
		if svc.Status == serviceStatusCancelled {
			return ErrServiceCancelled
		}
		if !l.mayChangeLocked(c, svc) {
			return ErrLinkLocked
		}
		n, err := q.UnlinkServiceMeasurement(ctx, db.UnlinkServiceMeasurementParams{
			ServiceID: svc.ID, Phase: phase, OrganizationID: svc.OrganizationID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrLinkNotFound
		}
		if err := l.recalculateDiffLocked(ctx, q, tx, svc); err != nil {
			return err
		}
		return nil
	})
}
