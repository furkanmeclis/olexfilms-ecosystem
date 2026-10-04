package usecase

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
)

// WithTx returns a copy of the service bound to the caller's transaction
// (TEC-316 lead conversion): its own transactions become savepoints and
// commit only with the caller's transaction. Search index mutations are held
// back; the caller runs flush after its commit so the index never sees rows
// that are not committed (or are rolled back).
func (s *Service) WithTx(tx pgx.Tx) (svc *Service, flush func(context.Context)) {
	cp := *s
	cp.pool = tx
	cp.q = s.q.WithTx(tx)
	if s.search == nil {
		return &cp, func(context.Context) {}
	}
	d := &deferredIndexer{}
	cp.search = d
	return &cp, func(ctx context.Context) { d.flush(ctx, s.search) }
}

type deferredOp struct {
	del      bool
	spec, id string
}

type deferredIndexer struct {
	mu  sync.Mutex
	ops []deferredOp
}

func (d *deferredIndexer) EnqueueUpsert(_ context.Context, spec, id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ops = append(d.ops, deferredOp{spec: spec, id: id})
}

func (d *deferredIndexer) EnqueueDelete(_ context.Context, spec, id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ops = append(d.ops, deferredOp{del: true, spec: spec, id: id})
}

func (d *deferredIndexer) flush(ctx context.Context, to SearchIndexer) {
	d.mu.Lock()
	ops := d.ops
	d.ops = nil
	d.mu.Unlock()
	for _, op := range ops {
		if op.del {
			to.EnqueueDelete(ctx, op.spec, op.id)
		} else {
			to.EnqueueUpsert(ctx, op.spec, op.id)
		}
	}
}
