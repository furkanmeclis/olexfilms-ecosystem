package usecase_test

import (
	"encoding/json"
	"strings"
	"testing"

	wh "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
)

// TEC-1230: the placement suggestions of an entry point to the location
// already holding the product, then to the empty leaf locations of the
// warehouse; the answer carries no price.
func TestStockEntryPlacementSuggestions(t *testing.T) {
	e := newEntryEnv(t)
	c := e.caller(e.center)
	w, err := e.tree.CreateWarehouse(e.ctx, c.Caller, wh.WarehouseInput{Code: "S" + e.suffix[len(e.suffix)-6:], Name: "t1230"})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	r, err := e.tree.CreateRoom(e.ctx, c.Caller, w.UUID, wh.RoomInput{Code: "R1", Name: "room"})
	if err != nil {
		t.Fatalf("room: %v", err)
	}
	full, err := e.tree.CreateLocation(e.ctx, c.Caller, wh.LocationInput{RoomUUID: r.UUID, Type: wh.TypeAisle, Code: "A1", Name: "full"})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	empty, err := e.tree.CreateLocation(e.ctx, c.Caller, wh.LocationInput{RoomUUID: r.UUID, Type: wh.TypeAisle, Code: "A2", Name: "empty"})
	if err != nil {
		t.Fatalf("location: %v", err)
	}

	first, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeGenerateNew})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.entries.AddLines(e.ctx, c, first.UUID, wh.EntryLinesInput{ProductUUID: e.piece.Uuid.String(), Count: 2, Prefix: e.prefix("G")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.entries.Place(e.ctx, c, first.UUID, wh.PlaceInput{LocationUUID: &full.UUID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.entries.Confirm(e.ctx, c, first.UUID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	second, err := e.entries.Create(e.ctx, c, wh.EntryInput{WarehouseUUID: w.UUID.String(), Mode: wh.EntryModeGenerateNew})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.entries.AddLines(e.ctx, c, second.UUID, wh.EntryLinesInput{ProductUUID: e.piece.Uuid.String(), Count: 1, Prefix: e.prefix("H")}); err != nil {
		t.Fatal(err)
	}
	got, err := e.entries.PlacementSuggestions(e.ctx, c, second.UUID)
	if err != nil {
		t.Fatalf("suggestions: %v", err)
	}
	if len(got.Products) != 1 || got.Products[0].Product.UUID != e.piece.Uuid || len(got.Products[0].Locations) != 1 {
		t.Fatalf("products = %+v", got.Products)
	}
	if l := got.Products[0].Locations[0]; l.UUID != full.UUID || l.Quantity == nil || *l.Quantity != 2 {
		t.Fatalf("same product location = %+v", l)
	}
	var sawEmpty bool
	for _, l := range got.EmptyLocations {
		if l.UUID == full.UUID {
			t.Fatal("a stocked location is not empty")
		}
		if l.UUID == empty.UUID {
			sawEmpty = l.Quantity == nil
		}
	}
	if !sawEmpty {
		t.Fatalf("empty locations = %+v", got.EmptyLocations)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "price") || strings.Contains(string(raw), "amount") {
		t.Fatalf("suggestions leak a price: %s", raw)
	}

	// Once placed, the line drops out of the suggestions.
	if _, err := e.entries.Place(e.ctx, c, second.UUID, wh.PlaceInput{LocationUUID: &full.UUID}); err != nil {
		t.Fatal(err)
	}
	if got, err = e.entries.PlacementSuggestions(e.ctx, c, second.UUID); err != nil || len(got.Products) != 0 {
		t.Fatalf("after place = %+v %v", got.Products, err)
	}
	// Another organization does not see the entry.
	if _, err := e.entries.PlacementSuggestions(e.ctx, e.caller(e.dist), second.UUID); err == nil {
		t.Fatal("distributor read the center's entry")
	}
}
