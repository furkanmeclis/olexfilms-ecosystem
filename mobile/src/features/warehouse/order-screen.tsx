import { useQueryClient } from "@tanstack/react-query";
import { Stack, useLocalSearchParams } from "expo-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Alert,
  Modal,
  Pressable,
  ScrollView,
  Switch,
  TextInput,
  View,
} from "react-native";

import { useCan } from "@/core/spaces";
import { usePricesVisible } from "@/core/use-prices-visible";
import { LinePlotter, OrderLinesEditor } from "@/features/orders/order-edit";
import { creatorOf } from "@/features/orders/order-summary";
import { OrderTimeline, useAmountText } from "@/features/orders/order-timeline";
import { mergeCuts } from "@/features/stock/family";
import { Button } from "@/ui/button";
import { Icon } from "@/ui/icon";
import { Input } from "@/ui/input";
import { SectionHeader } from "@/ui/list";
import { Screen } from "@/ui/screen";
import { StatusPill, type StatusTone } from "@/ui/status-pill";
import { Text } from "@/ui/text";
import { PullRefresh } from "@/ui/pull-refresh";
import {
  BackorderRows,
  DiscrepancySection,
  PartialShipButton,
  ReceiptCheckSheet,
} from "@/features/orders/discrepancy";

import { resolveScan } from "./api";
import { ContinuousScanner, type ScanOutcome } from "./continuous-scanner";
import {
  confirmAction,
  formatDate,
  formatNumber,
  isCode,
  whError,
} from "./format";
import {
  type Order,
  orderApi,
  type OrderItem,
  orderKeys,
  type OrderRollSplit,
  type OrderStatus,
  useDeliveryMethods,
  useOrder,
  usePickList,
  usePickLocations,
  useSuggestions,
  useUnplaced,
} from "./orders-api";
import {
  allCovered as everyLineCovered,
  asksWholeRoll,
  assignTarget,
  cutAssignBody,
  excessOf,
  fixedAssignQuantity,
  fixedSuggestionUnits,
  isCovered,
  isCutItem,
  isMeterItem,
  parseShipment,
  requiredOf,
} from "./order-assign";
import { CutConfirm } from "./cut-confirm";
import { canShareLabels, shareLabels } from "./label-print";
import {
  CancelSheet,
  cancelActions,
  DocumentsCard,
  WorkCard,
  type CancelKind,
} from "./order-extras";
import {
  Card,
  ErrorState,
  Field,
  IconButton,
  ListSkeleton,
  Segmented,
} from "./parts";
import { QtyStepper } from "./qty-stepper";
import { useDocAction } from "./use-action";
import { createRetryKey } from "./uuid";

export function orderTone(s: OrderStatus): StatusTone {
  if (s === "received" || s === "delivered") return "done";
  if (s === "cancelled" || s === "cancelling") return "problem";
  if (s === "draft" || s === "submitted") return "neutral";
  return "progress";
}

/** Ship transition extras: delivery method, tracking number and (TEC-1228) package count, desi, weight. */
type ShipExtra = {
  delivery_method_uuid?: string;
  tracking_no?: string;
  package_count?: number;
  desi?: number;
  weight_kg?: number;
};

const isRoll = (i: OrderItem) => isMeterItem(i);
const required = (i: OrderItem) => requiredOf(i);
const covered = (i: OrderItem) => isCovered(i);
const amount = (i: OrderItem, v: number | string | null) =>
  isRoll(i) ? `${formatNumber(v, 2)} m` : formatNumber(v);

/**
 * Order preparation in the warehouse. Seller: start preparing, assign units
 * by scan or suggestion, pick list and locations, mark ready, ship with a
 * delivery method. Buyer: receive, then shelve the received units.
 */
export function OrderScreen() {
  const { t } = useTranslation("warehouse");
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const qc = useQueryClient();
  const canShip = useCan("orders.ship");
  const canReceive = useCan("orders.receive");
  const canPlace = useCan("warehouse.write");
  const prices = usePricesVisible();
  const q = useOrder(uuid);
  const amountText = useAmountText();
  const o = q.data;
  const { busy, run } = useDocAction<Order>(orderKeys.order(uuid));
  const [view, setView] = useState<"lines" | "pick">("lines");
  const [scanning, setScanning] = useState<"assign" | "place" | null>(null);
  const [qty, setQty] = useState(1);
  // TEC-1048: an untouched stepper sends what the line still needs (one scan per fixed barcode line).
  const [qtyTouched, setQtyTouched] = useState(false);
  const resetQty = () => {
    setQty(1);
    setQtyTouched(false);
  };
  const [meters, setMeters] = useState("");
  const [suggestFor, setSuggestFor] = useState<OrderItem | null>(null);
  const [locationsFor, setLocationsFor] = useState<string | null>(null);
  const [shipping, setShipping] = useState(false);
  // TEC-1228: the cancel move being confirmed (with its reason).
  const [cancelling, setCancelling] = useState<{
    kind: CancelKind;
    status: OrderStatus;
    reasonRequired: boolean;
  } | null>(null);
  const [editing, setEditing] = useState(false);
  // TEC-1227: the buyer receives through the check sheet (missing / damaged units).
  const [checking, setChecking] = useState(false);
  const [target, setTarget] = useState<{ uuid: string; code: string } | null>(
    null,
  );
  const [splitKey] = useState(createRetryKey);
  // TEC-1241: the cut confirmation over the scanner, and "send in pieces" for the next scan (K11).
  const [cut, setCut] = useState<{
    item: OrderItem;
    barcode: string;
    name: string;
    partial: boolean;
    resolve: (o: ScanOutcome) => void;
  } | null>(null);
  const [partialNext, setPartialNext] = useState(false);

  const seller = o?.role === "seller";
  const preparing = o?.status === "preparing";
  const received = o?.status === "received" && o.role === "buyer";
  const pick = usePickList(uuid, !!o && seller && view === "pick");
  const locations = usePickLocations(uuid, locationsFor);
  const unplaced = useUnplaced(uuid, !!received);

  // TEC-1079: parts cut by this session's assignments (`<roll>-3M`), to label and scan.
  const [cuts, setCuts] = useState<OrderRollSplit[]>([]);
  const store = (next: Order) => {
    qc.setQueryData(orderKeys.order(uuid), next);
    setCuts((c) => mergeCuts(c, next));
  };
  const can = (s: OrderStatus) => !!o?.available_transitions.includes(s);

  const transition = (status: OrderStatus, key: string, extra?: ShipExtra) =>
    confirmAction({
      title: t(`orders.actions.${key}.title`),
      body: t(`orders.actions.${key}.description`),
      action: t(`orders.actions.${key}.confirm`),
      onConfirm: () =>
        void run(key, () => orderApi.transition(uuid, { status, ...extra }), {
          done: () => {
            setShipping(false);
            void qc.invalidateQueries({ queryKey: ["warehouse", "orders"] });
          },
        }),
    });

  /** Empty meters on a roll assign all of it: asked while the scanner waits. */
  const askWholeRoll = (remaining: string | null) =>
    new Promise<boolean>((resolve) =>
      Alert.alert(
        t("orders.assign.whole_roll_title", {
          meters: remaining ? formatNumber(remaining, 2) : "—",
        }),
        t("orders.assign.whole_roll_body"),
        [
          {
            text: t("common:cancel"),
            style: "cancel",
            onPress: () => resolve(false),
          },
          {
            text: t("orders.assign.whole_roll_confirm"),
            onPress: () => resolve(true),
          },
        ],
        { cancelable: true, onDismiss: () => resolve(false) },
      ),
    );

  /** Unassign from the line, asked first (a stray tap would drop it silently). */
  const askUnassign = (item: string, unit: string, barcode: string) =>
    confirmAction({
      title: t("orders.assign.remove_confirm", { barcode }),
      action: t("orders.assign.remove"),
      destructive: true,
      onConfirm: () =>
        void run(`del:${unit}`, () => orderApi.unassign(uuid, item, unit)),
    });

  /**
   * TEC-1241: a meter line of a box product is cut. The scan waits for the
   * confirmation (box opening, cut, remainder, remnant, credits); the label
   * of the new piece is handed to the print share sheet right away.
   */
  const askCut = (item: OrderItem, barcode: string, name: string) =>
    new Promise<ScanOutcome>((resolve) =>
      setCut({ item, barcode, name, partial: partialNext, resolve }),
    );

  const confirmCut = async (
    c: NonNullable<typeof cut>,
    preview: Parameters<typeof cutAssignBody>[1],
    partial: Parameters<typeof cutAssignBody>[2],
  ) => {
    const body = cutAssignBody(
      c.barcode,
      preview,
      partial,
      splitKey.for(`cut:${c.item.uuid}:${c.barcode}:${partial?.meters ?? ""}`),
    );
    const next = await orderApi.assign(uuid, c.item.uuid, body);
    store(next);
    splitKey.done();
    setPartialNext(false);
    setCut(null);
    const label = next.split?.new_barcode ?? c.barcode;
    if (canShareLabels()) {
      void shareLabels({
        path: "/v1/stock/labels/units.pdf",
        query: { barcode: [label] },
        name: `cut-${label}`,
        barcodes: [],
      }).catch(() => undefined);
    }
    c.resolve({
      ok: true,
      title: c.name,
      detail: next.split
        ? t("orders.cut.done", {
            barcode: next.split.new_barcode,
            meters: formatNumber(next.split.meters, 2),
            rest: formatNumber(next.split.source_remaining_meters, 2),
          })
        : t("orders.cut.done_whole", { barcode: label }),
    });
  };

  /** Scan a unit: it goes to the first open line of its product. */
  const onAssign = async (code: string): Promise<ScanOutcome> => {
    if (!o?.items) return { ok: false, title: t("scan.failed") };
    try {
      const r = await resolveScan(code);
      // TEC-1048: the product code of a fixed-barcode product (no own label per
      // piece) prepares the line by quantity from the suggested fixed barcodes.
      if (
        r.status === "ok" &&
        r.type === "product" &&
        r.product?.uses_fixed_barcode
      ) {
        return await assignFixedByProduct(r.product.uuid, r.product.name);
      }
      if (r.status !== "ok" || r.type !== "unit" || !r.unit)
        return { ok: false, title: t("m.scan_unit_not_product") };
      // TEC-1241: a unit with meters goes to the meter line, a closed box to the box line first.
      const line = assignTarget(
        o.items,
        r.product?.uuid,
        r.unit.unit_kind === "fixed"
          ? undefined
          : r.unit.remaining_meters != null,
      );
      if (line.kind === "not_on_order")
        return {
          ok: false,
          title: t("m.not_on_order", { barcode: r.unit.barcode }),
        };
      // Every line of this product is full: say so instead of over-assigning.
      if (line.kind === "covered")
        return {
          ok: false,
          title: t("orders.assign.line_full", { name: line.item.product.name }),
        };
      const item = line.item;
      if (isCutItem(item))
        return await askCut(
          item,
          r.unit.barcode,
          r.product?.name ?? r.unit.barcode,
        );
      if (r.unit.unit_kind !== "fixed" && asksWholeRoll(item, meters)) {
        const whole = await askWholeRoll(r.unit.remaining_meters);
        if (!whole)
          return {
            ok: false,
            title: t("orders.assign.whole_roll_skipped"),
            detail: r.unit.barcode,
          };
      }
      const body: {
        barcode: string;
        quantity?: number;
        meters?: string;
        idempotency_key?: string;
      } = {
        barcode: r.unit.barcode,
      };
      if (r.unit.unit_kind === "fixed")
        body.quantity = fixedAssignQuantity(
          item,
          qty,
          qtyTouched,
          r.unit.quantity_on_hand,
        );
      if (isRoll(item) && !isCutItem(item) && meters.trim()) {
        body.meters = meters.trim().replace(",", ".");
        body.idempotency_key = splitKey.for(
          `${item.uuid}:${r.unit.barcode}:${body.meters}`,
        );
      }
      const next = await orderApi.assign(uuid, item.uuid, body);
      store(next);
      splitKey.done();
      if (body.quantity) resetQty();
      if (body.meters) setMeters("");
      return {
        ok: true,
        title: r.product?.name ?? r.unit.barcode,
        detail: next.split
          ? t("m.split_new_code", {
              barcode: next.split.new_barcode,
              meters: formatNumber(next.split.meters, 2),
            })
          : body.meters
            ? `${body.meters} m`
            : body.quantity
              ? `× ${body.quantity}`
              : r.unit.location?.full_code,
      };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  /** Fixed-barcode product scanned by its code: assign the line's missing pieces from its fixed barcodes. */
  const assignFixedByProduct = async (
    product: string,
    name: string,
  ): Promise<ScanOutcome> => {
    const line = assignTarget(o?.items ?? [], product);
    if (line.kind === "not_on_order")
      return { ok: false, title: t("m.not_on_order", { barcode: name }) };
    if (line.kind === "covered")
      return { ok: false, title: t("orders.assign.line_full", { name }) };
    const s = await orderApi.suggestions(uuid, line.item.uuid, true);
    const units = fixedSuggestionUnits(s.suggestions, qtyTouched ? qty : null);
    if (!units.length)
      return { ok: false, title: t("orders.assign.fixed_none", { name }) };
    store(await orderApi.applySuggestions(uuid, line.item.uuid, { units }));
    resetQty();
    const total = units.reduce((n, u) => n + u.quantity, 0);
    return { ok: true, title: name, detail: `× ${total}` };
  };

  /** Buyer: a location QR sets the shelf; each unit scanned goes onto it. */
  const onPlace = async (code: string): Promise<ScanOutcome> => {
    try {
      const r = await resolveScan(code).catch((e: unknown) => {
        if (isCode(e, "SCAN_NO_MATCH")) return null;
        throw e;
      });
      if (r?.type === "location" && r.location) {
        setTarget({ uuid: r.location.uuid, code: r.location.full_code });
        return {
          ok: true,
          title: t("m.target_set", { location: r.location.full_code }),
        };
      }
      if (!target)
        return { ok: false, title: t("order_place.scan_location_first") };
      const barcode = r?.unit?.barcode ?? code;
      const res = await orderApi.place(uuid, {
        barcodes: [barcode],
        location_uuid: target.uuid,
      });
      void unplaced.refetch();
      const skipped = res.units.find((u) => u.skipped);
      if (!res.units.length || (skipped && res.units.length === 1)) {
        return {
          ok: false,
          title:
            skipped?.skipped === "fixed"
              ? t("order_place.fixed_scanned", { barcode })
              : t("order_place.already_placed", { barcode }),
        };
      }
      return {
        ok: true,
        title: r?.product?.name ?? barcode,
        detail: `→ ${target.code}`,
      };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  if (!o) {
    return (
      <>
        <Stack.Screen
          options={{ title: t("orders.detail.title"), headerLargeTitle: false }}
        />
        <Screen>
          {q.isLoading ? (
            <ListSkeleton rows={4} />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )}
        </Screen>
      </>
    );
  }

  const items = o.items ?? [];
  const allCovered = everyLineCovered(items);
  const other = seller ? o.buyer : o.seller;

  return (
    <>
      <Stack.Screen options={{ title: o.order_no, headerLargeTitle: false }} />
      <Screen
        refreshControl={
          <PullRefresh
            busy={q.isRefetching}
            onRefresh={() => {
              void q.refetch();
              if (view === "pick") void pick.refetch();
              if (received) void unplaced.refetch();
            }}
          />
        }
      >
        <Card>
          <View className="flex-row items-center gap-2 pb-1">
            <Icon name="package" tone="primary" size={18} />
            <Text variant="headline" className="flex-1" numberOfLines={2}>
              {other.name}
            </Text>
            <StatusPill
              tone={orderTone(o.status)}
              label={t(`orders.status.${o.status}`)}
            />
          </View>
          <Field
            label={t("orders.detail.role")}
            value={t(`orders.role.${o.role}`)}
          />
          {/* TEC-1053: the warehouse the center picked for this order (seller side). */}
          <Field
            label={t("fields.warehouse")}
            value={o.seller_warehouse?.name}
          />
          <Field
            label={t("orders.shipping.method")}
            value={o.delivery_method?.name}
          />
          <Field
            label={t("orders.shipping.tracking_no")}
            value={o.tracking_no}
            mono
          />
          <Field label={t("fields.note")} value={o.note} />
          {/* TEC-1067 / TEC-1059: how much is ordered and who placed it. */}
          <Field label={t("orders:detail.amount")} value={amountText(o)} />
          <Field
            label={t("orders:detail.created_by")}
            value={creatorOf(o.timeline)?.name}
          />
          <Field
            label={t("orders.detail.created_at")}
            value={formatDate(o.created_at)}
          />
          <Field
            label={t("orders.detail.approved_at")}
            value={formatDate(o.approved_at)}
          />
          <Field
            label={t("orders.detail.shipped_at")}
            value={formatDate(o.shipped_at)}
          />
          <Field label={t("m.received")} value={formatDate(o.received_at)} />
        </Card>

        {/* TEC-1227: backorder links and the receipt records (no prices). */}
        <BackorderRows order={o} />
        <DiscrepancySection order={o} onUpdated={store} />
        {/* TEC-1228: priority, due date and who took the job. */}
        {seller ? (
          <WorkCard
            order={o}
            busy={busy}
            onClaim={(takeover) =>
              void run("claim", () => orderApi.claim(uuid, takeover), {
                done: () =>
                  void qc.invalidateQueries({
                    queryKey: ["warehouse", "orders"],
                  }),
              })
            }
            onRelease={() => void run("release", () => orderApi.release(uuid))}
          />
        ) : null}

        {/* Seller actions */}
        {seller && canShip ? (
          <View className="gap-2">
            {can("preparing") ? (
              <Button
                size="lg"
                title={t("orders.actions.start_preparing.button")}
                loading={busy === "start_preparing"}
                // Nothing to pick on an order without items.
                disabled={items.length === 0}
                onPress={() => transition("preparing", "start_preparing")}
              />
            ) : null}
            {preparing ? (
              <>
                <Button
                  size="lg"
                  title={t("m.scan_to_assign")}
                  icon={<Icon name="scan" tone="primary-foreground" />}
                  onPress={() => setScanning("assign")}
                />
                <Text variant="footnote" tone="muted">
                  {allCovered
                    ? t("orders.assign.complete")
                    : t("orders.assign.not_complete")}
                </Text>
              </>
            ) : null}
            {can("ready") ? (
              <Button
                variant="secondary"
                title={t("orders.actions.mark_ready.button")}
                disabled={!allCovered}
                loading={busy === "mark_ready"}
                onPress={() => transition("ready", "mark_ready")}
              />
            ) : null}
            {/* TEC-1227: stock short — ship the picked part, the rest becomes a backorder. */}
            <PartialShipButton
              order={o}
              onReady={(next) => {
                store(next);
                if (next.available_transitions.includes("shipped"))
                  setShipping(true);
              }}
            />
            {/* TEC-1057: lines are fixed before the order is ready (add, change, remove). */}
            {o.can_edit_items ? (
              <Button
                variant="outline"
                title={t("orders:edit.button")}
                icon="edit"
                onPress={() => setEditing(true)}
              />
            ) : null}
            {can("shipped") ? (
              <Button
                size="lg"
                title={t("orders.actions.ship.button")}
                icon={<Icon name="truck" tone="primary-foreground" />}
                onPress={() => setShipping(true)}
              />
            ) : null}
          </View>
        ) : null}

        {/* TEC-1241: pieces cut for this order that stayed in stock (cancelled order, unassigned piece). */}
        {o.detached_cut_pieces?.length ? (
          <Card>
            <View className="flex-row items-center gap-2">
              <Icon name="alert" tone="destructive" size={18} />
              <Text variant="callout" className="flex-1 font-semibold">
                {t("orders.cut.detached", {
                  count: o.detached_cut_pieces.length,
                })}
              </Text>
            </View>
            {o.detached_cut_pieces.map((p) => (
              <Field
                key={p.unit_uuid}
                label={`${formatNumber(p.remaining_meters ?? p.cut_meters, 2)} m`}
                value={p.barcode}
                mono
              />
            ))}
          </Card>
        ) : null}
        {cuts.length ? (
          <>
            <SectionHeader title={t("orders.assign.cuts_title")} />
            <Card>
              {cuts.map((c) => (
                <Field
                  key={c.uuid}
                  label={t("orders.assign.cut_from", {
                    meters: formatNumber(c.meters, 2),
                    source: c.source_barcode,
                    remaining: formatNumber(c.source_remaining_meters, 2),
                  })}
                  value={c.new_barcode}
                  mono
                />
              ))}
            </Card>
          </>
        ) : null}

        {/* Seller: hand delivery or pickup at the warehouse closes the trip (TEC-1025); by courier the buyer receives directly. */}
        {seller && can("delivered") ? (
          <Button
            size="lg"
            variant="secondary"
            title={t("orders.actions.deliver.button")}
            icon={<Icon name="packageCheck" tone="secondary-foreground" />}
            loading={busy === "deliver"}
            onPress={() => transition("delivered", "deliver")}
          />
        ) : null}

        {/* TEC-1062/1063: the seller receives for a waiting pool recipient without members. */}
        {seller && o.receive_on_behalf && can("received") ? (
          <Button
            size="lg"
            variant="secondary"
            title={t("orders.actions.receive_on_behalf.button")}
            loading={busy === "receive_on_behalf"}
            onPress={() => transition("received", "receive_on_behalf")}
          />
        ) : null}

        {/* Buyer actions */}
        {!seller && canReceive && can("received") ? (
          <Button
            size="lg"
            title={t("orders.actions.receive.button")}
            loading={busy === "receive"}
            onPress={() => setChecking(true)}
          />
        ) : null}
        {received && canPlace ? (
          <Card>
            <Text variant="headline">{t("order_place.title")}</Text>
            {unplaced.isLoading ? (
              <ListSkeleton rows={1} />
            ) : unplaced.data ? (
              <>
                <Text variant="callout" tone="muted">
                  {unplaced.data.unplaced_count
                    ? t("order_place.unplaced_badge", {
                        count: unplaced.data.unplaced_count,
                      })
                    : unplaced.data.units.length
                      ? t("order_place.all_placed", {
                          count: unplaced.data.placed_count,
                        })
                      : t("order_place.nothing_to_place")}
                </Text>
                {unplaced.data.unplaced_count ? (
                  <Button
                    className="mt-2"
                    title={t("order_place.scan_location")}
                    icon={<Icon name="location" tone="primary-foreground" />}
                    onPress={() => setScanning("place")}
                  />
                ) : null}
                {unplaced.data.fixed.length ? (
                  <Text variant="footnote" tone="muted" className="pt-1">
                    {t("order_place.fixed_hint")}
                  </Text>
                ) : null}
              </>
            ) : (
              <ErrorState
                error={unplaced.error}
                onRetry={() => void unplaced.refetch()}
              />
            )}
          </Card>
        ) : null}

        {seller ? (
          <Segmented
            value={view}
            onChange={setView}
            options={[
              { value: "lines", label: t("orders.picking.lines") },
              { value: "pick", label: t("orders.picking.pick_list") },
            ]}
          />
        ) : (
          <SectionHeader
            title={t("orders.detail.items", { count: items.length })}
          />
        )}

        {view === "pick" && seller ? (
          pick.isLoading ? (
            <ListSkeleton rows={4} />
          ) : !pick.data ? (
            <ErrorState
              error={pick.error}
              onRetry={() => void pick.refetch()}
            />
          ) : pick.data.rows.length === 0 ? (
            <Text variant="callout" tone="muted" className="px-4">
              {t("orders.picking.empty")}
            </Text>
          ) : (
            <View className="gap-2">
              {pick.data.rows.map((r, i) => (
                <View
                  key={`${r.item_uuid}-${i}`}
                  className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-2.5"
                >
                  <View className="w-20">
                    <Text
                      variant="callout"
                      className="font-mono font-semibold"
                      numberOfLines={1}
                    >
                      {r.location_code ?? t("orders.picking.no_location")}
                    </Text>
                  </View>
                  <View className="flex-1 gap-0.5">
                    <Text variant="callout" numberOfLines={1}>
                      {r.product.name}
                    </Text>
                    <Text variant="footnote" tone="muted" numberOfLines={1}>
                      {r.kind === "stock"
                        ? t("orders.picking.stock_row")
                        : (r.barcode ?? "")}
                    </Text>
                  </View>
                  <Text variant="headline">
                    {r.meters
                      ? `${formatNumber(r.meters, 2)} m`
                      : formatNumber(r.quantity)}
                  </Text>
                </View>
              ))}
            </View>
          )
        ) : items.length === 0 ? (
          <Text variant="callout" tone="muted" className="px-4">
            {t("orders.detail.items_empty")}
          </Text>
        ) : (
          <View className="gap-2">
            {items.map((i) => (
              <View
                key={i.uuid}
                className="gap-2 rounded-lg border border-border bg-card px-4 py-3"
              >
                <View className="flex-row items-start gap-2">
                  <View className="flex-1 gap-0.5">
                    <Text
                      variant="callout"
                      className="font-semibold"
                      numberOfLines={2}
                    >
                      {i.product.name}
                    </Text>
                    <Text variant="footnote" tone="muted" className="font-mono">
                      {i.product.sku}
                    </Text>
                    {/* TEC-1051: the warehouse space shows nothing about prices. */}
                    {prices && i.price_missing ? (
                      <View className="self-start pt-1">
                        <StatusPill
                          tone="problem"
                          label={t("orders.price_missing")}
                        />
                      </View>
                    ) : null}
                  </View>
                  <View className="items-end">
                    <Text
                      variant="headline"
                      className={covered(i) ? "text-success" : ""}
                    >
                      {amount(i, i.assigned)} / {amount(i, required(i))}
                    </Text>
                    <Text variant="footnote" tone="muted">
                      {t("orders.assign.assigned")}
                    </Text>
                    {excessOf(i) > 0 ? (
                      <Text variant="footnote" tone="muted">
                        {t("orders.cut.excess_free", {
                          excess: formatNumber(i.excess_meters, 2),
                        })}
                      </Text>
                    ) : null}
                  </View>
                </View>

                {/* TEC-1059: "sent plotter cut", information only. */}
                <LinePlotter order={o} item={i} />

                {i.units.map((u) => (
                  <View
                    key={u.unit_uuid}
                    className="flex-row items-center gap-2 border-t border-border pt-1.5"
                  >
                    <Text
                      variant="footnote"
                      className="flex-1 font-mono"
                      numberOfLines={1}
                    >
                      {u.barcode}
                      {u.meters
                        ? ` · ${formatNumber(u.meters, 2)} m`
                        : u.unit_kind === "fixed" && u.quantity
                          ? ` × ${u.quantity}`
                          : ""}
                      {isCutItem(i) && u.credits != null
                        ? ` · ${t("orders.cut.unit_credits", { credits: u.credits })}`
                        : ""}
                    </Text>
                    {preparing && canShip && !u.shipped ? (
                      <IconButton
                        icon="delete"
                        tone="destructive"
                        label={t("orders.assign.remove")}
                        busy={busy === `del:${u.unit_uuid}`}
                        onPress={() =>
                          askUnassign(i.uuid, u.unit_uuid, u.barcode)
                        }
                      />
                    ) : null}
                  </View>
                ))}

                {seller && canShip && (preparing || o.status === "approved") ? (
                  <View className="flex-row gap-2">
                    {preparing && !covered(i) ? (
                      <Button
                        className="flex-1"
                        variant="secondary"
                        title={t("orders.suggest.button")}
                        onPress={() => setSuggestFor(i)}
                      />
                    ) : null}
                    <Button
                      className="flex-1"
                      variant="outline"
                      title={t("orders.picking.locations")}
                      onPress={() =>
                        setLocationsFor(locationsFor === i.uuid ? null : i.uuid)
                      }
                    />
                  </View>
                ) : null}

                {locationsFor === i.uuid ? (
                  locations.isLoading ? (
                    <ListSkeleton rows={1} />
                  ) : (locations.data?.length ?? 0) === 0 ? (
                    <Text variant="footnote" tone="muted">
                      {t("orders.picking.locations_empty")}
                    </Text>
                  ) : (
                    <View className="gap-1">
                      {locations.data?.map((l, k) => (
                        <View
                          key={`${l.location_uuid}-${k}`}
                          className="flex-row justify-between"
                        >
                          <Text variant="footnote" className="font-mono">
                            {l.location_code ?? t("orders.picking.no_location")}
                          </Text>
                          <Text variant="footnote">
                            {l.meters
                              ? `${formatNumber(l.meters, 2)} m`
                              : formatNumber(l.quantity)}
                          </Text>
                        </View>
                      ))}
                    </View>
                  )
                ) : null}
              </View>
            ))}
          </View>
        )}

        <OrderTimeline timeline={o.timeline} />

        {/* TEC-1228: shipping note, order slip and the delivery receipt. */}
        <DocumentsCard
          order={o}
          canShip={canShip}
          canReceive={canReceive}
          busy={busy}
          run={run}
          regenerate={() => orderApi.regenerateShippingDocument(uuid)}
          attachReceipt={(photo) => orderApi.attachReceipt(uuid, photo)}
        />

        {/* TEC-1228: cancel moves the server offers (same rules as the web). */}
        {cancelActions(o).length ? (
          <View className="gap-2">
            {cancelActions(o).map((a) => (
              <Button
                key={a.kind}
                variant={a.kind === "complete_cancel" ? "secondary" : "outline"}
                title={t(`orders.actions.${a.kind}.button`)}
                onPress={() => setCancelling(a)}
              />
            ))}
          </View>
        ) : null}
      </Screen>

      <OrderLinesEditor
        order={o}
        visible={editing}
        onClose={() => setEditing(false)}
      />

      <ContinuousScanner
        visible={scanning === "assign"}
        onClose={() => {
          // A pending cut confirmation is cancelled with the scanner.
          cut?.resolve({
            ok: false,
            title: t("orders.cut.cancelled"),
            detail: cut.barcode,
          });
          setCut(null);
          setScanning(null);
        }}
        title={t("m.scan_to_assign")}
        hint={t("orders.assign.hint")}
        onCode={onAssign}
        complete={
          allCovered
            ? {
                title: t("orders.assign.order_complete"),
                action: t("common:close"),
              }
            : undefined
        }
        overlay={
          cut ? (
            <CutConfirm
              key={`${cut.item.uuid}:${cut.barcode}`}
              barcode={cut.barcode}
              productName={cut.name}
              partialStart={cut.partial}
              load={(m) =>
                orderApi.cutPreview(
                  uuid,
                  cut.item.uuid,
                  cut.barcode,
                  m || undefined,
                )
              }
              onConfirm={(p, partial) => confirmCut(cut, p, partial)}
              onCancel={() => {
                cut.resolve({
                  ok: false,
                  title: t("orders.cut.cancelled"),
                  detail: cut.barcode,
                });
                setCut(null);
              }}
            />
          ) : null
        }
        footer={
          <>
            {items.some(isCutItem) ? (
              <View className="flex-row items-center gap-2 rounded-lg bg-card px-3 py-1.5">
                <Text variant="footnote" className="flex-1 font-medium">
                  {t("orders.cut.partial_next")}
                </Text>
                <Switch value={partialNext} onValueChange={setPartialNext} />
              </View>
            ) : null}
            <QtyStepper
              label={
                qtyTouched
                  ? t("m.fixed_qty")
                  : t("orders.assign.fixed_qty_auto")
              }
              value={qty}
              onChange={(n) => {
                setQty(n);
                setQtyTouched(true);
              }}
            />
            {items.some((i) => isRoll(i) && !isCutItem(i)) ? (
              <View className="gap-1.5 rounded-lg bg-card p-2">
                {/* The header hint is cut to two lines; the roll rule is told in full here. */}
                <Text variant="footnote" tone="muted" className="px-2 pt-1">
                  {t("orders.assign.roll_hint")}
                </Text>
                <View className="flex-row items-center gap-2">
                  <Text
                    variant="footnote"
                    tone="muted"
                    className="flex-1 ps-2 font-medium"
                  >
                    {t("orders.assign.meters")}
                  </Text>
                  <TextInput
                    value={meters}
                    onChangeText={setMeters}
                    keyboardType="decimal-pad"
                    placeholder={t("orders.assign.meters_placeholder")}
                    placeholderTextColorClassName="accent-muted-foreground"
                    className="min-h-11 w-28 rounded-md border border-border px-2 text-center text-[17px] text-foreground"
                  />
                </View>
              </View>
            ) : null}
          </>
        }
      />
      <ContinuousScanner
        visible={scanning === "place"}
        onClose={() => {
          setScanning(null);
          void unplaced.refetch();
        }}
        title={t("order_place.title")}
        hint={t("order_place.scan_location_hint")}
        onCode={onPlace}
        footer={
          <View className="gap-2 rounded-lg bg-card p-3">
            <Text variant="callout" className="font-semibold">
              {target
                ? t("m.target_bin", { location: target.code })
                : t("order_place.no_target")}
            </Text>
            {target && unplaced.data?.unplaced_count ? (
              <Button
                variant="secondary"
                title={t("m.place_rest", {
                  count: unplaced.data.unplaced_count,
                })}
                loading={busy === "place_all"}
                onPress={() =>
                  void run("place_all", async () => {
                    await orderApi.place(uuid, { location_uuid: target.uuid });
                    await unplaced.refetch();
                  })
                }
              />
            ) : null}
          </View>
        }
      />
      {suggestFor ? (
        <SuggestSheet
          order={uuid}
          item={suggestFor}
          onClose={() => setSuggestFor(null)}
          onApplied={(next) => {
            store(next);
            setSuggestFor(null);
          }}
        />
      ) : null}
      <CancelSheet
        key={cancelling?.kind ?? "none"}
        order={o}
        kind={cancelling}
        busy={busy === "cancel"}
        onClose={() => setCancelling(null)}
        onSubmit={(status, reason) =>
          void run(
            "cancel",
            () => orderApi.transition(uuid, { status, reason }),
            {
              done: () => {
                setCancelling(null);
                void qc.invalidateQueries({
                  queryKey: ["warehouse", "orders"],
                });
              },
            },
          )
        }
      />
      <ShipSheet
        visible={shipping}
        busy={busy === "ship"}
        onClose={() => setShipping(false)}
        onShip={(extra) => transition("shipped", "ship", extra)}
      />
      {checking && o ? (
        <ReceiptCheckSheet
          order={o}
          onClose={() => setChecking(false)}
          onDone={(next) => {
            store(next);
            void qc.invalidateQueries({ queryKey: ["warehouse", "orders"] });
          }}
        />
      ) : null}
    </>
  );
}

function SuggestSheet({
  order,
  item,
  onClose,
  onApplied,
}: {
  order: string;
  item: OrderItem;
  onClose: () => void;
  onApplied: (o: Order) => void;
}) {
  const { t } = useTranslation("warehouse");
  const [unlocated, setUnlocated] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const q = useSuggestions(order, item.uuid, unlocated);
  const s = q.data;

  const apply = async () => {
    if (!s?.suggestions.length) return;
    setBusy(true);
    setError(null);
    try {
      onApplied(
        await orderApi.applySuggestions(order, item.uuid, {
          units: s.suggestions.map((x) => ({
            unit_uuid: x.unit_uuid,
            quantity:
              x.unit_kind === "fixed" && x.quantity ? x.quantity : undefined,
            meters: x.meters ?? undefined,
            // TEC-1241: a proposed closed box is opened on apply.
            open_box: x.open_box || undefined,
          })),
        }),
      );
    } catch (e) {
      setError(whError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      visible
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      <View className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={1}>
            {t("orders.suggest.title")}
          </Text>
          <Button title={t("common:close")} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-3 px-4 pb-10">
          <Text variant="headline">{item.product.name}</Text>
          <Text variant="footnote" tone="muted">
            {t("orders.suggest.rule")}
          </Text>
          <View className="flex-row items-center justify-between gap-3">
            <Text variant="callout" className="flex-1">
              {t("orders.suggest.include_unlocated")}
            </Text>
            <Switch value={unlocated} onValueChange={setUnlocated} />
          </View>
          {q.isLoading ? (
            <ListSkeleton rows={3} />
          ) : !s ? (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          ) : s.suggestions.length === 0 ? (
            <Text variant="callout" tone="muted">
              {t("orders.suggest.empty")}
            </Text>
          ) : (
            <>
              <Text
                variant="callout"
                className={s.complete ? "" : "text-destructive"}
              >
                {t("orders.suggest.coverage", {
                  covered: s.covered,
                  missing: s.missing,
                  unit: isRoll(item)
                    ? t("orders.unit.m")
                    : t("orders.unit.pcs"),
                })}
                {s.complete ? "" : ` · ${t("orders.suggest.incomplete")}`}
              </Text>
              <View className="gap-2">
                {s.suggestions.map((x) => (
                  <View
                    key={x.unit_uuid}
                    className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-2.5"
                  >
                    <Text
                      variant="callout"
                      className="w-20 font-mono font-semibold"
                      numberOfLines={1}
                    >
                      {x.location_code ?? t("orders.picking.no_location")}
                    </Text>
                    <Text
                      variant="footnote"
                      className="flex-1 font-mono"
                      numberOfLines={1}
                    >
                      {x.barcode}
                    </Text>
                    <View className="items-end">
                      <Text variant="callout">
                        {x.meters
                          ? `${formatNumber(x.meters, 2)} m`
                          : `× ${formatNumber(x.quantity)}`}
                      </Text>
                      {x.open_box ? (
                        <Text variant="footnote" tone="danger">
                          {t("orders.cut.suggest_open_box", {
                            box: formatNumber(x.remaining_meters, 2),
                          })}
                        </Text>
                      ) : null}
                      {excessOf(x) > 0 ? (
                        <Text variant="footnote" tone="muted">
                          {t("orders.cut.suggest_whole", {
                            excess: formatNumber(x.excess_meters, 2),
                          })}
                        </Text>
                      ) : null}
                    </View>
                  </View>
                ))}
              </View>
              {error ? (
                <Text variant="footnote" tone="danger">
                  {error}
                </Text>
              ) : null}
              <Button
                size="lg"
                title={t("orders.suggest.apply")}
                loading={busy}
                onPress={() => void apply()}
              />
            </>
          )}
        </ScrollView>
      </View>
    </Modal>
  );
}

function ShipSheet({
  visible,
  busy,
  onClose,
  onShip,
}: {
  visible: boolean;
  busy: boolean;
  onClose: () => void;
  onShip: (extra: ShipExtra) => void;
}) {
  const { t } = useTranslation("warehouse");
  const methods = useDeliveryMethods(visible);
  const [method, setMethod] = useState<string | undefined>(undefined);
  const [tracking, setTracking] = useState("");
  // TEC-1228: package count, desi and weight, printed on the shipping note.
  const [packages, setPackages] = useState("");
  const [desi, setDesi] = useState("");
  const [weight, setWeight] = useState("");
  const details = parseShipment(packages, desi, weight);
  return (
    <Modal
      visible={visible}
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      <View className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title">{t("orders.shipping.title")}</Text>
          <Button title={t("common:close")} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView
          contentContainerClassName="gap-4 px-4 pb-10"
          keyboardShouldPersistTaps="handled"
        >
          <Text variant="callout" tone="muted">
            {t("orders.actions.ship.description")}
          </Text>
          {(methods.data?.length ?? 0) > 0 ? (
            <View className="gap-2">
              <Text variant="footnote" tone="muted" className="font-medium">
                {t("orders.shipping.method")}
              </Text>
              {methods.data?.map((m) => (
                <Pressable
                  key={m.uuid}
                  accessibilityRole="radio"
                  accessibilityState={{ checked: method === m.uuid }}
                  onPress={() =>
                    setMethod(method === m.uuid ? undefined : m.uuid)
                  }
                  className={`min-h-12 flex-row items-center rounded-lg border px-4 ${method === m.uuid ? "border-primary bg-secondary" : "border-border bg-card"}`}
                >
                  <Text variant="body" className="flex-1">
                    {m.name}
                  </Text>
                  {method === m.uuid ? (
                    <Icon name="truck" tone="primary" size={18} />
                  ) : null}
                </Pressable>
              ))}
            </View>
          ) : null}
          <Input
            label={t("orders.shipping.tracking_no")}
            placeholder={t("orders.shipping.tracking_placeholder")}
            value={tracking}
            onChangeText={setTracking}
            autoCapitalize="characters"
            autoCorrect={false}
          />
          <View className="flex-row gap-2">
            <View className="flex-1">
              <Input
                label={t("shipment.package_count")}
                placeholder={t("shipment.package_count_placeholder")}
                value={packages}
                onChangeText={setPackages}
                keyboardType="number-pad"
              />
            </View>
            <View className="flex-1">
              <Input
                label={t("shipment.desi")}
                placeholder={t("shipment.desi_placeholder")}
                value={desi}
                onChangeText={setDesi}
                keyboardType="decimal-pad"
              />
            </View>
            <View className="flex-1">
              <Input
                label={t("shipment.weight_kg")}
                placeholder={t("shipment.weight_kg_placeholder")}
                value={weight}
                onChangeText={setWeight}
                keyboardType="decimal-pad"
              />
            </View>
          </View>
          {details ? null : (
            <Text variant="footnote" tone="danger">
              {t("shipment.invalid")}
            </Text>
          )}
          <Text variant="footnote" tone="muted">
            {t("shipment.on_note")}
          </Text>
          <Button
            size="lg"
            title={t("orders.actions.ship.button")}
            loading={busy}
            disabled={!details}
            onPress={() =>
              onShip({
                delivery_method_uuid: method,
                tracking_no: tracking.trim() || undefined,
                ...details,
              })
            }
          />
        </ScrollView>
      </View>
    </Modal>
  );
}
