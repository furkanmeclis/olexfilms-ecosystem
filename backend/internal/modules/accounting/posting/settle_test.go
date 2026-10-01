package posting

import (
	"context"
	"errors"
	"testing"
)

// TEC-172: a collection or payment always moves a cash/bank account and
// settles a cari; anything else is refused before the database.
func TestSettlementNeedsAccountAndCari(t *testing.T) {
	t.Parallel()
	p := New(nil, nil, nil)
	for _, e := range []Entry{
		{AccountID: 1},
		{CounterpartyOrgID: 2},
		{},
	} {
		if _, err := p.Collect(context.Background(), nil, e); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Collect(%+v) = %v", e, err)
		}
		if _, err := p.Pay(context.Background(), nil, e); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Pay(%+v) = %v", e, err)
		}
	}
}
