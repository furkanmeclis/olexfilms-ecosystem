"use client";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { LabelButton } from "@/features/warehouse/components/label-button";
import {
  locationLabelsPath,
  type WarehouseScanResult,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

type Location = NonNullable<WarehouseScanResult["location"]>;

function LocationPath({ location }: { location: Location }) {
  return (
    <ol
      className="text-muted-foreground flex flex-wrap items-center gap-1 text-xs"
      data-testid="scan-path"
    >
      {location.path.map((p, i) => (
        <li key={`${p.level}-${p.uuid}`} className="flex items-center gap-1">
          {i > 0 ? <span aria-hidden>›</span> : null}
          <span className="font-mono" dir="ltr">
            {p.code}
          </span>
        </li>
      ))}
    </ol>
  );
}

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[9rem_1fr] gap-2 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  );
}

/** What a scan resolved to (TEC-203): a location, a unit or a product. */
export function ScanResultCard({ result }: { result: WarehouseScanResult }) {
  const { t, format } = useLocale();
  const { location, unit, product } = result;

  return (
    <Card data-testid="scan-result" data-type={result.type}>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <Badge>{t(`warehouse.scan.type.${result.type}`)}</Badge>
          <span className="font-mono" dir="ltr">
            {unit?.barcode ??
              location?.full_code ??
              product?.sku ??
              result.code}
          </span>
        </CardTitle>
        <span className="text-muted-foreground text-xs">
          {t(`warehouse.scan.matched_by.${result.matched_by}`)}
        </span>
      </CardHeader>
      <CardContent className="space-y-3">
        {result.type === "location" && location ? (
          <dl className="space-y-2">
            <Row label={t("warehouse.fields.type")}>
              {location.type ? t(`warehouse.type.${location.type}`) : "—"}
            </Row>
            <Row label={t("warehouse.fields.name")}>{location.name}</Row>
            <Row label={t("warehouse.scan.path")}>
              <LocationPath location={location} />
            </Row>
            <Row label={t("warehouse.fields.active")}>
              {location.active
                ? t("warehouse.scan.yes")
                : t("warehouse.scan.no")}
            </Row>
            <div className="pt-2">
              <LabelButton
                path={locationLabelsPath({ locations: [location.uuid] })}
                filename={`${location.full_code}.pdf`}
              />
            </div>
          </dl>
        ) : null}

        {result.type === "unit" && unit ? (
          <dl className="space-y-2">
            {product ? (
              <Row label={t("warehouse.fields.product")}>
                {product.name}{" "}
                <span
                  className="text-muted-foreground font-mono text-xs"
                  dir="ltr"
                >
                  {product.sku}
                </span>
              </Row>
            ) : null}
            <Row label={t("warehouse.fields.status")}>
              {t(`stock.status.${unit.status}`)}
            </Row>
            <Row label={t("warehouse.fields.holder")}>
              {unit.holder?.name ?? "—"}
            </Row>
            <Row label={t("warehouse.fields.location")}>
              {unit.location ? (
                <span className="font-mono" dir="ltr">
                  {unit.location.full_code}
                </span>
              ) : (
                t("warehouse.scan.unplaced")
              )}
            </Row>
            {unit.remaining_meters !== null ? (
              <Row label={t("warehouse.fields.meters")}>
                {t("stock.list.meters_of", {
                  remaining: unit.remaining_meters,
                  initial: unit.initial_meters ?? "—",
                })}
              </Row>
            ) : unit.quantity_on_hand !== null ? (
              <Row label={t("warehouse.fields.quantity")}>
                {format.number(unit.quantity_on_hand)}
              </Row>
            ) : null}
            <div className="pt-2">
              <LabelButton
                path={`/v1/stock/labels/units.pdf?barcode=${encodeURIComponent(unit.barcode)}`}
                filename={`${unit.barcode}.pdf`}
              />
            </div>
          </dl>
        ) : null}

        {result.type === "product" && product ? (
          <dl className="space-y-2">
            <Row label={t("warehouse.fields.product")}>{product.name}</Row>
            <Row label={t("warehouse.fields.sku")}>
              <span className="font-mono" dir="ltr">
                {product.sku}
              </span>
            </Row>
            <Row label={t("warehouse.fields.unit_type")}>
              {t(`warehouse.unit_type.${product.unit_type}`)}
            </Row>
          </dl>
        ) : null}
      </CardContent>
    </Card>
  );
}
