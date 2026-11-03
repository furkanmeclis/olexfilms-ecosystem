import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, ScrollView, Switch, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { useModule } from '@/core/modules';
import { activeMembership, isCenterWorkplace, useCan } from '@/core/spaces';
import { readJSON, storage, writeJSON } from '@/core/storage';
import { PlateBadge } from '@/features/customer-space/home-parts';
import { EditVehicleSheet } from '@/features/customers/edit-sheets';
import { type PickerOption, PickerSheet } from '@/features/picker-sheet';
import { VehiclePartPicker } from '@/features/vehicle-parts';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { confirmAction } from '@/ui/confirm';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import type { IconName } from '@/ui/icons';
import { useLeaveGuard } from '@/ui/use-leave-guard';

import { customerSubtitle, customerTitle } from './customer-name';
import { IntakePhotos, intakeKey } from './intake-photos';
import { formatNumber, type Service, type ServiceStatus, serviceTone, useLinks, type Vehicle, vehicleLabel } from './lib';
import { CustomerStep, type PickedCustomer, VehicleStep } from './pickers';
import { useService, useServiceWrite } from './service-detail-screen';
import { ServiceHero } from './service-hero';
import { ServiceNoField } from './service-no-field';
import { DetailSkeleton, ErrorState, errorText, Section } from './ui';
import {
  customerStepError,
  defaultFinishChoice,
  finishBlocker,
  finishButtonKey,
  finishChoices,
  type GateCtx,
  initialParts,
  isBlocking,
  isWizardStep,
  leaveBlocker,
  nextStep,
  parseKm,
  previousStep,
  resolveStep,
  servicePatch,
  stepBlocker,
  type StepBlocker,
  summaryError,
  type SummaryForm,
  summaryFromService,
  wizardDirty,
  wizardSteps,
  type WizardStep,
} from './wizard';

/** Why a step is closed, told under the step buttons. */
const BLOCKED_KEY: Record<StepBlocker, string> = {
  service: 'wizard.draft_hint',
  photos: 'wizard.photos_missing',
  parts: 'wizard.needs_parts',
  items: 'wizard.needs_items',
};

const partsKey = (uuid: string) => `service-wizard.parts.v1.${uuid}`;

/** Stored part selection of a draft; storage may be missing or blocked. */
function storedParts(uuid: string): string[] | null {
  try {
    const v = readJSON<unknown>(partsKey(uuid));
    return Array.isArray(v) && v.every((p) => typeof p === 'string') ? v : null;
  } catch {
    return null;
  }
}

/**
 * Service intake wizard in the legacy app's order: customer and vehicle
 * (opens the draft), intake photos (photo standard only), applied parts,
 * products, summary (number, notes, measurement and the status to leave the
 * service in). `/services/new` starts a new draft; `/services/<uuid>/wizard`
 * continues one. Every role uses this one screen: the backend's
 * `available_transitions`, `items_editable` and `service_no_editable` and the
 * session permissions decide what each step offers.
 */
export function NewServiceScreen() {
  const params = useLocalSearchParams<{ uuid?: string; step?: string }>();
  const { t } = useTranslation('dealer');
  const links = useLinks();
  const qc = useQueryClient();
  const canWrite = useCan('services.write');

  const [createdUuid, setCreatedUuid] = useState<string | null>(null);
  const uuid = params.uuid ?? createdUuid ?? '';
  const svc = useService(uuid, !!uuid);
  const s = uuid ? (svc.data ?? null) : null;
  const onWrite = useServiceWrite(uuid);

  // Photo standard gate (TEC-499): the step shows when the module is on and
  // at least one angle applies; a missing required angle locks the later steps.
  const photoModule = useModule('photo_standard');
  const intake = useQuery({
    queryKey: intakeKey(uuid),
    queryFn: () => unwrap(api.GET('/v1/services/{uuid}/intake-photos', { params: { path: { uuid } } })),
    enabled: !!uuid && photoModule,
    retry: false,
  });
  const photosOn = photoModule && !!intake.data?.angles.some((a) => !a.angle.hidden);
  const photosMissing = photosOn && (intake.data?.missing.length ?? 0) > 0;
  const gateLoading = !!uuid && photoModule && intake.isPending;
  const steps = wizardSteps(photosOn);

  // Parts of the parts step: kept per draft on the device, else the items' parts.
  const [partsState, setPartsState] = useState<{ uuid: string; parts: string[] } | null>(null);
  if (s && partsState?.uuid !== s.uuid) {
    setPartsState({ uuid: s.uuid, parts: initialParts(storedParts(s.uuid), s.items) });
  }
  const parts = partsState?.uuid === uuid ? partsState.parts : [];
  const setParts = (next: string[]) => {
    setPartsState({ uuid, parts: next });
    try {
      writeJSON(partsKey(uuid), next);
    } catch {
      // The selection then lives only in memory.
    }
  };

  // Legacy rule (mobile only, every role): a body part before products, a product before the summary.
  const gate = { hasService: !!s, photosMissing, partsCount: parts.length, itemCount: s?.items?.length ?? 0, steps };
  const [wanted, setWanted] = useState<WizardStep>(() =>
    params.uuid ? (isWizardStep(params.step) ? params.step : 'photos') : 'customer',
  );
  const step = resolveStep(wanted, gate);
  const [formError, setStepError] = useState<string | null>(null);
  // Why the user could not move on; shown until the gate is met.
  const [blocked, setBlocked] = useState<StepBlocker | null>(null);
  const stepError = formError ?? (blocked && isBlocking(blocked, gate) ? t(BLOCKED_KEY[blocked]) : null);
  const go = (to: WizardStep | null) => {
    setStepError(null);
    setBlocked(null);
    if (to) setWanted(to);
  };
  /** Next step, or the reason it is closed shown under the buttons. */
  const leave = (from: WizardStep) => {
    const b = leaveBlocker(from, gate);
    if (b) setBlocked(b);
    else go(nextStep(from, steps));
  };

  // First step (before the draft exists).
  const [customer, setCustomer] = useState<PickedCustomer | null>(null);
  const [vehicle, setVehicle] = useState<Vehicle | null>(null);
  const performers = usePerformers();
  const [performer, setPerformer] = useState<string>(OWN);
  const [kmInvalid, setKmInvalid] = useState(false);

  // Fields saved on the summary step; filled once from the service.
  const [form, setForm] = useState<SummaryForm>({ km: '', notes: '', serviceNo: '', measure: false });
  const [formOf, setFormOf] = useState<string | null>(null);
  if (s && formOf !== s.uuid) {
    setFormOf(s.uuid);
    setForm(summaryFromService(s));
  }
  const patchForm = (p: Partial<SummaryForm>) => setForm((f) => ({ ...f, ...p }));

  const [choice, setChoice] = useState<ServiceStatus | null>(null);
  // Completion is never preselected (it deducts stock); the next forward step is.
  const finishTo = s ? (choice ?? defaultFinishChoice(s.status, s.available_transitions)) : null;
  // Vehicle without brand / model tapped on the first step: fixed in the edit sheet.
  const [fixing, setFixing] = useState<Vehicle | null>(null);

  // Typed or picked values not saved yet: leaving asks first (saving does not).
  const releaseLeave = useLeaveGuard(wizardDirty({ service: s, form, picked: !!customer || performer !== OWN }));
  const performerName = performer === OWN ? null : (performers.dealers.find((d) => d.uuid === performer)?.name ?? null);

  const create = useMutation({
    mutationFn: () => {
      const km = parseKm(form.km);
      return unwrap(
        api.POST('/v1/services', {
          body: {
            customer_uuid: customer!.uuid,
            vehicle_uuid: vehicle!.uuid,
            km: typeof km === 'number' ? km : undefined,
            // Asked on the summary step (it needs the VIN snapshot of the service).
            has_measurement: false,
            // TEC-1003: the center may open the service for a dealer of its network.
            organization_uuid: performers.enabled && performer !== OWN ? performer : undefined,
          },
        }),
      );
    },
    onSuccess: (created) => {
      qc.setQueryData(['services', 'detail', created.uuid], created);
      void qc.invalidateQueries({ queryKey: ['services', 'list'] });
      void qc.invalidateQueries({ queryKey: ['dealer-home'] });
      setCreatedUuid(created.uuid);
      setWanted('photos');
    },
  });

  const removeItem = useMutation({
    mutationFn: (item: string) => unwrap(api.DELETE('/v1/services/{uuid}/items/{item}', { params: { path: { uuid, item } } })),
    onSuccess: onWrite,
    onError: (e) => Alert.alert(t('action_failed'), errorText(e)),
  });

  const finish = useMutation({
    mutationFn: async ({ to }: { to: ServiceStatus }) => {
      let cur = s!;
      const body = servicePatch(cur, form);
      if (body) {
        cur = await unwrap(api.PATCH('/v1/services/{uuid}', { params: { path: { uuid } }, body }));
        onWrite(cur);
      }
      if (to !== cur.status) {
        cur = await unwrap(api.POST('/v1/services/{uuid}/transitions', { params: { path: { uuid } }, body: { status: to } }));
      }
      return cur;
    },
    onSuccess: (done) => {
      onWrite(done);
      try {
        storage.remove(partsKey(uuid));
      } catch {
        // Nothing to clean up.
      }
      // A kept draft is easy to mistake for a finished service: say so.
      if (done.status === 'draft') Alert.alert(t('wizard.saved_draft'));
      releaseLeave();
      if (params.uuid && router.canGoBack()) router.back();
      else router.replace(links.service(uuid));
    },
    onError: (e) => {
      if (isApiError(e, 'PHOTO_STANDARD_INCOMPLETE')) {
        void qc.invalidateQueries({ queryKey: intakeKey(uuid) });
        setWanted('photos');
        Alert.alert(t('photos_missing_title'), t('photos_missing_body'));
        return;
      }
      if (isApiError(e, 'VALIDATION_ERROR') && e.details?.some((d) => d.field === 'items')) {
        Alert.alert(t('action_failed'), t('complete_needs_items'));
        return;
      }
      Alert.alert(t('action_failed'), errorText(e));
    },
  });

  const continueFromCustomer = () => {
    setStepError(null);
    setKmInvalid(false);
    const err = s
      ? parseKm(form.km) === 'invalid'
        ? 'km'
        : null
      : customerStepError({ customer, vehicle, km: form.km });
    if (err === 'km') return setKmInvalid(true);
    if (err) return setStepError(t(`wizard.error.${err}`));
    if (s) leave('customer');
    else if (performerName) {
      // Opened for another dealer: the performer cannot be changed afterwards.
      confirmAction({
        title: t('wizard.open_for_confirm', { name: performerName }),
        body: t('wizard.open_for_body'),
        action: t('wizard.open_draft'),
        onConfirm: () => create.mutate(),
      });
    } else create.mutate();
  };

  const submit = () => {
    if (!s) return;
    setStepError(null);
    const closed = stepBlocker('summary', gate);
    if (closed) return setBlocked(closed);
    const err = summaryError(form);
    if (err) return setStepError(t(`wizard.error.${err}`));
    const to = finishTo ?? s.status;
    if (finishBlocker(to, s.items?.length ?? 0)) return setStepError(t('complete_needs_items'));
    if (to === 'completed') {
      Alert.alert(t('transition_confirm.completed'), t('wizard.complete_body'), [
        { text: t('common:cancel'), style: 'cancel' },
        { text: t('transition.completed'), onPress: () => finish.mutate({ to }) },
      ]);
      return;
    }
    finish.mutate({ to });
  };

  const confirmRemove = (item: string, name: string) => {
    Alert.alert(t('item_remove_confirm'), name, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('item_remove'), style: 'destructive', onPress: () => removeItem.mutate(item) },
    ]);
  };

  const loading = !!uuid && !s;
  const busy = create.isPending || finish.isPending;
  const editable = !!s && canWrite && s.items_editable;

  return (
    <>
      <Stack.Screen options={{ title: s ? s.service_no : t('new_service'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        automaticallyAdjustKeyboardInsets
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2">
        <Progress
          steps={steps}
          current={step}
          gate={gate}
          disabled={busy}
          onSelect={(to) => go(to)}
          onBlocked={(b) => {
            setStepError(null);
            setBlocked(b);
          }}
        />

        {loading ? (
          svc.error ? (
            <ErrorState error={svc.error} onRetry={() => void svc.refetch()} />
          ) : (
            <DetailSkeleton />
          )
        ) : gateLoading && step !== 'customer' ? (
          <DetailSkeleton />
        ) : step === 'customer' ? (
          <View className="gap-4">
            {s ? (
              <LockedIntake s={s} />
            ) : (
              <>
                {performers.enabled ? (
                  // Center workplace: who performs the service is the first question.
                  <PerformerPicker performers={performers} value={performer} onChange={setPerformer} />
                ) : null}
                <StepBlock icon="customer" title={t('customer')} done={!!customer}>
                  {customer ? (
                    <Chosen
                      title={customerTitle(customer)}
                      subtitle={customerSubtitle(customer)}
                      onChange={() => {
                        setCustomer(null);
                        setVehicle(null);
                      }}
                    />
                  ) : (
                    <CustomerStep selected={customer} onPick={(c) => setCustomer(c)} />
                  )}
                </StepBlock>
                {customer ? (
                  <StepBlock icon="vehicle" title={t('vehicle')} done={!!vehicle}>
                    {vehicle ? (
                      <Chosen
                        title={vehicleLabel(vehicle.car_brand?.name, vehicle.car_model?.name, vehicle.model_year) || (vehicle.plate ?? '')}
                        plate={vehicle.plate}
                        plateCountry={vehicle.plate_country}
                        onChange={() => setVehicle(null)}
                      />
                    ) : (
                      <VehicleStep customer={customer} selected={vehicle} onPick={(v) => setVehicle(v)} onFix={setFixing} />
                    )}
                  </StepBlock>
                ) : null}
                {fixing ? (
                  <EditVehicleSheet
                    vehicle={fixing}
                    onClose={() => setFixing(null)}
                    onSaved={(v) => {
                      if (v.car_model) setVehicle(v);
                    }}
                  />
                ) : null}
              </>
            )}
            {s || vehicle ? (
              <Input
                label={t('km')}
                placeholder={t('placeholder_km', { ns: 'common' })}
                keyboardType="number-pad"
                value={form.km}
                onChangeText={(km) => {
                  setKmInvalid(false);
                  patchForm({ km });
                }}
                editable={!s || s.editable}
                error={kmInvalid ? t('wizard.error.km') : undefined}
              />
            ) : null}
            {create.error ? <ErrorLine text={errorText(create.error)} /> : null}
            {!s && !vehicle && stepError ? <ErrorLine text={stepError} /> : null}
            {!s && vehicle && performerName ? (
              <View className="flex-row items-center gap-2 rounded-lg bg-primary/10 px-3 py-2.5">
                <Icon name="building" tone="primary" size={16} />
                <Text variant="callout" tone="primary" className="flex-1 font-semibold">
                  {t('wizard.open_for', { name: performerName })}
                </Text>
              </View>
            ) : null}
            {s || vehicle ? (
              <StepActions
                error={stepError}
                nextTitle={s ? t('wizard.continue') : t('wizard.open_draft')}
                loading={create.isPending}
                onNext={continueFromCustomer}
              />
            ) : null}
            {!s && vehicle ? (
              <Text variant="footnote" tone="muted" className="text-center">
                {t('wizard.draft_hint')}
              </Text>
            ) : null}
          </View>
        ) : step === 'photos' && s ? (
          <View className="gap-4">
            <Text variant="callout" tone="muted">
              {t('wizard.photos_hint')}
            </Text>
            <IntakePhotos uuid={s.uuid} canCapture={canWrite && s.editable && s.status !== 'completed' && s.status !== 'cancelled'} />
            <StepActions
              error={photosMissing ? t('wizard.photos_missing') : stepError}
              onBack={() => go(previousStep('photos', steps))}
              onNext={() => leave('photos')}
              nextDisabled={photosMissing}
            />
          </View>
        ) : step === 'parts' && s ? (
          <View className="gap-4">
            <Text variant="callout" tone="muted">
              {t('wizard.parts_hint')}
            </Text>
            <VehiclePartPicker
              value={parts}
              onChange={
                editable
                  ? (next) => {
                      setStepError(null);
                      setParts(next);
                    }
                  : undefined
              }
              readOnly={!editable}
            />
            <StepActions error={stepError} onBack={() => go(previousStep('parts', steps))} onNext={() => leave('parts')} />
          </View>
        ) : step === 'products' && s ? (
          <View className="gap-4">
            <ItemsCard
              s={s}
              editable={editable}
              removing={removeItem.isPending}
              onAdd={() => router.push(links.addItem(s.uuid, parts))}
              onRemove={confirmRemove}
            />
            {parts.length ? (
              <Text variant="footnote" tone="muted">
                {t('wizard.parts_applied', { count: parts.length })}
              </Text>
            ) : null}
            <StepActions
              error={stepError}
              onBack={() => go(previousStep('products', steps))}
              onNext={() => leave('products')}
            />
          </View>
        ) : step === 'summary' && s ? (
          <SummaryStep
            s={s}
            form={form}
            onForm={patchForm}
            parts={parts}
            choice={finishTo ?? s.status}
            onChoice={setChoice}
            error={stepError}
            busy={finish.isPending}
            onBack={() => go(previousStep('summary', steps))}
            onSubmit={submit}
            onJump={(to) => go(to)}
          />
        ) : null}
      </ScrollView>
    </>
  );
}

// ---------------------------------------------------------------- progress

function Progress({
  steps,
  current,
  gate,
  disabled,
  onSelect,
  onBlocked,
}: {
  steps: readonly WizardStep[];
  current: WizardStep;
  gate: GateCtx;
  disabled: boolean;
  onSelect: (s: WizardStep) => void;
  /** A closed step was tapped: tell why instead of ignoring the tap. */
  onBlocked: (b: StepBlocker) => void;
}) {
  const { t, i18n } = useTranslation('dealer');
  const at = steps.indexOf(current);
  return (
    <View className="gap-2.5 rounded-2xl border border-border bg-card p-3.5" style={{ borderCurve: 'continuous' }}>
      <View className="flex-row items-baseline justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="min-w-0 flex-1">
          {t(`wizard.step.${current}`)}
        </Text>
        <Text variant="footnote" tone="muted" className="tabular-nums">
          {t('wizard.step_of', { at: formatNumber(at + 1, i18n.language), total: formatNumber(steps.length, i18n.language) })}
        </Text>
      </View>
      <View className="flex-row gap-1.5">
        {steps.map((st, i) => {
          const blocker = stepBlocker(st, gate);
          return (
            <Pressable
              key={st}
              accessibilityRole="tab"
              accessibilityLabel={t(`wizard.step.${st}`)}
              accessibilityState={{ selected: st === current, disabled: !!blocker || disabled }}
              disabled={disabled}
              hitSlop={{ top: 10, bottom: 10 }}
              onPress={() => (blocker ? onBlocked(blocker) : onSelect(st))}
              className="flex-1">
              <View className={cn('h-1.5 rounded-full', i < at ? 'bg-primary' : i === at ? 'bg-primary/70' : 'bg-muted')} />
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

function StepActions({
  onBack,
  onNext,
  nextTitle,
  nextDisabled,
  loading,
  error,
}: {
  onBack?: () => void;
  onNext: () => void;
  nextTitle?: string;
  nextDisabled?: boolean;
  loading?: boolean;
  error?: string | null;
}) {
  const { t } = useTranslation('dealer');
  return (
    <View className="gap-2 pt-1">
      {error ? <ErrorLine text={error} /> : null}
      <View className="flex-row gap-2.5">
        {onBack ? (
          <Button title={t('common:back')} variant="outline" size="lg" icon="back" onPress={onBack} disabled={loading} className="flex-1" />
        ) : null}
        <Button
          title={nextTitle ?? t('wizard.continue')}
          size="lg"
          loading={loading}
          disabled={nextDisabled}
          onPress={onNext}
          className="flex-[1.4]"
        />
      </View>
    </View>
  );
}

function ErrorLine({ text }: { text: string }) {
  return (
    <View className="flex-row items-start gap-2 rounded-lg bg-destructive/10 px-3 py-2.5">
      <Icon name="alert" tone="destructive" size={16} />
      <Text variant="footnote" tone="danger" className="flex-1">
        {text}
      </Text>
    </View>
  );
}

// ---------------------------------------------------------------- step 1

function StepBlock({ icon, title, done, children }: { icon: IconName; title: string; done: boolean; children: React.ReactNode }) {
  return (
    <View className="gap-2">
      <View className="flex-row items-center gap-2 px-1">
        <View className={cn('size-6 items-center justify-center rounded-full', done ? 'bg-primary' : 'bg-primary/10')}>
          <Icon name={done ? 'check' : icon} tone={done ? 'primary-foreground' : 'primary'} size={13} weight="semibold" />
        </View>
        <Text variant="callout" className="font-semibold">
          {title}
        </Text>
      </View>
      {children}
    </View>
  );
}

function Chosen({
  title,
  subtitle,
  plate,
  plateCountry,
  onChange,
}: {
  title: string;
  subtitle?: string;
  plate?: string | null;
  plateCountry?: string | null;
  onChange: () => void;
}) {
  const { t } = useTranslation('dealer');
  return (
    <View className="flex-row items-center gap-3 rounded-2xl border border-border bg-card px-3.5 py-3" style={{ borderCurve: 'continuous' }}>
      <View className="min-w-0 flex-1 gap-1">
        <Text variant="body" className="font-semibold" numberOfLines={1}>
          {title}
        </Text>
        {plate ? (
          <View className="flex-row">
            <PlateBadge plate={plate} country={plateCountry} />
          </View>
        ) : null}
        {subtitle ? (
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {subtitle}
          </Text>
        ) : null}
      </View>
      <Pressable accessibilityRole="button" hitSlop={8} onPress={onChange}>
        <Text variant="footnote" tone="primary" className="font-semibold">
          {t('change')}
        </Text>
      </Pressable>
    </View>
  );
}

/** Customer and vehicle of an open draft: copied into the service, not changeable. */
function LockedIntake({ s }: { s: Service }) {
  const { t } = useTranslation('dealer');
  return (
    <View className="gap-2">
      <ServiceHero s={s} />
      <Text variant="footnote" tone="muted" className="px-1">
        {t('wizard.intake_locked')}
      </Text>
    </View>
  );
}

const OWN = 'own';

/**
 * Who performs the service. Since TEC-1003 the center runs its own service
 * point and may also open a service on behalf of a dealer of its network
 * (web: "Hizmeti yapan"); the backend accepts `organization_uuid` only there.
 */
function usePerformers() {
  const me = useSession((s) => s.me);
  const canList = useCan('organizations.read');
  const enabled = isCenterWorkplace(me) && canList;
  const dealers = useQuery({
    queryKey: ['tenant', 'organizations', 'dealers'],
    enabled,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/organizations', { params: { query: { type: 'dealer', limit: 200 } } })),
  });
  return {
    enabled,
    ownName: activeMembership(me)?.name ?? '',
    dealers: (dealers.data?.items ?? []).map((o) => ({ uuid: o.uuid, name: o.name })),
    isPending: enabled && dealers.isPending,
    error: dealers.error,
    refetch: () => void dealers.refetch(),
  };
}

function PerformerPicker({
  performers,
  value,
  onChange,
}: {
  performers: ReturnType<typeof usePerformers>;
  value: string;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation('dealer');
  const [open, setOpen] = useState(false);
  const own = t('performer_own', { name: performers.ownName });
  const current = value === OWN ? own : (performers.dealers.find((d) => d.uuid === value)?.name ?? own);
  // Own service point first, then the dealers of the network (search above seven).
  const options: PickerOption[] = [
    { key: OWN, title: own, icon: 'building' },
    ...performers.dealers.map((d) => ({ key: d.uuid, title: d.name, icon: 'building' as const })),
  ];
  return (
    <StepBlock icon="building" title={t('performer_label')} done={false}>
      <Section>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={`${t('performer_label')}, ${current}`}
          accessibilityHint={t('performer_hint')}
          onPress={() => setOpen(true)}
          className="flex-row items-center gap-3 px-4 py-3 active:bg-muted">
          <View className="min-w-0 flex-1 gap-0.5">
            <Text variant="body" numberOfLines={1}>
              {current}
            </Text>
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {t('performer_hint')}
            </Text>
          </View>
          <Icon name="chevronDown" tone="muted-foreground" size={16} />
        </Pressable>
      </Section>
      <PickerSheet
        visible={open}
        title={t('performer_label')}
        options={performers.isPending || performers.error ? [] : options}
        selectedKey={value}
        selectedLabel={current}
        loading={performers.isPending}
        error={performers.error}
        onRetry={performers.refetch}
        onPick={(o) => onChange(o.key)}
        onClose={() => setOpen(false)}
      />
    </StepBlock>
  );
}

// ---------------------------------------------------------------- products

function ItemsCard({
  s,
  editable,
  removing,
  onAdd,
  onRemove,
}: {
  s: Service;
  editable: boolean;
  removing: boolean;
  onAdd: () => void;
  onRemove: (item: string, name: string) => void;
}) {
  const { t, i18n } = useTranslation('dealer');
  const items = s.items ?? [];
  return (
    <View className="gap-2">
      <View className="flex-row items-center gap-2 px-1">
        <Text variant="caption" tone="muted" className="flex-1">
          {t('items')}
        </Text>
        {items.length ? (
          <View className="rounded-full bg-primary/10 px-2 py-0.5">
            <Text variant="footnote" tone="primary" className="font-semibold tabular-nums">
              {formatNumber(items.length, i18n.language)}
            </Text>
          </View>
        ) : null}
      </View>
      {items.length ? (
        <Section>
          {items.map((it, i) => (
            <View key={it.uuid} className={cn('flex-row items-center gap-3 px-4 py-3', i < items.length - 1 && 'border-b border-border')}>
              <View className="size-9 items-center justify-center rounded-full bg-secondary">
                <Icon name="barcode" tone="primary" size={16} />
              </View>
              <View className="min-w-0 flex-1 gap-0.5">
                <Text variant="body" numberOfLines={2}>
                  {it.product.name}
                </Text>
                <Text variant="footnote" tone="muted" numberOfLines={2}>
                  {[
                    it.barcode,
                    it.kind === 'partial' && it.meters ? t('meters_value', { value: it.meters }) : null,
                    it.quantity ? t('quantity_value', { value: formatNumber(it.quantity, i18n.language) }) : null,
                    it.applied_parts.length ? t('parts_count', { count: it.applied_parts.length }) : null,
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </Text>
              </View>
              {editable ? (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('item_remove')}
                  hitSlop={8}
                  disabled={removing}
                  onPress={() => onRemove(it.uuid, it.product.name)}>
                  <Icon name="delete" tone="destructive" size={18} />
                </Pressable>
              ) : null}
            </View>
          ))}
        </Section>
      ) : (
        <View className="items-center gap-2 rounded-2xl border border-dashed border-border bg-card px-4 py-6" style={{ borderCurve: 'continuous' }}>
          <Icon name="packageIn" tone="muted-foreground" />
          <Text variant="callout" tone="muted" className="text-center">
            {t('items_empty')}
          </Text>
          <Text variant="footnote" tone="muted" className="text-center">
            {t('wizard.needs_items')}
          </Text>
        </View>
      )}
      {editable ? <Button title={t('item_add')} variant="secondary" icon="add" onPress={onAdd} /> : null}
    </View>
  );
}

// ---------------------------------------------------------------- summary

function SummaryStep({
  s,
  form,
  onForm,
  parts,
  choice,
  onChoice,
  error,
  busy,
  onBack,
  onSubmit,
  onJump,
}: {
  s: Service;
  form: SummaryForm;
  onForm: (p: Partial<SummaryForm>) => void;
  parts: string[];
  choice: ServiceStatus;
  onChoice: (c: ServiceStatus) => void;
  error: string | null;
  busy: boolean;
  onBack: () => void;
  onSubmit: () => void;
  onJump: (to: WizardStep) => void;
}) {
  const { t, i18n } = useTranslation('dealer');
  const items = s.items?.length ?? 0;
  const choices = finishChoices(s.status, s.available_transitions);
  const km = parseKm(form.km);

  return (
    <View className="gap-4">
      <ServiceHero s={s}>
        <View className="flex-row gap-2">
          <Fact
            label={t('wizard.fact_parts')}
            value={formatNumber(parts.length, i18n.language)}
            onPress={() => onJump('parts')}
          />
          <Fact label={t('wizard.fact_items')} value={formatNumber(items, i18n.language)} onPress={() => onJump('products')} />
          <Fact
            label={t('km')}
            value={typeof km === 'number' ? formatNumber(km, i18n.language) : '—'}
            onPress={() => onJump('customer')}
          />
        </View>
      </ServiceHero>

      {s.service_no_editable !== false ? (
        <ServiceNoField value={form.serviceNo} onChange={(serviceNo) => onForm({ serviceNo })} current={s.service_no} />
      ) : null}

      <Input
        label={t('notes')}
        placeholder={t('placeholder_notes')}
        value={form.notes}
        onChangeText={(notes) => onForm({ notes })}
        multiline
        maxLength={5000}
        editable={s.editable}
        className="min-h-24 py-3"
      />

      <View className="flex-row items-center justify-between gap-3 rounded-2xl border border-border bg-card px-4 py-3" style={{ borderCurve: 'continuous' }}>
        <View className="min-w-0 flex-1 gap-0.5">
          <Text variant="body">{t('has_measurement')}</Text>
          <Text variant="footnote" tone="muted">
            {s.vin ? t('wizard.measurement_hint') : t('measurement_needs_vin')}
          </Text>
        </View>
        <Switch
          accessibilityLabel={t('has_measurement')}
          value={form.measure && !!s.vin}
          onValueChange={(measure) => onForm({ measure })}
          disabled={!s.vin || !s.editable}
        />
      </View>

      <View className="gap-2">
        <Text variant="caption" tone="muted" className="px-1">
          {t('wizard.finish_as')}
        </Text>
        <Section>
          {choices.map((c, i) => {
            const blocked = finishBlocker(c, items) !== null;
            return (
              <Pressable
                key={c}
                accessibilityRole="radio"
                accessibilityState={{ checked: c === choice, disabled: blocked }}
                disabled={blocked || busy}
                onPress={() => onChoice(c)}
                className={cn(
                  'min-h-14 flex-row items-center gap-3 px-4 py-3 active:bg-muted',
                  i < choices.length - 1 && 'border-b border-border',
                  blocked && 'opacity-50',
                )}>
                <View className={cn('size-5 items-center justify-center rounded-full border-2', c === choice ? 'border-primary' : 'border-border')}>
                  {c === choice ? <View className="size-2.5 rounded-full bg-primary" /> : null}
                </View>
                <View className="min-w-0 flex-1 gap-0.5">
                  <Text variant="body" className={cn(c === choice && 'font-semibold')}>
                    {c === s.status ? t('wizard.keep', { status: t(`service_status.${c}`) }) : t(`service_status.${c}`)}
                  </Text>
                  <Text variant="footnote" tone="muted">
                    {blocked ? t('complete_needs_items') : t(`wizard.finish_hint.${c}`)}
                  </Text>
                </View>
                <StatusPill label={t(`service_status.${c}`)} tone={serviceTone(c)} />
              </Pressable>
            );
          })}
        </Section>
      </View>

      <StepActions
        error={error}
        onBack={onBack}
        onNext={onSubmit}
        loading={busy}
        nextTitle={t(finishButtonKey(s.status, choice), { status: t(`service_status.${choice}`) })}
      />
    </View>
  );
}

function Fact({ label, value, onPress }: { label: string; value: string; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${label}: ${value}`}
      onPress={onPress}
      className="min-w-0 flex-1 gap-0.5 rounded-xl bg-secondary px-3 py-2 active:opacity-80"
      style={{ borderCurve: 'continuous' }}>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {label}
      </Text>
      <Text variant="headline" className="tabular-nums" numberOfLines={1}>
        {value}
      </Text>
    </Pressable>
  );
}
