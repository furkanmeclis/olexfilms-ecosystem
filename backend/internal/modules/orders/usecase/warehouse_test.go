package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// TEC-1052: the warehouse staff are told of a new order without the actor.
func TestWarehouseNewRecipients(t *testing.T) {
	cases := []struct {
		name  string
		staff []int64
		actor int64
		want  []int64
	}{
		{"actor dropped", []int64{1, 2, 3}, 2, []int64{1, 3}},
		{"actor not staff", []int64{1, 3}, 9, []int64{1, 3}},
		{"only the actor", []int64{4}, 4, []int64{}},
		{"no staff", nil, 4, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := warehouseNewRecipients(tc.staff, tc.actor); !slices.Equal(got, tc.want) {
				t.Fatalf("recipients = %v, want %v", got, tc.want)
			}
		})
	}
}

// TEC-1052: self_prepare needs orders.ship; the check runs before any
// database access.
func TestCreateForBuyerSelfPrepareNeedsShip(t *testing.T) {
	s := &Service{}
	buyer := "00000000-0000-0000-0000-000000000001"
	_, err := s.createForBuyer(context.Background(), Caller{}, CreateInput{SelfPrepare: true, BuyerOrgUUID: &buyer})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "self_prepare" {
		t.Fatalf("self_prepare without orders.ship = %v", err)
	}
}
