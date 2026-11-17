import { CameraView, useCameraPermissions } from 'expo-camera';
import { Image } from 'expo-image';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Linking, Modal, Pressable, ScrollView, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { alertError, confirmAction, formatDate } from './format';
import type { Order, OrderStatus } from './orders-api';
import { Card, Field } from './parts';
import { canShareDocument, sharePanelDocument } from './pdf-share';

type Run = (name: string, fn: () => Promise<Order | void>, opts?: { done?: () => void }) => Promise<void>;

/** A YYYY-MM-DD due date in the app language. */
export function dueText(due: string) {
  return formatDate(`${due}T12:00:00`, false);
}

/**
 * TEC-1228: short queue facts of an order for list cards — urgent, due date
 * (overdue flagged) and who took the job. Nothing about prices.
 */
export function useQueueFacts() {
  const { t } = useTranslation('warehouse');
  return (o: Order) =>
    [
      o.priority === 'urgent' ? t('queue.urgent') : null,
      o.due_date
        ? t(o.is_overdue ? 'queue.overdue' : 'queue.due', {
            date: dueText(o.due_date),
          })
        : null,
      o.picker ? (o.claimed_by_me ? t('queue.claimed_by_me') : t('queue.claimed_by', { name: o.picker.name })) : null,
    ].filter((x): x is string => !!x);
}

/** TEC-1228: priority, due date and the job holder with take / take over / release. */
export function WorkCard({
  order: o,
  busy,
  onClaim,
  onRelease,
}: {
  order: Order;
  busy: string | null;
  onClaim: (takeover: boolean) => void;
  onRelease: () => void;
}) {
  const { t } = useTranslation('warehouse');
  const heldByOther = o.picker != null && !o.claimed_by_me;
  const showJob = o.can_claim || o.picker != null;
  if (!showJob && o.priority !== 'urgent' && !o.due_date) return null;
  return (
    <Card>
      <View className="flex-row items-center gap-2 pb-1">
        <Icon name="clock" tone={o.is_overdue || o.priority === 'urgent' ? 'destructive' : 'primary'} size={18} />
        <Text variant="headline" className="flex-1">
          {t('queue.title')}
        </Text>
      </View>
      <Field label={t('queue.priority')} value={t(`queue.priority_${o.priority}`)} />
      <Field label={t('queue.due_date')} value={o.due_date ? `${dueText(o.due_date)}${o.is_overdue ? ` · ${t('queue.overdue_short')}` : ''}` : null} />
      {showJob ? (
        <Field label={t('queue.picker')} value={o.picker ? `${o.picker.name}${o.claimed_at ? ` · ${formatDate(o.claimed_at)}` : ''}` : t('queue.unclaimed')} />
      ) : null}
      {heldByOther && o.can_claim ? (
        <Text variant="footnote" tone="muted" className="pt-1">
          {t('queue.held_warning', { name: o.picker?.name ?? '' })}
        </Text>
      ) : null}
      <View className="gap-2 pt-2">
        {o.can_claim && o.picker == null ? <Button title={t('queue.claim')} icon="check" loading={busy === 'claim'} onPress={() => onClaim(false)} /> : null}
        {heldByOther && o.can_take_over ? (
          <Button
            variant="outline"
            title={t('queue.take_over')}
            loading={busy === 'claim'}
            onPress={() =>
              confirmAction({
                title: t('queue.take_over_title', {
                  name: o.picker?.name ?? '',
                }),
                action: t('queue.take_over'),
                onConfirm: () => onClaim(true),
              })
            }
          />
        ) : null}
        {o.picker != null && (o.claimed_by_me || o.can_take_over) ? (
          <Button variant="ghost" title={t('queue.release')} loading={busy === 'release'} onPress={onRelease} />
        ) : null}
      </View>
    </Card>
  );
}

const SHIPPED: OrderStatus[] = ['shipped', 'delivered', 'received', 'cancelling'];

/**
 * TEC-1228: order documents on mobile — the shipping note (share, render
 * again), the order slip and the buyer's delivery receipt (share, take a
 * photo and upload). Prices stay as the server renders them: the warehouse
 * roles get them hidden on the slip and the shipping note never has any.
 */
export function DocumentsCard({
  order: o,
  canShip,
  canReceive,
  busy,
  run,
  regenerate,
  attachReceipt,
}: {
  order: Order;
  canShip: boolean;
  canReceive: boolean;
  busy: string | null;
  run: Run;
  regenerate: () => Promise<Order>;
  attachReceipt: (uri: string) => Promise<Order>;
}) {
  const { t } = useTranslation('warehouse');
  const [sharing, setSharing] = useState<string | null>(null);
  const [camera, setCamera] = useState(false);
  const seller = o.role === 'seller';
  const shipped = SHIPPED.includes(o.status);
  const canUpload = !seller && canReceive && (o.status === 'shipped' || o.status === 'delivered' || o.status === 'received');
  const share = async (key: string, path: string, title: string) => {
    if (sharing) return;
    setSharing(key);
    try {
      await sharePanelDocument(path, title);
    } catch (e) {
      alertError(e);
    } finally {
      setSharing(null);
    }
  };
  const base = `/v1/orders/${encodeURIComponent(o.uuid)}`;
  const items: React.ReactNode[] = [];
  if (canShareDocument()) {
    if (o.has_shipping_document) {
      items.push(
        <Button
          key="note"
          variant="outline"
          icon="share"
          title={t('documents.share_shipping_note')}
          loading={sharing === 'note'}
          onPress={() => void share('note', `${base}/documents/shipping`, `${t('documents.shipping_note')} ${o.order_no}`)}
        />,
      );
    }
    if (seller) {
      items.push(
        <Button
          key="slip"
          variant="outline"
          icon="share"
          title={t('documents.share_order_slip')}
          loading={sharing === 'slip'}
          onPress={() => void share('slip', `${base}/order-slip.pdf`, `${t('documents.order_slip')} ${o.order_no}`)}
        />,
      );
    }
    if (o.has_receipt_document) {
      items.push(
        <Button
          key="receipt"
          variant="outline"
          icon="share"
          title={t('documents.share_receipt')}
          loading={sharing === 'receipt'}
          onPress={() => void share('receipt', `${base}/documents/receipt`, `${t('documents.receipt')} ${o.order_no}`)}
        />,
      );
    }
  }
  if (seller && canShip && shipped) {
    items.push(
      <Button
        key="regen"
        variant="ghost"
        icon="undo"
        title={o.has_shipping_document ? t('documents.regenerate') : t('documents.generate')}
        loading={busy === 'regenerate'}
        onPress={() => void run('regenerate', regenerate)}
      />,
    );
  }
  if (canUpload) {
    items.push(
      <Button
        key="photo"
        variant={o.has_receipt_document ? 'ghost' : 'secondary'}
        icon="camera"
        title={o.has_receipt_document ? t('documents.replace_receipt') : t('documents.add_receipt')}
        loading={busy === 'receipt'}
        onPress={() => setCamera(true)}
      />,
    );
  }
  if (!items.length && o.package_count == null) return null;
  return (
    <Card>
      <View className="flex-row items-center gap-2 pb-1">
        <Icon name="report" tone="primary" size={18} />
        <Text variant="headline" className="flex-1">
          {t('documents.title')}
        </Text>
      </View>
      {/* Shipment details entered at shipping (printed on the shipping note). */}
      <Field label={t('shipment.package_count')} value={o.package_count != null ? String(o.package_count) : null} />
      <Field label={t('shipment.desi')} value={o.desi} />
      <Field label={t('shipment.weight_kg')} value={o.weight_kg} />
      <View className="gap-2 pt-2">{items}</View>
      {!canShareDocument() ? (
        <Text variant="footnote" tone="muted" className="pt-1">
          {t('documents.share_unavailable')}
        </Text>
      ) : null}
      {camera ? (
        <ReceiptCapture
          onClose={() => setCamera(false)}
          onUse={(uri) =>
            void run('receipt', () => attachReceipt(uri), {
              done: () => setCamera(false),
            })
          }
          uploading={busy === 'receipt'}
        />
      ) : null}
    </Card>
  );
}

/** Full-screen camera for the signed delivery receipt: take, review, use. */
function ReceiptCapture({ onClose, onUse, uploading }: { onClose: () => void; onUse: (uri: string) => void; uploading: boolean }) {
  const { t } = useTranslation('warehouse');
  const insets = useSafeAreaInsets();
  const camera = useRef<CameraView>(null);
  const [permission, request] = useCameraPermissions();
  const [shot, setShot] = useState<string | null>(null);
  const [taking, setTaking] = useState(false);
  const [ready, setReady] = useState(false);
  const take = async () => {
    if (!camera.current || taking) return;
    setTaking(true);
    try {
      const pic = await camera.current.takePictureAsync({ quality: 0.7 });
      if (pic?.uri) setShot(pic.uri);
    } catch (e) {
      alertError(e);
    } finally {
      setTaking(false);
    }
  };
  return (
    <Modal
      visible
      animationType="slide"
      presentationStyle="fullScreen"
      onRequestClose={() => {
        if (!uploading) onClose();
      }}>
      <View className="flex-1 bg-black" style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}>
        <View className="flex-row items-center gap-3 px-4 py-3">
          <Pressable accessibilityRole="button" accessibilityLabel={t('common:close')} hitSlop={10} disabled={uploading} onPress={onClose}>
            <Icon name="close" tone="primary-foreground" />
          </Pressable>
          <Text variant="headline" className="flex-1 text-white">
            {t('documents.add_receipt')}
          </Text>
        </View>
        <View className="flex-1 overflow-hidden">
          {shot ? (
            <Image source={{ uri: shot }} style={{ flex: 1 }} contentFit="contain" />
          ) : !permission ? null : !permission.granted ? (
            <View className="flex-1 items-center justify-center gap-3 px-8">
              <Text variant="body" className="text-center text-white">
                {t('documents.camera_needed')}
              </Text>
              <Button
                title={permission.canAskAgain ? t('documents.camera_allow') : t('documents.open_settings')}
                onPress={() => void (permission.canAskAgain ? request() : Linking.openSettings())}
              />
            </View>
          ) : (
            <CameraView ref={camera} style={{ flex: 1 }} facing="back" onCameraReady={() => setReady(true)} />
          )}
        </View>
        <View className="flex-row items-center justify-center gap-4 px-4 py-5">
          {shot ? (
            <>
              <Button title={t('documents.retake')} variant="secondary" onPress={() => setShot(null)} disabled={uploading} className="grow" />
              <Button title={t('documents.use_photo')} icon="addPhoto" loading={uploading} onPress={() => onUse(shot)} className="grow" />
            </>
          ) : permission?.granted ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('documents.take_photo')}
              disabled={!ready || taking}
              onPress={() => void take()}
              className="size-[72px] items-center justify-center rounded-full border-4 border-white active:opacity-70">
              {taking ? <ActivityIndicator colorClassName="accent-on-media" /> : <View className="size-14 rounded-full bg-white" />}
            </Pressable>
          ) : null}
        </View>
      </View>
    </Modal>
  );
}

/** The cancel moves the server offers the caller (same rules as the web). */
export type CancelKind = 'cancel' | 'request_cancel' | 'complete_cancel';

export function cancelActions(o: Order): { kind: CancelKind; status: OrderStatus; reasonRequired: boolean }[] {
  const out: {
    kind: CancelKind;
    status: OrderStatus;
    reasonRequired: boolean;
  }[] = [];
  if (o.available_transitions.includes('cancelling'))
    out.push({
      kind: 'request_cancel',
      status: 'cancelling',
      reasonRequired: true,
    });
  if (o.available_transitions.includes('cancelled')) {
    out.push(
      o.status === 'cancelling'
        ? {
            kind: 'complete_cancel',
            status: 'cancelled',
            reasonRequired: false,
          }
        : { kind: 'cancel', status: 'cancelled', reasonRequired: true },
    );
  }
  return out;
}

/**
 * TEC-1228: cancel, request a cancel and "cancelling → cancelled (the goods
 * came back)" on mobile. available_transitions carries the same permission
 * rules as the web; a reason is asked where the web asks for one.
 */
export function CancelSheet({
  order: o,
  kind,
  busy,
  onClose,
  onSubmit,
}: {
  order: Order;
  kind: {
    kind: CancelKind;
    status: OrderStatus;
    reasonRequired: boolean;
  } | null;
  busy: boolean;
  onClose: () => void;
  onSubmit: (status: OrderStatus, reason: string | undefined) => void;
}) {
  const { t } = useTranslation('warehouse');
  const [reason, setReason] = useState('');
  if (!kind) return null;
  const ok = !kind.reasonRequired || reason.trim().length >= 3;
  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <View className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={2}>
            {t(`orders.actions.${kind.kind}.title`)}
          </Text>
          <Button title={t('common:close')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-4 px-4 pb-10" keyboardShouldPersistTaps="handled">
          <Text variant="callout" tone="muted">
            {o.order_no} · {t(`orders.actions.${kind.kind}.description`)}
          </Text>
          <Input
            label={kind.reasonRequired ? t('orders.actions.reason_required') : t('orders.actions.reason_optional')}
            value={reason}
            onChangeText={setReason}
            multiline
            maxLength={500}
          />
          <Button
            size="lg"
            variant={kind.kind === 'complete_cancel' ? 'primary' : 'destructive'}
            title={t(`orders.actions.${kind.kind}.confirm`)}
            disabled={!ok}
            loading={busy}
            onPress={() => onSubmit(kind.status, reason.trim() || undefined)}
          />
        </ScrollView>
      </View>
    </Modal>
  );
}
