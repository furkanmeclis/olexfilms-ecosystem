package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TEC-170: a reversed or empty created range is refused before any query runs.
func TestListRejectsReversedCreatedRange(t *testing.T) {
	from := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	_, _, err := (&Service{}).List(context.Background(), Caller{}, ListFilter{
		Side: SideBuyer, CreatedFrom: &from, CreatedTo: &to,
	})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "created_to" {
		t.Fatalf("want created_to validation error, got %v", err)
	}
	same := from
	_, _, err = (&Service{}).List(context.Background(), Caller{}, ListFilter{
		Side: SideBuyer, CreatedFrom: &from, CreatedTo: &same,
	})
	if !errors.As(err, &ve) {
		t.Fatalf("an empty range must be refused, got %v", err)
	}
}
