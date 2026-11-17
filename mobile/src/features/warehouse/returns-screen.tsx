import { useQueryClient } from "@tanstack/react-query";
import * as Haptics from "expo-haptics";
import { Stack } from "expo-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Modal, Pressable, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { useCan } from "@/core/spaces";
import {
  CardRow,
  PagedList,
  useDebounced,
  useFormat,
} from "@/features/admin/kit";
import { Button } from "@/ui/button";
import { Icon } from "@/ui/icon";
import { Input } from "@/ui/input";
import { SectionHeader } from "@/ui/list";
import { Screen } from "@/ui/screen";
import { StatusPill } from "@/ui/status-pill";
import { Text } from "@/ui/text";

import {
  INSPECTION_DECISIONS,
  type InspectionDecision,
  inspectReturn,
  type QuarantineItem,
  RETURN_REASONS,
  type ReturnReason,
  useQuarantine,
} from "./adjust-api";
import { alertError } from "./format";
import { Card, Field } from "./parts";

/**
 * TEC-1226 depot return inspection: received returns wait in quarantine (not
 * counted, not pickable or usable). Back to stock, scrap (a scrap document
 * that waits for approval) or back to the supplier; the reason is recorded.
 */
export function ReturnsScreen() {
  const { t } = useTranslation("adjust");
  const f = useFormat();
  const write = useCan("warehouse.write");
  const [q, setQ] = useState("");
  const query = useDebounced(q.trim());
  const list = useQuarantine(query);
  const [open, setOpen] = useState<QuarantineItem | null>(null);

  return (
    <>
      <Stack.Screen
        options={{
          title: t("returns.title"),
          headerLargeTitle: false,
          headerSearchBarOptions: {
            placeholder: t("returns.search"),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(""),
          },
        }}
      />
      <PagedList
        query={list}
        keyOf={(r) => r.unit.uuid}
        header={
          <Text variant="footnote" tone="muted" className="px-4 pb-1">
            {t("returns.description")}
          </Text>
        }
        empty={{
          icon: "inbox",
          title: t("returns.empty_title"),
          body: t("returns.empty_description"),
        }}
        renderItem={({ item }) => (
          <CardRow
            title={item.unit.barcode}
            subtitle={[
              item.product.name,
              item.remaining_meters ? `${item.remaining_meters} m` : null,
            ]
              .filter(Boolean)
              .join(" · ")}
            meta={[
              item.return?.transfer_no,
              item.return?.from_org_name,
              f.date(item.quarantined_at),
            ]
              .filter(Boolean)
              .join(" · ")}
            trailing={
              <StatusPill
                label={t(
                  item.pending_scrap
                    ? "returns.pending_scrap"
                    : "returns.waiting",
                )}
                tone={item.pending_scrap ? "neutral" : "progress"}
              />
            }
            onPress={
              write && !item.pending_scrap ? () => setOpen(item) : undefined
            }
          />
        )}
      />
      {open ? <InspectSheet item={open} onClose={() => setOpen(null)} /> : null}
    </>
  );
}

function InspectSheet({
  item,
  onClose,
}: {
  item: QuarantineItem;
  onClose: () => void;
}) {
  const { t } = useTranslation("adjust");
  const insets = useSafeAreaInsets();
  const qc = useQueryClient();
  const adjust = useCan("stock.adjust");
  const [decision, setDecision] = useState<InspectionDecision>("restock");
  const [reason, setReason] = useState<ReturnReason | null>(null);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const decisions = INSPECTION_DECISIONS.filter(
    (d) => d !== "supplier_return" || adjust,
  );

  const save = async () => {
    if (!reason) return;
    setBusy(true);
    try {
      await inspectReturn({
        unit_uuid: item.unit.uuid,
        decision,
        return_reason: reason,
        note: note.trim() || null,
      });
      void qc.invalidateQueries({ queryKey: ["warehouse", "quarantine"] });
      void qc.invalidateQueries({ queryKey: ["warehouse", "adjust"] });
      void qc.invalidateQueries({ queryKey: ["stock"] });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      onClose();
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  const choice = (selected: boolean, label: string, onPress: () => void) => (
    <Pressable
      key={label}
      accessibilityRole="radio"
      accessibilityState={{ selected }}
      onPress={onPress}
      className={`rounded-full border px-4 py-2 ${selected ? "border-primary bg-primary" : "border-border bg-card"}`}
    >
      <Text
        variant="callout"
        className={selected ? "text-primary-foreground" : undefined}
      >
        {label}
      </Text>
    </Pressable>
  );

  return (
    <Modal
      visible
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      <View
        className="flex-1 bg-background"
        style={{ paddingBottom: insets.bottom }}
      >
        <View className="flex-row items-center gap-3 px-4 py-3">
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t("common:close")}
            hitSlop={10}
            onPress={onClose}
          >
            <Icon name="close" />
          </Pressable>
          <Text variant="headline" className="flex-1">
            {t("returns.inspect")}
          </Text>
        </View>
        <Screen keyboardShouldPersistTaps="handled">
          <Card>
            <Text variant="headline" className="font-mono">
              {item.unit.barcode}
            </Text>
            <Field label={t("returns.product")} value={item.product.name} />
            <Field
              label={t("returns.return")}
              value={
                item.return
                  ? `${item.return.transfer_no} · ${item.return.from_org_name}`
                  : null
              }
            />
            <Field label={t("fields.note")} value={item.return?.note} />
          </Card>
          <SectionHeader title={t("returns.decision")} />
          <View className="flex-row flex-wrap gap-2">
            {decisions.map((d) =>
              choice(decision === d, t(`returns.decisions.${d}`), () =>
                setDecision(d),
              ),
            )}
          </View>
          <Text variant="footnote" tone="muted">
            {t(`returns.decision_help.${decision}`)}
          </Text>
          <SectionHeader title={t("returns.reason")} />
          <View className="flex-row flex-wrap gap-2">
            {RETURN_REASONS.map((r) =>
              choice(reason === r, t(`returns.reasons.${r}`), () =>
                setReason(r),
              ),
            )}
          </View>
          <Input
            label={t("fields.note")}
            value={note}
            onChangeText={setNote}
            multiline
            maxLength={1000}
          />
          <Button
            title={t("returns.save")}
            size="lg"
            loading={busy}
            disabled={!reason}
            onPress={() => void save()}
          />
        </Screen>
      </View>
    </Modal>
  );
}
