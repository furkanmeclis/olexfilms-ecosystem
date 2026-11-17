import { useQueryClient } from "@tanstack/react-query";
import { CameraView, useCameraPermissions } from "expo-camera";
import * as Haptics from "expo-haptics";
import { Image } from "expo-image";
import { router, Stack, useLocalSearchParams } from "expo-router";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ActivityIndicator,
  Alert,
  Linking,
  Modal,
  Pressable,
  View,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { useCan } from "@/core/spaces";
import {
  CardRow,
  Chips,
  ErrorState,
  ListSkeleton,
  PagedList,
  useChildHref,
  useDebounced,
  useFormat,
} from "@/features/admin/kit";
import { Button } from "@/ui/button";
import { Icon } from "@/ui/icon";
import { Input } from "@/ui/input";
import { SectionHeader } from "@/ui/list";
import { PullRefresh } from "@/ui/pull-refresh";
import { Screen } from "@/ui/screen";
import { StatusPill } from "@/ui/status-pill";
import { Text } from "@/ui/text";

import {
  ADJUSTMENT_TYPES,
  type AdjustFilter,
  type Adjustment,
  type AdjustmentType,
  addAdjustmentLine,
  adjustKeys,
  adjustTone,
  adjustmentAction,
  createAdjustment,
  deleteAdjustmentLine,
  needsPhoto,
  normalizeMeters,
  uploadAdjustmentPhoto,
  useAdjustment,
  useAdjustments,
} from "./adjust-api";
import { ContinuousScanner, type ScanOutcome } from "./continuous-scanner";
import { alertError, confirmAction, whError } from "./format";
import { Card, Field, IconButton } from "./parts";

/** TEC-1226: stock adjustment documents (waste, damage, scrap, loss, found, other). No price. */
export function AdjustListScreen() {
  const { t } = useTranslation("adjust");
  const f = useFormat();
  const href = useChildHref();
  const write = useCan("warehouse.write");
  const [filter, setFilter] = useState<AdjustFilter>("open");
  const [q, setQ] = useState("");
  const query = useDebounced(q.trim());
  const list = useAdjustments(filter, query);

  return (
    <>
      <Stack.Screen
        options={{
          title: t("title"),
          headerLargeTitle: false,
          headerSearchBarOptions: {
            placeholder: t("search"),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(""),
          },
          headerRight: write
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t("new")}
                  hitSlop={8}
                  onPress={() => router.push(href("new"))}
                  className="size-8 items-center justify-center"
                >
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <PagedList
        query={list}
        keyOf={(a) => a.uuid}
        header={
          <Chips
            options={(["open", "posted", "all"] as AdjustFilter[]).map((k) => ({
              key: k,
              label: t(`filter.${k}`),
            }))}
            value={filter}
            onChange={setFilter}
          />
        }
        empty={{
          icon: "edit",
          title: query ? t("no_results") : t("empty_title"),
          body: query ? undefined : t("empty_description"),
        }}
        renderItem={({ item }) => (
          <CardRow
            title={item.adjustment_no}
            subtitle={`${t(`type.${item.type}`)} · ${t("totals", { pieces: item.total_quantity, meters: item.total_meters })}`}
            meta={f.date(item.created_at)}
            trailing={
              <StatusPill
                label={t(`status.${item.status}`)}
                tone={adjustTone(item.status)}
              />
            }
            onPress={() => router.push(href(item.uuid))}
          />
        )}
      />
    </>
  );
}

/** New draft: pick a type; lines and photos follow on the document. */
export function AdjustNewScreen() {
  const { t } = useTranslation("adjust");
  const qc = useQueryClient();
  const [type, setType] = useState<AdjustmentType>("damage");
  const [busy, setBusy] = useState(false);

  const create = async () => {
    setBusy(true);
    try {
      const a = await createAdjustment(type);
      void qc.invalidateQueries({ queryKey: ["warehouse", "adjust", "list"] });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.replace(`/depo/adjust/${a.uuid}` as never);
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Stack.Screen options={{ title: t("new"), headerLargeTitle: false }} />
      <Screen>
        <Text variant="callout" tone="muted">
          {t("new_description")}
        </Text>
        <SectionHeader title={t("fields.type")} />
        <View className="flex-row flex-wrap gap-2">
          {ADJUSTMENT_TYPES.map((k) => (
            <Pressable
              key={k}
              accessibilityRole="radio"
              accessibilityState={{ selected: type === k }}
              onPress={() => setType(k)}
              className={`rounded-full border px-4 py-2 ${type === k ? "border-primary bg-primary" : "border-border bg-card"}`}
            >
              <Text
                variant="callout"
                className={type === k ? "text-primary-foreground" : undefined}
              >
                {t(`type.${k}`)}
              </Text>
            </Pressable>
          ))}
        </View>
        <Text variant="footnote" tone="muted">
          {t(`type_help.${type}`)}
        </Text>
        <Button
          title={t("create")}
          size="lg"
          loading={busy}
          onPress={() => void create()}
        />
      </Screen>
    </>
  );
}

export function AdjustDetailScreen() {
  const { t } = useTranslation("adjust");
  const f = useFormat();
  const qc = useQueryClient();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const write = useCan("warehouse.write");
  const q = useAdjustment(uuid);
  const a = q.data;
  const [busy, setBusy] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [camera, setCamera] = useState(false);
  const [barcode, setBarcode] = useState("");
  const [meters, setMeters] = useState("");
  const [quantity, setQuantity] = useState("");

  const apply = (next: Adjustment) => {
    qc.setQueryData(adjustKeys.one(uuid), next);
    void qc.invalidateQueries({ queryKey: ["warehouse", "adjust", "list"] });
  };

  const run = async (action: "submit" | "approve" | "cancel") => {
    setBusy(action);
    try {
      apply(await adjustmentAction(uuid, action));
      void qc.invalidateQueries({ queryKey: ["stock"] });
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(null);
    }
  };

  const add = async (code: string): Promise<boolean> => {
    const m = meters.trim() ? normalizeMeters(meters) : undefined;
    if (meters.trim() && !m) {
      Alert.alert(t("meters_invalid"));
      return false;
    }
    try {
      apply(
        await addAdjustmentLine(
          uuid,
          code.trim(),
          m,
          quantity.trim() ? Number(quantity) : undefined,
        ),
      );
      setBarcode("");
      setMeters("");
      setQuantity("");
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      return true;
    } catch (e) {
      alertError(e);
      return false;
    }
  };

  const onScan = async (code: string): Promise<ScanOutcome> => {
    try {
      apply(await addAdjustmentLine(uuid, code.trim()));
      return { ok: true, title: code };
    } catch (e) {
      return { ok: false, title: code, detail: whError(e) };
    }
  };

  if (q.isPending) return <ListSkeleton />;
  if (q.error || !a)
    return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;

  const draft = a.status === "draft";
  const open = draft || a.status === "pending_approval";

  return (
    <>
      <Stack.Screen
        options={{ title: a.adjustment_no, headerLargeTitle: false }}
      />
      <Screen
        keyboardShouldPersistTaps="handled"
        refreshControl={
          <PullRefresh
            busy={q.isRefetching}
            onRefresh={() => void q.refetch()}
          />
        }
      >
        <Card>
          <View className="flex-row items-center justify-between gap-2">
            <Text variant="headline">{t(`type.${a.type}`)}</Text>
            <StatusPill
              label={t(`status.${a.status}`)}
              tone={adjustTone(a.status)}
            />
          </View>
          <Field label={t("fields.source")} value={t(`source.${a.source}`)} />
          <Field
            label={t("fields.totals")}
            value={t("totals", {
              pieces: a.total_quantity,
              meters: a.total_meters,
            })}
          />
          <Field label={t("fields.reason_code")} value={a.reason_code} />
          <Field label={t("fields.note")} value={a.note} />
          <Field label={t("fields.created_by")} value={a.created_by?.name} />
          <Field label={t("fields.approved_by")} value={a.approved_by?.name} />
          <Field label={t("fields.created_at")} value={f.date(a.created_at)} />
        </Card>

        {a.status === "pending_approval" ? (
          <Text variant="footnote" tone="muted">
            {a.requires_second_approval
              ? t("hint.second_approval")
              : t("hint.single_approval")}
          </Text>
        ) : null}
        {draft && needsPhoto(a) ? (
          <Text variant="footnote" tone="danger">
            {t("hint.photo_required")}
          </Text>
        ) : null}

        <SectionHeader title={t("lines.title")} />
        {draft && write ? (
          <View className="gap-2.5">
            <Button
              title={t("lines.scan")}
              icon="scan"
              variant="secondary"
              onPress={() => setScanning(true)}
            />
            <Input
              label={t("lines.barcode")}
              icon="barcode"
              value={barcode}
              onChangeText={setBarcode}
              autoCapitalize="characters"
              autoCorrect={false}
            />
            <View className="flex-row gap-2.5">
              <View className="flex-1">
                <Input
                  label={t("lines.meters")}
                  value={meters}
                  onChangeText={setMeters}
                  keyboardType="decimal-pad"
                />
              </View>
              <View className="flex-1">
                <Input
                  label={t("lines.quantity")}
                  value={quantity}
                  onChangeText={setQuantity}
                  keyboardType="number-pad"
                />
              </View>
            </View>
            <Text variant="footnote" tone="muted">
              {a.type === "found" ? t("lines.help_found") : t("lines.help")}
            </Text>
            <Button
              title={t("lines.add")}
              disabled={!barcode.trim()}
              onPress={() => void add(barcode)}
            />
          </View>
        ) : null}
        {(a.lines ?? []).length === 0 ? (
          <Text variant="callout" tone="muted">
            {t("lines.empty")}
          </Text>
        ) : (
          <View className="gap-2">
            {(a.lines ?? []).map((l) => (
              <Card key={l.uuid}>
                <View className="flex-row items-center justify-between gap-2">
                  <View className="flex-1 gap-0.5">
                    <Text variant="headline" className="font-mono">
                      {l.unit.barcode}
                    </Text>
                    <Text variant="footnote" tone="muted">
                      {l.product.name}
                    </Text>
                    <Text variant="footnote">
                      {l.kind === "meters"
                        ? t("lines.meters_value", { meters: l.meters ?? "" })
                        : l.kind === "quantity"
                          ? t("lines.quantity_value", {
                              quantity: l.quantity ?? 0,
                            })
                          : l.unit.is_roll
                            ? t("lines.whole_roll", {
                                meters: l.roll_meters ?? "",
                              })
                            : t("lines.whole")}
                    </Text>
                  </View>
                  {draft && write ? (
                    <IconButton
                      icon="delete"
                      label={t("common:delete")}
                      onPress={() =>
                        void deleteAdjustmentLine(uuid, l.uuid)
                          .then(apply)
                          .catch(alertError)
                      }
                    />
                  ) : null}
                </View>
              </Card>
            ))}
          </View>
        )}

        <SectionHeader
          title={t("photos.title", { count: a.photos?.length ?? 0 })}
        />
        {open && write ? (
          <Button
            title={t("photos.add")}
            icon="camera"
            variant="secondary"
            onPress={() => setCamera(true)}
          />
        ) : null}

        {write ? (
          <View className="gap-2.5 pt-2">
            {draft ? (
              <Button
                title={t("actions.submit")}
                size="lg"
                loading={busy === "submit"}
                disabled={(a.lines ?? []).length === 0}
                onPress={() => void run("submit")}
              />
            ) : null}
            {a.status === "pending_approval" && a.can_approve ? (
              <Button
                title={t("actions.approve")}
                size="lg"
                loading={busy === "approve"}
                onPress={() => void run("approve")}
              />
            ) : null}
            {open ? (
              <Button
                title={t("actions.cancel")}
                variant="secondary"
                loading={busy === "cancel"}
                onPress={() =>
                  confirmAction({
                    title: t("cancel_title"),
                    body: t("cancel_description"),
                    action: t("actions.cancel"),
                    destructive: true,
                    onConfirm: () => void run("cancel"),
                  })
                }
              />
            ) : null}
          </View>
        ) : null}
      </Screen>
      <ContinuousScanner
        visible={scanning}
        onClose={() => setScanning(false)}
        title={t("lines.scan")}
        hint={t("lines.scan_hint")}
        onCode={onScan}
      />
      {camera ? (
        <AdjustPhotoCapture
          uuid={uuid}
          onDone={apply}
          onClose={() => setCamera(false)}
        />
      ) : null}
    </>
  );
}

/** Full-screen camera for one adjustment photo: take, review, upload (JPEG). */
function AdjustPhotoCapture({
  uuid,
  onDone,
  onClose,
}: {
  uuid: string;
  onDone: (a: Adjustment) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation("adjust");
  const insets = useSafeAreaInsets();
  const ref = useRef<CameraView>(null);
  const [permission, request] = useCameraPermissions();
  const [shot, setShot] = useState<string | null>(null);
  const [taking, setTaking] = useState(false);
  const [ready, setReady] = useState(false);
  const [uploading, setUploading] = useState(false);

  const take = async () => {
    if (!ref.current || taking) return;
    setTaking(true);
    try {
      const pic = await ref.current.takePictureAsync({ quality: 0.7 });
      if (pic?.uri) setShot(pic.uri);
    } catch {
      Alert.alert(t("photos.camera_failed"));
    } finally {
      setTaking(false);
    }
  };

  const upload = async () => {
    if (!shot) return;
    setUploading(true);
    try {
      onDone(await uploadAdjustmentPhoto(uuid, shot));
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      onClose();
    } catch (e) {
      alertError(e);
    } finally {
      setUploading(false);
    }
  };

  return (
    <Modal
      visible
      animationType="slide"
      presentationStyle="fullScreen"
      onRequestClose={() => (uploading ? undefined : onClose())}
    >
      <View
        className="flex-1 bg-black"
        style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}
      >
        <View className="flex-row items-center gap-3 px-4 py-3">
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t("common:close")}
            hitSlop={10}
            disabled={uploading}
            onPress={onClose}
          >
            <Icon name="close" tone="primary-foreground" />
          </Pressable>
          <Text variant="headline" className="flex-1 text-white">
            {t("photos.add")}
          </Text>
        </View>
        <View className="flex-1 overflow-hidden">
          {shot ? (
            <Image
              source={{ uri: shot }}
              style={{ flex: 1 }}
              contentFit="contain"
            />
          ) : !permission ? null : !permission.granted ? (
            <View className="flex-1 items-center justify-center gap-3 px-8">
              <Text variant="body" className="text-center text-white">
                {t("photos.camera_needed")}
              </Text>
              <Button
                title={
                  permission.canAskAgain
                    ? t("photos.camera_allow")
                    : t("photos.open_settings")
                }
                onPress={() =>
                  void (permission.canAskAgain
                    ? request()
                    : Linking.openSettings())
                }
              />
            </View>
          ) : (
            <CameraView
              ref={ref}
              style={{ flex: 1 }}
              facing="back"
              onCameraReady={() => setReady(true)}
            />
          )}
        </View>
        <View className="flex-row items-center justify-center gap-4 px-4 py-5">
          {shot ? (
            <>
              <Button
                title={t("photos.retake")}
                variant="secondary"
                onPress={() => setShot(null)}
                disabled={uploading}
                className="grow"
              />
              <Button
                title={t("photos.use")}
                icon={
                  <Icon name="addPhoto" tone="primary-foreground" size={18} />
                }
                loading={uploading}
                onPress={() => void upload()}
                className="grow"
              />
            </>
          ) : permission?.granted ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t("photos.take")}
              disabled={!ready || taking}
              onPress={() => void take()}
              className="size-[72px] items-center justify-center rounded-full border-4 border-white active:opacity-70"
            >
              {taking ? (
                <ActivityIndicator colorClassName="accent-on-media" />
              ) : (
                <View className="size-14 rounded-full bg-white" />
              )}
            </Pressable>
          ) : null}
        </View>
      </View>
    </Modal>
  );
}
