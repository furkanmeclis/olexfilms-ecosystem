package features

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrUnknownModule: the key is not in the catalog.
	ErrUnknownModule = errors.New("features: unknown module")
	// ErrCoreModule: core modules cannot be switched.
	ErrCoreModule = errors.New("features: core modules cannot be switched off")
	// ErrUpstreamDisabled: the module is closed one level up, so it cannot be
	// switched on here.
	ErrUpstreamDisabled = errors.New("features: module is disabled upstream")
	// ErrAdminOverride: the platform admin set the dealer's value directly;
	// the distributor cannot change it.
	ErrAdminOverride = errors.New("features: module value is set by the platform admin")
	// ErrNotDistributor: dealer settings need a distributor organization.
	ErrNotDistributor = errors.New("features: active organization is not a distributor")
	// ErrNotOwnDealer: a target is not a dealer of the distributor.
	ErrNotOwnDealer = errors.New("features: organization is not a dealer of this distributor")
	// ErrOrganizationNotFound: the organization does not exist.
	ErrOrganizationNotFound = errors.New("features: organization not found")
)

// Service resolves and changes module flags.
type Service struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	cache Cache
	log   *slog.Logger
}

// New creates the service. cache may be nil (no caching).
func New(pool *pgxpool.Pool, q *db.Queries, cache Cache, log *slog.Logger) *Service {
	if cache == nil {
		cache = NoCache{}
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{pool: pool, q: q, cache: cache, log: log}
}

// SyncCatalog upserts the Go catalog into the modules table. Level and sort
// order follow the catalog; admin-edited defaults survive.
func (s *Service) SyncCatalog(ctx context.Context) error {
	for _, m := range Modules {
		if err := s.q.UpsertModuleCatalog(ctx, db.UpsertModuleCatalogParams{
			Key: m.Key, Level: string(m.Level), DefaultEnabled: m.DefaultEnabled,
			IsPaid: m.Paid, SortOrder: m.SortOrder(),
		}); err != nil {
			return fmt.Errorf("features: sync %s: %w", m.Key, err)
		}
	}
	s.cache.BumpGeneration(ctx)
	return nil
}

// Enabled reports whether key is on for the organization. It implements
// middleware.FeatureChecker. Unknown keys read as off (and are logged).
func (s *Service) Enabled(ctx context.Context, organizationID int64, key string) (bool, error) {
	def, ok := ModuleByKey(key)
	if !ok {
		s.log.Warn("feature_unknown_key", "key", key, "organization_id", organizationID)
		return false, nil
	}
	if def.Level == LevelCore {
		return true, nil
	}
	states, err := s.Snapshot(ctx, organizationID)
	if err != nil {
		return false, err
	}
	st, ok := Lookup(states, key)
	if !ok {
		// In the Go catalog but not in the table yet (SyncCatalog not run).
		s.log.Warn("feature_key_not_seeded", "key", key)
		return false, nil
	}
	return st.Enabled, nil
}

// Snapshot returns the effective module states of an organization, cached
// for CacheTTL.
func (s *Service) Snapshot(ctx context.Context, organizationID int64) ([]State, error) {
	if st, ok := s.cache.Get(ctx, organizationID); ok {
		return st, nil
	}
	st, _, err := s.resolve(ctx, s.q, organizationID)
	if err != nil {
		return nil, err
	}
	s.cache.Set(ctx, organizationID, st)
	return st, nil
}

func modulesFromRows(rows []db.Module) []ModuleRow {
	out := make([]ModuleRow, 0, len(rows))
	for _, m := range rows {
		out = append(out, ModuleRow{Key: m.Key, Level: Level(m.Level), DefaultEnabled: m.DefaultEnabled, Paid: m.IsPaid})
	}
	return out
}

func flagsFromRows(rows []db.ListModuleFlagsForOrgsRow) []FlagRow {
	out := make([]FlagRow, 0, len(rows))
	for _, f := range rows {
		fr := FlagRow{Scope: f.Scope, OrgID: f.OrganizationID.Int64, Key: f.ModuleKey, Enabled: f.Enabled, Source: f.Source}
		if f.UpdatedAt.Valid {
			fr.UpdatedAt = f.UpdatedAt.Time
		}
		if f.SetByUuid.Valid {
			fr.SetBy = &Actor{UUID: uuid.UUID(f.SetByUuid.Bytes), Name: f.SetByName}
		}
		out = append(out, fr)
	}
	return out
}

func (s *Service) node(ctx context.Context, q *db.Queries, orgID int64) (OrgNode, error) {
	org, err := q.GetOrganizationByID(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgNode{}, ErrOrganizationNotFound
	}
	if err != nil {
		return OrgNode{}, err
	}
	n := OrgNode{ID: org.ID, Type: org.Type}
	if org.ParentID.Valid {
		parent, err := q.GetOrganizationByID(ctx, org.ParentID.Int64)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return OrgNode{}, err
		}
		if err == nil {
			n.ParentID, n.ParentType = parent.ID, parent.Type
		}
	}
	return n, nil
}

func (s *Service) resolve(ctx context.Context, q *db.Queries, orgID int64) ([]State, OrgNode, error) {
	n, err := s.node(ctx, q, orgID)
	if err != nil {
		return nil, OrgNode{}, err
	}
	mods, err := q.ListModules(ctx)
	if err != nil {
		return nil, OrgNode{}, err
	}
	ids := []int64{n.ID}
	if n.ParentID != 0 {
		ids = append(ids, n.ParentID)
	}
	flags, err := q.ListModuleFlagsForOrgs(ctx, ids)
	if err != nil {
		return nil, OrgNode{}, err
	}
	return Resolve(Input{Modules: modulesFromRows(mods), Flags: flagsFromRows(flags), Org: n}), n, nil
}

// switchable returns the catalog entry of a module that may be switched.
func switchable(key string) (ModuleDef, error) {
	def, ok := ModuleByKey(key)
	if !ok {
		return ModuleDef{}, ErrUnknownModule
	}
	if def.Level == LevelCore {
		return ModuleDef{}, ErrCoreModule
	}
	return def, nil
}

func actorArg(actorID int64) pgtype.Int8 {
	if actorID == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: actorID, Valid: true}
}

func (s *Service) systemClosed(ctx context.Context, q *db.Queries, key string) (bool, error) {
	flags, err := q.ListModuleFlagsForOrgs(ctx, []int64{})
	if err != nil {
		return false, err
	}
	for _, f := range flags {
		if f.Scope == ScopeSystem && f.ModuleKey == key && !f.Enabled {
			return true, nil
		}
	}
	return false, nil
}

// invalidateTree drops the snapshots of orgID and every organization below.
func (s *Service) invalidateTree(ctx context.Context, orgID int64) {
	ids := []int64{orgID}
	below, err := s.q.Descendants(ctx, orgID)
	if err != nil {
		s.log.Warn("feature_cache_tree_failed", "organization_id", orgID, "error", err)
		s.cache.BumpGeneration(ctx)
		return
	}
	for _, o := range below {
		ids = append(ids, o.ID)
	}
	s.cache.Invalidate(ctx, ids...)
}

// --- Platform (admin) -------------------------------------------------------

// PlatformModule is one catalog row as the platform admin sees it.
type PlatformModule struct {
	Key            string `json:"key"`
	Level          Level  `json:"level"`
	Enabled        bool   `json:"enabled"`
	DefaultEnabled bool   `json:"default_enabled"`
	Paid           bool   `json:"paid"`
	SetBy          *Actor `json:"set_by,omitempty"`
}

// PlatformModules lists the catalog with the system switch of each module.
func (s *Service) PlatformModules(ctx context.Context) ([]PlatformModule, error) {
	mods, err := s.q.ListModules(ctx)
	if err != nil {
		return nil, err
	}
	flags, err := s.q.ListModuleFlagsForOrgs(ctx, []int64{})
	if err != nil {
		return nil, err
	}
	sys := map[string]FlagRow{}
	for _, f := range flagsFromRows(flags) {
		if f.Scope == ScopeSystem {
			sys[f.Key] = f
		}
	}
	out := make([]PlatformModule, 0, len(mods))
	for _, m := range mods {
		pm := PlatformModule{Key: m.Key, Level: Level(m.Level), Enabled: true, DefaultEnabled: m.DefaultEnabled, Paid: m.IsPaid}
		if f, ok := sys[m.Key]; ok && m.Level != string(LevelCore) {
			pm.Enabled, pm.SetBy = f.Enabled, f.SetBy
		}
		out = append(out, pm)
	}
	return out, nil
}

// PlatformModuleInput changes a module system wide. Nil fields stay.
type PlatformModuleInput struct {
	// Enabled false closes the module everywhere; true reopens it.
	Enabled        *bool
	DefaultEnabled *bool
	Paid           *bool
}

// UpdatePlatformModule applies an admin change and drops every snapshot.
func (s *Service) UpdatePlatformModule(ctx context.Context, actorID int64, key string, in PlatformModuleInput) (PlatformModule, error) {
	def, ok := ModuleByKey(key)
	if !ok {
		return PlatformModule{}, ErrUnknownModule
	}
	if def.Level == LevelCore && ((in.Enabled != nil && !*in.Enabled) || (in.DefaultEnabled != nil && !*in.DefaultEnabled)) {
		return PlatformModule{}, ErrCoreModule
	}
	err := s.inTx(ctx, func(q *db.Queries) error {
		if in.DefaultEnabled != nil || in.Paid != nil {
			args := db.UpdateModuleDefaultsParams{Key: key}
			if in.DefaultEnabled != nil {
				args.DefaultEnabled = pgtype.Bool{Bool: *in.DefaultEnabled, Valid: true}
			}
			if in.Paid != nil {
				args.IsPaid = pgtype.Bool{Bool: *in.Paid, Valid: true}
			}
			if _, err := q.UpdateModuleDefaults(ctx, args); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrUnknownModule
				}
				return err
			}
		}
		if in.Enabled != nil && def.Level != LevelCore {
			if *in.Enabled {
				// Open again: no system row means "available".
				if _, err := q.DeleteSystemModuleFlag(ctx, key); err != nil {
					return err
				}
			} else if _, err := q.UpsertSystemModuleFlag(ctx, db.UpsertSystemModuleFlagParams{
				ModuleKey: key, Enabled: false, SetByUserID: actorArg(actorID),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return PlatformModule{}, err
	}
	s.cache.BumpGeneration(ctx)
	all, err := s.PlatformModules(ctx)
	if err != nil {
		return PlatformModule{}, err
	}
	for _, m := range all {
		if m.Key == key {
			return m, nil
		}
	}
	return PlatformModule{}, ErrUnknownModule
}

// SetByAdmin sets an organization's value as the platform admin
// (source=admin). It works below a closed distributor, never below a closed
// system switch.
func (s *Service) SetByAdmin(ctx context.Context, actorID, orgID int64, key string, enabled bool) (State, error) {
	if _, err := switchable(key); err != nil {
		return State{}, err
	}
	if enabled {
		closed, err := s.systemClosed(ctx, s.q, key)
		if err != nil {
			return State{}, err
		}
		if closed {
			return State{}, ErrUpstreamDisabled
		}
	}
	if _, err := s.node(ctx, s.q, orgID); err != nil {
		return State{}, err
	}
	if _, err := s.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
		Scope: ScopeOrg, OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ModuleKey: key,
		Enabled: enabled, Source: SourceAdmin, SetByUserID: actorArg(actorID),
	}); err != nil {
		return State{}, err
	}
	s.invalidateTree(ctx, orgID)
	return s.stateOf(ctx, orgID, key)
}

// ClearByAdmin removes an organization's own value (back to inheritance).
func (s *Service) ClearByAdmin(ctx context.Context, orgID int64, key string) (State, error) {
	if _, err := switchable(key); err != nil {
		return State{}, err
	}
	if _, err := s.node(ctx, s.q, orgID); err != nil {
		return State{}, err
	}
	if _, err := s.q.DeleteOrgModuleFlag(ctx, db.DeleteOrgModuleFlagParams{
		Scope: ScopeOrg, OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ModuleKey: key,
	}); err != nil {
		return State{}, err
	}
	s.invalidateTree(ctx, orgID)
	return s.stateOf(ctx, orgID, key)
}

// SetByService sets an organization's value from a paid service subscription
// (source=service). Unlike admin overrides it obeys the parent/system chain.
// q is the caller's transaction so the flag commits or rolls back together
// with the subscription; the caller invalidates the cache (InvalidateOrg)
// after commit.
func (s *Service) SetByService(ctx context.Context, q *db.Queries, actorID, orgID, serviceID int64, key string) error {
	if _, err := switchable(key); err != nil {
		return err
	}
	if q == nil {
		q = s.q
	}
	st, _, err := s.resolve(ctx, q, orgID)
	if err != nil {
		return err
	}
	cur, ok := Lookup(st, key)
	if !ok {
		return ErrUnknownModule
	}
	if !cur.UpstreamEnabled {
		return ErrUpstreamDisabled
	}
	_, err = q.UpsertServiceModuleFlag(ctx, db.UpsertServiceModuleFlagParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
		ModuleKey:      key,
		Enabled:        true,
		SetByUserID:    actorArg(actorID),
		ServiceID:      pgtype.Int8{Int64: serviceID, Valid: true},
		Note:           pgtype.Text{String: "service_subscription", Valid: true},
	})
	return err
}

// ClearByService removes a service-owned flag inside the caller's
// transaction; callers decide whether another active subscription still
// keeps the module open and invalidate the cache after commit.
func (s *Service) ClearByService(ctx context.Context, q *db.Queries, orgID int64, key string) error {
	if _, err := switchable(key); err != nil {
		return err
	}
	if q == nil {
		q = s.q
	}
	_, err := q.DeleteServiceModuleFlag(ctx, db.DeleteServiceModuleFlagParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
		ModuleKey:      key,
	})
	return err
}

// InvalidateOrg drops the cached snapshots of orgID and its subtree. Callers
// that change flags inside their own transaction call it after commit.
func (s *Service) InvalidateOrg(ctx context.Context, orgID int64) {
	s.invalidateTree(ctx, orgID)
}

// OrgStates resolves an organization without the cache (platform detail).
func (s *Service) OrgStates(ctx context.Context, orgID int64) ([]State, error) {
	st, _, err := s.resolve(ctx, s.q, orgID)
	return st, err
}

func (s *Service) stateOf(ctx context.Context, orgID int64, key string) (State, error) {
	st, _, err := s.resolve(ctx, s.q, orgID)
	if err != nil {
		return State{}, err
	}
	out, ok := Lookup(st, key)
	if !ok {
		return State{}, ErrUnknownModule
	}
	return out, nil
}

// --- Distributor ------------------------------------------------------------

func (s *Service) distributor(ctx context.Context, q *db.Queries, distributorID int64) ([]State, error) {
	st, n, err := s.resolve(ctx, q, distributorID)
	if err != nil {
		return nil, err
	}
	if n.Type != OrgDistributor {
		return nil, ErrNotDistributor
	}
	return st, nil
}

// checkDealers verifies that every id is a direct dealer of the distributor.
func checkDealers(ctx context.Context, q *db.Queries, distributorID int64, dealerIDs []int64) error {
	if len(dealerIDs) == 0 {
		return fmt.Errorf("%w: no dealers", ErrNotOwnDealer)
	}
	for _, id := range dealerIDs {
		o, err := q.GetOrganizationByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotOwnDealer
		}
		if err != nil {
			return err
		}
		if o.Type != OrgDealer || !o.ParentID.Valid || o.ParentID.Int64 != distributorID {
			return ErrNotOwnDealer
		}
	}
	return nil
}

// SetForDealers switches a module for one or more dealers of a distributor
// (source=distributor), all or nothing. Switching on needs the module on for
// the distributor itself; dealers whose value the admin set are refused.
func (s *Service) SetForDealers(ctx context.Context, actorID, distributorID int64, dealerIDs []int64, key string, enabled bool) error {
	if _, err := switchable(key); err != nil {
		return err
	}
	err := s.inTx(ctx, func(q *db.Queries) error {
		dist, err := s.distributor(ctx, q, distributorID)
		if err != nil {
			return err
		}
		if err := checkDealers(ctx, q, distributorID, dealerIDs); err != nil {
			return err
		}
		if st, _ := Lookup(dist, key); enabled && !st.Enabled {
			return ErrUpstreamDisabled
		}
		for _, id := range dealerIDs {
			if err := refuseAdminOverride(ctx, q, id, key); err != nil {
				return err
			}
			if _, err := q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
				Scope: ScopeOrg, OrganizationID: pgtype.Int8{Int64: id, Valid: true}, ModuleKey: key,
				Enabled: enabled, Source: SourceDistributor, SetByUserID: actorArg(actorID),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.cache.Invalidate(ctx, dealerIDs...)
	return nil
}

// ClearForDealers removes the distributor's value of dealers (back to the
// dealer standard).
func (s *Service) ClearForDealers(ctx context.Context, distributorID int64, dealerIDs []int64, key string) error {
	if _, err := switchable(key); err != nil {
		return err
	}
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := s.distributor(ctx, q, distributorID); err != nil {
			return err
		}
		if err := checkDealers(ctx, q, distributorID, dealerIDs); err != nil {
			return err
		}
		for _, id := range dealerIDs {
			if err := refuseAdminOverride(ctx, q, id, key); err != nil {
				return err
			}
			if _, err := q.DeleteOrgModuleFlag(ctx, db.DeleteOrgModuleFlagParams{
				Scope: ScopeOrg, OrganizationID: pgtype.Int8{Int64: id, Valid: true}, ModuleKey: key,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.cache.Invalidate(ctx, dealerIDs...)
	return nil
}

func refuseAdminOverride(ctx context.Context, q *db.Queries, orgID int64, key string) error {
	f, err := q.GetOrgModuleFlag(ctx, db.GetOrgModuleFlagParams{
		Scope: ScopeOrg, OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, ModuleKey: key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if f.Source == SourceAdmin {
		return ErrAdminOverride
	}
	return nil
}

// SetDealerStandard sets the distributor's standard for its dealers; every
// dealer without its own value follows it at once (and new dealers too).
func (s *Service) SetDealerStandard(ctx context.Context, actorID, distributorID int64, key string, enabled bool) error {
	if _, err := switchable(key); err != nil {
		return err
	}
	dist, err := s.distributor(ctx, s.q, distributorID)
	if err != nil {
		return err
	}
	if st, _ := Lookup(dist, key); enabled && !st.Enabled {
		return ErrUpstreamDisabled
	}
	if _, err := s.q.UpsertOrgModuleFlag(ctx, db.UpsertOrgModuleFlagParams{
		Scope: ScopeDealerStandard, OrganizationID: pgtype.Int8{Int64: distributorID, Valid: true}, ModuleKey: key,
		Enabled: enabled, Source: SourceDistributor, SetByUserID: actorArg(actorID),
	}); err != nil {
		return err
	}
	s.invalidateTree(ctx, distributorID)
	return nil
}

// ClearDealerStandard drops the standard value (dealers follow the default).
func (s *Service) ClearDealerStandard(ctx context.Context, distributorID int64, key string) error {
	if _, err := switchable(key); err != nil {
		return err
	}
	if _, err := s.distributor(ctx, s.q, distributorID); err != nil {
		return err
	}
	if _, err := s.q.DeleteOrgModuleFlag(ctx, db.DeleteOrgModuleFlagParams{
		Scope: ScopeDealerStandard, OrganizationID: pgtype.Int8{Int64: distributorID, Valid: true}, ModuleKey: key,
	}); err != nil {
		return err
	}
	s.invalidateTree(ctx, distributorID)
	return nil
}

// StandardEntry is one module of a distributor's dealer standard.
type StandardEntry struct {
	Key   string `json:"key"`
	Level Level  `json:"level"`
	Paid  bool   `json:"paid"`
	// DistributorEnabled is the module's value for the distributor itself;
	// the standard cannot switch on what the distributor does not have.
	DistributorEnabled bool `json:"distributor_enabled"`
	// Enabled is what a dealer without its own value gets.
	Enabled bool `json:"enabled"`
	// Explicit is true when the distributor set the standard (false: the
	// module default applies).
	Explicit bool   `json:"explicit"`
	SetBy    *Actor `json:"set_by,omitempty"`
}

// DealerStandard returns the distributor's standard for every switchable
// module that is open system wide.
func (s *Service) DealerStandard(ctx context.Context, distributorID int64) ([]StandardEntry, error) {
	dist, err := s.distributor(ctx, s.q, distributorID)
	if err != nil {
		return nil, err
	}
	flags, err := s.q.ListModuleFlagsForOrgs(ctx, []int64{distributorID})
	if err != nil {
		return nil, err
	}
	std := map[string]FlagRow{}
	for _, f := range flagsFromRows(flags) {
		if f.Scope == ScopeDealerStandard && f.OrgID == distributorID {
			std[f.Key] = f
		}
	}
	out := make([]StandardEntry, 0, len(dist))
	for _, st := range dist {
		if st.Level == LevelCore || st.Source == FromSystem {
			continue
		}
		e := StandardEntry{Key: st.Key, Level: st.Level, Paid: st.Paid, DistributorEnabled: st.Enabled, Enabled: st.DefaultEnabled}
		if f, ok := std[st.Key]; ok {
			e.Enabled, e.Explicit, e.SetBy = f.Enabled, true, f.SetBy
		}
		if !st.Enabled {
			e.Enabled = false
		}
		out = append(out, e)
	}
	return out, nil
}

// DealerModules is the module states of one dealer.
type DealerModules struct {
	OrgID   int64
	UUID    uuid.UUID
	Name    string
	Slug    string
	Modules []State
}

// DealerMatrix resolves every dealer of a distributor in one pass.
func (s *Service) DealerMatrix(ctx context.Context, distributorID int64) ([]DealerModules, error) {
	n, err := s.node(ctx, s.q, distributorID)
	if err != nil {
		return nil, err
	}
	if n.Type != OrgDistributor {
		return nil, ErrNotDistributor
	}
	children, err := s.q.ListOrganizationChildren(ctx, pgtype.Int8{Int64: distributorID, Valid: true})
	if err != nil {
		return nil, err
	}
	mods, err := s.q.ListModules(ctx)
	if err != nil {
		return nil, err
	}
	ids := []int64{distributorID}
	for _, c := range children {
		ids = append(ids, c.Organization.ID)
	}
	flagRows, err := s.q.ListModuleFlagsForOrgs(ctx, ids)
	if err != nil {
		return nil, err
	}
	flags, modRows := flagsFromRows(flagRows), modulesFromRows(mods)
	out := make([]DealerModules, 0, len(children))
	for _, c := range children {
		o := c.Organization
		if o.Type != OrgDealer {
			continue
		}
		st := Resolve(Input{Modules: modRows, Flags: flags, Org: OrgNode{
			ID: o.ID, Type: o.Type, ParentID: distributorID, ParentType: OrgDistributor,
		}})
		out = append(out, DealerModules{OrgID: o.ID, UUID: o.Uuid, Name: o.Name, Slug: o.Slug, Modules: st})
	}
	return out, nil
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	if s.pool == nil {
		return fn(s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
