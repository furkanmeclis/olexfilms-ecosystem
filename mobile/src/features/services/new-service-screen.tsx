import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Pressable, ScrollView, Switch, View } from 'react-native';
import { z } from 'zod';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { type CustomerSummary, fullName, useDebounced, useLinks, type Vehicle, vehicleLabel } from './lib';
import { errorText, ListSkeleton, SearchField, Section } from './ui';
import type { IconName } from '@/ui/icons';

type Step = 'customer' | 'vehicle' | 'details';
const STEPS: Step[] = ['customer', 'vehicle', 'details'];

type PickedCustomer = { uuid: string; name: string; surname?: string | null; phone?: string | null };

/**
 * New draft service in three short steps: customer, vehicle, details. Items
 * are added on the service screen right after (stock picker or barcode).
 */
export function NewServiceScreen() {
  const { t } = useTranslation('dealer');
  const links = useLinks();
  const qc = useQueryClient();
  const [step, setStep] = useState<Step>('customer');
  const [customer, setCustomer] = useState<PickedCustomer | null>(null);
  const [vehicle, setVehicle] = useState<Vehicle | null>(null);
  const [km, setKm] = useState('');
  const [notes, setNotes] = useState('');
  const [measure, setMeasure] = useState(false);

  const create = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/services', {
          body: {
            customer_uuid: customer!.uuid,
            vehicle_uuid: vehicle!.uuid,
            km: km.trim() ? Number.parseInt(km.replace(/\D/g, ''), 10) : undefined,
            notes: notes.trim() || undefined,
            has_measurement: measure && !!vehicle?.vin,
          },
        }),
      ),
    onSuccess: (s) => {
      qc.setQueryData(['services', 'detail', s.uuid], s);
      void qc.invalidateQueries({ queryKey: ['services', 'list'] });
      void qc.invalidateQueries({ queryKey: ['dealer-home'] });
      router.replace(links.service(s.uuid));
    },
  });

  const at = STEPS.indexOf(step);

  return (
    <>
      <Stack.Screen options={{ title: t('new_service'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2">
        <View className="flex-row gap-1.5">
          {STEPS.map((s, i) => (
            <Pressable
              key={s}
              accessibilityRole="tab"
              accessibilityState={{ selected: s === step }}
              disabled={i > at}
              onPress={() => setStep(s)}
              className="flex-1 gap-1.5">
              <View className={cn('h-1 rounded-full', i <= at ? 'bg-primary' : 'bg-muted')} />
              <Text variant="footnote" tone={s === step ? 'default' : 'muted'} className="font-semibold">
                {i + 1}. {t(`step.${s}`)}
              </Text>
            </Pressable>
          ))}
        </View>

        {step === 'customer' ? (
          <CustomerStep
            selected={customer}
            onPick={(c) => {
              if (c.uuid !== customer?.uuid) setVehicle(null);
              setCustomer(c);
              setStep('vehicle');
            }}
          />
        ) : null}

        {step === 'vehicle' && customer ? (
          <VehicleStep
            customer={customer}
            selected={vehicle}
            onPick={(v) => {
              setVehicle(v);
              setStep('details');
            }}
          />
        ) : null}

        {step === 'details' && customer && vehicle ? (
          <View className="gap-4">
            <Section>
              <Summary icon="customer" title={fullName(customer)} subtitle={customer.phone ?? ''} onPress={() => setStep('customer')} />
              <Summary
                icon="vehicle"
                title={vehicle.plate ?? ''}
                subtitle={vehicleLabel(vehicle.car_brand?.name, vehicle.car_model?.name, vehicle.model_year)}
                onPress={() => setStep('vehicle')}
                last
              />
            </Section>
            <Input label={t('km')} keyboardType="number-pad" value={km} onChangeText={setKm} />
            <Input label={t('notes')} value={notes} onChangeText={setNotes} multiline className="min-h-24 py-3" />
            <View className="flex-row items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3">
              <View className="flex-1 gap-0.5">
                <Text variant="body">{t('has_measurement')}</Text>
                {!vehicle.vin ? (
                  <Text variant="footnote" tone="muted">
                    {t('measurement_needs_vin')}
                  </Text>
                ) : null}
              </View>
              <Switch value={measure && !!vehicle.vin} onValueChange={setMeasure} disabled={!vehicle.vin} />
            </View>
            {create.error ? (
              <Text variant="footnote" tone="danger">
                {errorText(create.error)}
              </Text>
            ) : null}
            <Button title={t('create_service')} size="lg" loading={create.isPending} onPress={() => create.mutate()} />
            <Text variant="footnote" tone="muted" className="text-center">
              {t('items_after_create')}
            </Text>
          </View>
        ) : null}
      </ScrollView>
    </>
  );
}

function Summary({
  icon,
  title,
  subtitle,
  onPress,
  last,
}: {
  icon: IconName;
  title: string;
  subtitle: string;
  onPress: () => void;
  last?: boolean;
}) {
  const { t } = useTranslation('dealer');
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className={cn('flex-row items-center gap-3 px-4 py-3 active:bg-muted', !last && 'border-b border-border')}>
      <Icon name={icon} tone="primary" size={20} />
      <View className="flex-1 gap-0.5">
        <Text variant="body" numberOfLines={1}>
          {title}
        </Text>
        {subtitle ? (
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {subtitle}
          </Text>
        ) : null}
      </View>
      <Text variant="footnote" tone="primary">
        {t('change')}
      </Text>
    </Pressable>
  );
}

function PickRow({
  title,
  subtitle,
  selected,
  disabled,
  onPress,
  last,
}: {
  title: string;
  subtitle?: string;
  selected?: boolean;
  disabled?: boolean;
  onPress: () => void;
  last?: boolean;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected, disabled }}
      disabled={disabled}
      onPress={onPress}
      className={cn('min-h-12 flex-row items-center gap-3 px-4 py-3 active:bg-muted', !last && 'border-b border-border', disabled && 'opacity-50')}>
      <View className="flex-1 gap-0.5">
        <Text variant="body" numberOfLines={1}>
          {title}
        </Text>
        {subtitle ? (
          <Text variant="footnote" tone="muted" numberOfLines={2}>
            {subtitle}
          </Text>
        ) : null}
      </View>
      {selected ? <Icon name="check" tone="primary" size={18} /> : null}
    </Pressable>
  );
}

// ---------------------------------------------------------------- customer

function CustomerStep({ selected, onPick }: { selected: PickedCustomer | null; onPick: (c: PickedCustomer) => void }) {
  const { t } = useTranslation('dealer');
  const canCreate = useCan('customers.write');
  const [q, setQ] = useState('');
  const [creating, setCreating] = useState(false);
  const query = useDebounced(q.trim());

  const list = useQuery({
    queryKey: ['customers', 'pick', query],
    queryFn: () =>
      unwrap(api.GET('/v1/customers', { params: { query: { q: query || undefined, status: 'active', limit: 10, offset: 0 } } })),
    enabled: !creating,
  });

  if (creating) return <NewCustomerForm initial={q} onCreated={onPick} onCancel={() => setCreating(false)} />;

  const items: CustomerSummary[] = list.data?.items ?? [];
  return (
    <View className="gap-3">
      <SearchField value={q} onChange={setQ} placeholder={t('customer_search')} />
      {list.isPending ? (
        <ListSkeleton rows={3} />
      ) : list.error ? (
        <Text variant="footnote" tone="danger">
          {errorText(list.error)}
        </Text>
      ) : items.length ? (
        <Section>
          {items.map((c, i) => (
            <PickRow
              key={c.uuid}
              title={fullName(c) || c.company_name || '—'}
              subtitle={[c.phone, c.company_name].filter(Boolean).join(' · ')}
              selected={c.uuid === selected?.uuid}
              onPress={() => onPick(c)}
              last={i === items.length - 1}
            />
          ))}
        </Section>
      ) : (
        <Text variant="callout" tone="muted" className="py-2 text-center">
          {t('customers_empty_filtered')}
        </Text>
      )}
      {canCreate ? (
        <Button
          title={t('customer_new')}
          variant="outline"
          icon={<Icon name="add" tone="primary" size={18} />}
          onPress={() => setCreating(true)}
        />
      ) : null}
    </View>
  );
}

const customerSchema = z.object({
  name: z.string().trim().min(1),
  surname: z.string().trim().optional(),
  phone: z.string().trim().min(7),
});
type CustomerForm = z.infer<typeof customerSchema>;

function NewCustomerForm({
  initial,
  onCreated,
  onCancel,
}: {
  initial: string;
  onCreated: (c: PickedCustomer) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('dealer');
  const qc = useQueryClient();
  const looksLikePhone = /^[+\d\s()-]{7,}$/.test(initial);
  const form = useForm<CustomerForm>({
    resolver: zodResolver(customerSchema),
    defaultValues: { name: looksLikePhone ? '' : initial, surname: '', phone: looksLikePhone ? initial : '' },
  });
  const create = useMutation({
    mutationFn: (v: CustomerForm) =>
      unwrap(api.POST('/v1/customers', { body: { name: v.name, surname: v.surname || undefined, phone: v.phone } })),
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: ['customers'] });
      onCreated(c);
    },
    onError: (e) => {
      if (!isApiError(e)) return;
      for (const [field, message] of Object.entries(e.fieldErrors())) {
        if (field === 'name' || field === 'surname' || field === 'phone') form.setError(field, { message });
      }
    },
  });
  const err = (f: keyof CustomerForm) => (form.formState.errors[f] ? t(`invalid.${f}`) : undefined);

  return (
    <View className="gap-3">
      <Text variant="headline">{t('customer_new')}</Text>
      <Controller
        control={form.control}
        name="name"
        render={({ field }) => (
          <Input label={t('first_name')} value={field.value} onChangeText={field.onChange} error={err('name')} autoCapitalize="words" />
        )}
      />
      <Controller
        control={form.control}
        name="surname"
        render={({ field }) => (
          <Input label={t('surname')} value={field.value ?? ''} onChangeText={field.onChange} autoCapitalize="words" />
        )}
      />
      <Controller
        control={form.control}
        name="phone"
        render={({ field }) => (
          <Input
            label={t('phone')}
            value={field.value}
            onChangeText={field.onChange}
            error={err('phone')}
            keyboardType="phone-pad"
            hint={t('phone_hint')}
          />
        )}
      />
      {create.error ? (
        <Text variant="footnote" tone="danger">
          {errorText(create.error)}
        </Text>
      ) : null}
      <Button title={t('customer_save')} loading={create.isPending} onPress={() => void form.handleSubmit((v) => create.mutate(v))()} />
      <Button title={t('common:back')} variant="ghost" onPress={onCancel} disabled={create.isPending} />
    </View>
  );
}

// ----------------------------------------------------------------- vehicle

function VehicleStep({
  customer,
  selected,
  onPick,
}: {
  customer: PickedCustomer;
  selected: Vehicle | null;
  onPick: (v: Vehicle) => void;
}) {
  const { t } = useTranslation('dealer');
  const canCreate = useCan('vehicles.write');
  const [creating, setCreating] = useState(false);
  const list = useQuery({
    queryKey: ['vehicles', 'of-customer', customer.uuid],
    queryFn: () =>
      unwrap(api.GET('/v1/vehicles', { params: { query: { customer_uuid: customer.uuid, limit: 50, offset: 0 } } })),
  });

  if (creating) return <NewVehicleForm customerUuid={customer.uuid} onCreated={onPick} onCancel={() => setCreating(false)} />;

  const items = list.data?.items ?? [];
  return (
    <View className="gap-3">
      <Text variant="footnote" tone="muted">
        {t('vehicles_of', { name: fullName(customer) })}
      </Text>
      {list.isPending ? (
        <ListSkeleton rows={2} />
      ) : list.error ? (
        <Text variant="footnote" tone="danger">
          {errorText(list.error)}
        </Text>
      ) : items.length ? (
        <Section>
          {items.map((v, i) => (
            <PickRow
              key={v.uuid}
              title={v.plate || v.vin || '—'}
              subtitle={
                v.car_model
                  ? vehicleLabel(v.car_brand?.name, v.car_model?.name, v.model_year)
                  : t('vehicle_needs_model')
              }
              disabled={!v.car_model}
              selected={v.uuid === selected?.uuid}
              onPress={() => onPick(v)}
              last={i === items.length - 1}
            />
          ))}
        </Section>
      ) : (
        <Text variant="callout" tone="muted" className="py-2 text-center">
          {t('vehicles_empty')}
        </Text>
      )}
      {canCreate ? (
        <Button
          title={t('vehicle_new')}
          variant={items.length ? 'outline' : 'primary'}
          icon={<Icon name="add" tone={items.length ? 'primary' : 'primary-foreground'} size={18} />}
          onPress={() => setCreating(true)}
        />
      ) : null}
    </View>
  );
}

export const VIN_RE = /^[A-HJ-NPR-Z0-9]{17}$/;
const vehicleSchema = z.object({
  plate: z.string().trim().min(2).max(20),
  model: z.object({ uuid: z.string(), name: z.string(), brand: z.string() }).nullable().refine((m) => m !== null),
  year: z
    .string()
    .trim()
    .refine((y) => y === '' || (/^\d{4}$/.test(y) && +y >= 1900 && +y <= new Date().getFullYear() + 1)),
  vin: z
    .string()
    .trim()
    .refine((v) => v === '' || VIN_RE.test(v.toUpperCase())),
});
type VehicleForm = z.output<typeof vehicleSchema>;
type VehicleFormInput = z.input<typeof vehicleSchema>;

function NewVehicleForm({
  customerUuid,
  onCreated,
  onCancel,
}: {
  customerUuid: string;
  onCreated: (v: Vehicle) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('dealer');
  const qc = useQueryClient();
  const form = useForm<VehicleFormInput, unknown, VehicleForm>({
    resolver: zodResolver(vehicleSchema),
    defaultValues: { plate: '', model: null, year: '', vin: '' },
  });
  const create = useMutation({
    mutationFn: (v: VehicleForm) =>
      unwrap(
        api.POST('/v1/vehicles', {
          body: {
            customer_uuid: customerUuid,
            plate: v.plate.toUpperCase(),
            car_model_uuid: v.model?.uuid,
            model_year: v.year ? Number(v.year) : undefined,
            vin: v.vin ? v.vin.toUpperCase() : undefined,
          },
        }),
      ),
    onSuccess: (v) => {
      void qc.invalidateQueries({ queryKey: ['vehicles'] });
      onCreated(v);
    },
    onError: (e) => {
      if (!isApiError(e)) return;
      const f = e.fieldErrors();
      if (f.plate || e.code === 'INVALID_PLATE') form.setError('plate', { message: f.plate ?? e.code });
      if (f.vin) form.setError('vin', { message: f.vin });
      if (f.model_year) form.setError('year', { message: f.model_year });
    },
  });
  const err = (f: keyof VehicleFormInput) => (form.formState.errors[f] ? t(`invalid.${f}`) : undefined);

  return (
    <View className="gap-3">
      <Text variant="headline">{t('vehicle_new')}</Text>
      <Controller
        control={form.control}
        name="plate"
        render={({ field }) => (
          <Input
            label={t('plate')}
            value={field.value}
            onChangeText={field.onChange}
            error={err('plate')}
            autoCapitalize="characters"
            autoCorrect={false}
          />
        )}
      />
      <Controller
        control={form.control}
        name="model"
        render={({ field }) => <ModelPicker value={field.value} onChange={field.onChange} error={err('model')} />}
      />
      <Controller
        control={form.control}
        name="year"
        render={({ field }) => (
          <Input
            label={t('model_year')}
            value={field.value}
            onChangeText={field.onChange}
            error={err('year')}
            keyboardType="number-pad"
            maxLength={4}
          />
        )}
      />
      <Controller
        control={form.control}
        name="vin"
        render={({ field }) => (
          <Input
            label={t('vin_optional')}
            value={field.value}
            onChangeText={field.onChange}
            error={err('vin')}
            autoCapitalize="characters"
            autoCorrect={false}
            maxLength={17}
          />
        )}
      />
      {create.error ? (
        <Text variant="footnote" tone="danger">
          {errorText(create.error)}
        </Text>
      ) : null}
      <Button title={t('vehicle_save')} loading={create.isPending} onPress={() => void form.handleSubmit((v) => create.mutate(v))()} />
      <Button title={t('common:back')} variant="ghost" onPress={onCancel} disabled={create.isPending} />
    </View>
  );
}

export type ModelValue = { uuid: string; name: string; brand: string } | null;

/** Car model from the vehicle catalog; "brand model" search (a model implies its brand). */
export function ModelPicker({ value, onChange, error }: { value: ModelValue; onChange: (v: ModelValue) => void; error?: string }) {
  const { t } = useTranslation('dealer');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const open = !value;
  const models = useQuery({
    queryKey: ['vehicle-catalog', 'models', query],
    queryFn: () =>
      unwrap(api.GET('/v1/vehicle-catalog/models', { params: { query: { q: query || undefined, limit: 12, offset: 0 } } })),
    enabled: open && query.length >= 2,
  });

  if (value) {
    return (
      <View className="gap-1.5">
        <Text variant="footnote" tone="muted" className="font-medium">
          {t('brand_model')}
        </Text>
        <Pressable
          accessibilityRole="button"
          onPress={() => onChange(null)}
          className="min-h-[52px] flex-row items-center justify-between rounded-lg border border-border bg-card px-4">
          <Text variant="body">
            {value.brand} {value.name}
          </Text>
          <Text variant="footnote" tone="primary">
            {t('change')}
          </Text>
        </Pressable>
      </View>
    );
  }

  const items = models.data?.items ?? [];
  return (
    <View className="gap-1.5">
      <Text variant="footnote" tone="muted" className="font-medium">
        {t('brand_model')}
      </Text>
      <SearchField value={q} onChange={setQ} placeholder={t('model_search')} />
      {error ? (
        <Text variant="footnote" tone="danger">
          {error}
        </Text>
      ) : null}
      {query.length >= 2 ? (
        models.isPending ? (
          <ListSkeleton rows={2} />
        ) : items.length ? (
          <Section>
            {items.map((m, i) => (
              <PickRow
                key={m.uuid}
                title={`${m.brand.name} ${m.name}`}
                subtitle={[m.year_start && `${m.year_start}–${m.year_stop ?? ''}`, m.body_type].filter(Boolean).join(' · ')}
                onPress={() => onChange({ uuid: m.uuid, name: m.name, brand: m.brand.name })}
                last={i === items.length - 1}
              />
            ))}
          </Section>
        ) : (
          <Text variant="footnote" tone="muted">
            {models.error ? errorText(models.error) : t('model_none')}
          </Text>
        )
      ) : null}
    </View>
  );
}
