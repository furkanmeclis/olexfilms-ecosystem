package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type qrMemStore struct {
	rows map[string]db.QrLoginChallenge
	now  func() time.Time
}

func (s *qrMemStore) CreateQRLoginChallenge(_ context.Context, a db.CreateQRLoginChallengeParams) (db.QrLoginChallenge, error) {
	row := db.QrLoginChallenge{
		Code: a.Code, SecretHash: a.SecretHash, Status: QRStatusPending, ExpiresAt: a.ExpiresAt,
		WebIp: a.WebIp, WebUserAgent: a.WebUserAgent, BrandID: a.BrandID,
		CreatedAt: pgtype.Timestamptz{Time: s.now(), Valid: true},
	}
	s.rows[a.Code] = row
	return row, nil
}

func (s *qrMemStore) GetQRLoginChallengeByCode(_ context.Context, code string) (db.QrLoginChallenge, error) {
	row, ok := s.rows[code]
	if !ok {
		return db.QrLoginChallenge{}, pgx.ErrNoRows
	}
	return row, nil
}

func (s *qrMemStore) update(code string, from []string, fn func(*db.QrLoginChallenge)) (db.QrLoginChallenge, error) {
	row, ok := s.rows[code]
	if !ok || !row.ExpiresAt.Time.After(s.now()) {
		return db.QrLoginChallenge{}, pgx.ErrNoRows
	}
	for _, f := range from {
		if row.Status == f {
			fn(&row)
			s.rows[code] = row
			return row, nil
		}
	}
	return db.QrLoginChallenge{}, pgx.ErrNoRows
}

func (s *qrMemStore) MarkQRLoginChallengeScanned(_ context.Context, code string) (db.QrLoginChallenge, error) {
	return s.update(code, []string{QRStatusPending}, func(r *db.QrLoginChallenge) { r.Status = QRStatusScanned })
}

func (s *qrMemStore) DecideQRLoginChallenge(_ context.Context, a db.DecideQRLoginChallengeParams) (db.QrLoginChallenge, error) {
	return s.update(a.Code, []string{QRStatusPending, QRStatusScanned}, func(r *db.QrLoginChallenge) {
		r.Status, r.UserID = a.Status, a.UserID
	})
}

func (s *qrMemStore) ConsumeQRLoginChallenge(_ context.Context, code string) (db.QrLoginChallenge, error) {
	return s.update(code, []string{QRStatusApproved}, func(r *db.QrLoginChallenge) { r.Status = QRStatusConsumed })
}

func (s *qrMemStore) DeleteStaleQRLoginChallenges(context.Context) (int64, error) { return 0, nil }

func (s *qrMemStore) GetOrganizationByID(context.Context, int64) (db.Organization, error) {
	return db.Organization{}, pgx.ErrNoRows
}

type qrPub struct{ msgs []string }

func (p *qrPub) Publish(_ context.Context, channel string, data any) error {
	b, _ := json.Marshal(data)
	p.msgs = append(p.msgs, channel+" "+string(b))
	return nil
}

// The guest channel only ever carries {status}; secrets and states map to
// the documented errors.
func TestQRLoginStatesAndPublishes(t *testing.T) {
	now := time.Now()
	store := &qrMemStore{rows: map[string]db.QrLoginChallenge{}, now: func() time.Time { return now }}
	pub := &qrPub{}
	uc := &AuthUseCase{now: func() time.Time { return now }}
	uc.SetQRLogin(store, pub, nil, time.Minute)
	ctx := context.Background()

	start, err := uc.QRStart(ctx, model.SessionMeta{IP: "198.51.100.4", UserAgent: "Firefox"})
	if err != nil {
		t.Fatal(err)
	}
	if start.Realtime.Enabled || start.Realtime.Channel != "qr:"+start.Code || start.Secret == "" {
		t.Fatalf("start = %+v", start)
	}
	if store.rows[start.Code].SecretHash == start.Secret {
		t.Fatal("secret stored in clear")
	}
	scan, err := uc.QRScan(ctx, start.Code)
	if err != nil || scan.WebIP != "198.51.100.4" || scan.WebUserAgent != "Firefox" || scan.Status != QRStatusScanned {
		t.Fatalf("scan = %+v %v", scan, err)
	}
	if _, err := uc.QRComplete(ctx, start.Code, "bad", model.SessionMeta{}); !errors.Is(err, ErrQRSecret) {
		t.Fatalf("bad secret = %v", err)
	}
	if _, err := uc.QRComplete(ctx, start.Code, start.Secret, model.SessionMeta{}); !errors.Is(err, ErrQRPending) {
		t.Fatalf("pending complete = %v", err)
	}
	row := store.rows[start.Code]
	row.Status = QRStatusRejected
	store.rows[start.Code] = row
	if _, err := uc.QRComplete(ctx, start.Code, start.Secret, model.SessionMeta{}); !errors.Is(err, ErrQRRejected) {
		t.Fatalf("rejected complete = %v", err)
	}
	now = now.Add(2 * time.Minute)
	other, _ := uc.QRStart(ctx, model.SessionMeta{})
	now = now.Add(2 * time.Minute)
	if _, err := uc.QRStatus(ctx, other.Code); !errors.Is(err, ErrQRExpired) {
		t.Fatalf("expired status = %v", err)
	}
	if _, err := uc.QRScan(ctx, other.Code); !errors.Is(err, ErrQRExpired) {
		t.Fatalf("expired scan = %v", err)
	}
	if _, err := uc.QRStatus(ctx, "missing"); !errors.Is(err, ErrQRNotFound) {
		t.Fatalf("missing = %v", err)
	}
	if len(pub.msgs) != 1 || pub.msgs[0] != "qr:"+start.Code+` {"status":"scanned"}` {
		t.Fatalf("publishes = %v", pub.msgs)
	}
}
