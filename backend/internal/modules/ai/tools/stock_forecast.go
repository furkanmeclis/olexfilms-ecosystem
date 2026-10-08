package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	sfuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockforecast/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
)

type StockForecastReader interface {
	List(context.Context, sfuc.Caller, sfuc.ListFilter) ([]sfuc.ForecastView, int64, error)
	Detail(context.Context, sfuc.Caller, uuid.UUID) (sfuc.ProductDetail, error)
}

type stockForecastBase struct {
	forecasts StockForecastReader
	tree      scopefilter.TreeReader
}

func (b stockForecastBase) caller(ctx context.Context, p Principal) (sfuc.Caller, error) {
	f, err := resolveScope(ctx, b.tree, p, rbac.PermStockForecastRead)
	if err != nil {
		return sfuc.Caller{}, err
	}
	return sfuc.Caller{Principal: p.Auth, Org: *p.Org, Filter: f}, nil
}

func (stockForecastBase) errs(tool string) errCases {
	return errCases{tool: tool, what: "stock forecast", notFound: []error{sfuc.ErrNotFound},
		forbidden: []error{sfuc.ErrForbidden}}
}

func NewStockForecastTools(svc StockForecastReader, tree scopefilter.TreeReader) []Tool {
	if svc == nil {
		return nil
	}
	b := stockForecastBase{forecasts: svc, tree: tree}
	return []Tool{StockForecastList{b}, StockForecastProduct{b}}
}

type StockForecastList struct{ stockForecastBase }

func (StockForecastList) Spec() Spec {
	return Spec{
		Name:        "stock_forecast_list",
		Description: "List latest algorithmic stock forecasts for your organization: status, days left, on-hand amount and suggested order amount.",
		InputSchema: object(map[string]any{
			"query":         str("Optional product name or SKU.", 100),
			"status":        enum("Optional forecast status.", sfuc.StatusCritical, sfuc.StatusWarning, sfuc.StatusOK, sfuc.StatusNoConsumption, sfuc.StatusInsufficientData),
			"days_left_max": map[string]any{"type": "number", "description": "Only products with days_left at or below this value."},
			"limit":         limitProp(),
		}),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleStockForecast,
		Permissions: []string{rbac.PermStockForecastRead},
	}
}

func (t StockForecastList) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		Query       string   `json:"query"`
		Status      string   `json:"status"`
		DaysLeftMax *float64 `json:"days_left_max"`
		Limit       int      `json:"limit"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	statuses := []string(nil)
	if strings.TrimSpace(in.Status) != "" {
		statuses = []string{in.Status}
	}
	items, total, err := t.forecasts.List(ctx, c, sfuc.ListFilter{
		Q: strings.TrimSpace(in.Query), Statuses: statuses, DaysLeftMax: in.DaysLeftMax,
		Limit: limitArg(in.Limit), SortKey: "days_left",
	})
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	return JSONResult(NewList(items, total))
}

type StockForecastProduct struct{ stockForecastBase }

func (StockForecastProduct) Spec() Spec {
	return Spec{
		Name:        "stock_forecast_product",
		Description: "Read one product's stock forecast detail: 90-day consumption series, 90-day projection, thresholds and parameters.",
		InputSchema: object(map[string]any{
			"product_uuid": strMin("Product uuid from stock_forecast_list or search_products.", 36, 36),
		}, "product_uuid"),
		Kind: KindRead, Realm: RealmPanel, Feature: features.ModuleStockForecast,
		Permissions: []string{rbac.PermStockForecastRead},
	}
}

func (t StockForecastProduct) Run(ctx context.Context, env Env, raw json.RawMessage) (Result, error) {
	var in struct {
		ProductUUID string `json:"product_uuid"`
	}
	if r := decode(t.Spec(), raw, &in); r != nil {
		return *r, nil
	}
	id, ok := parseID(in.ProductUUID)
	if !ok {
		return invalidArg("product_uuid must be a uuid"), nil
	}
	c, err := t.caller(ctx, env.Principal)
	if err != nil {
		return t.errs(t.Spec().Name).result(err)
	}
	item, err := t.forecasts.Detail(ctx, c, id)
	if err != nil {
		if errors.Is(err, sfuc.ErrNotFound) {
			return notFound("stock forecast"), nil
		}
		return t.errs(t.Spec().Name).result(err)
	}
	return JSONResult(item)
}
