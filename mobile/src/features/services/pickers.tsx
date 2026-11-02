import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Controller, useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';
import { z } from 'zod';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useCan } from '@/core/spaces';
import { PlateScanButton } from '@/features/plate-scan/plate-scan-button';
import { EMPTY_VEHICLE, isComplete, VehiclePicker, type VehicleValue } from '@/features/vehicle-picker';
import { Segmented } from '@/features/warehouse/parts';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { customerSubtitle, customerTitle } from './customer-name';
import { type CustomerSummary, useDebounced, type Vehicle, vehicleLabel } from './lib';
import { errorText, ListSkeleton, SearchField, Section } from './ui';

/**
 * Customer and vehicle pickers of the service intake (search, pick or create
 * inline). The appointment form reuses the two steps.
 */

export type PickedCustomer = {
  uuid: string;
  name: string;
  surname?: string | null;
  phone?: string | null;
  type?: string | null;
  company_name?: string | null;
};

export function PickRow({
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

export function CustomerStep({ selected, onPick }: { selected: PickedCustomer | null; onPick: (c: PickedCustomer) => void }) {
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
              title={customerTitle(c)}
              subtitle={customerSubtitle(c)}
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

const CUSTOMER_TYPES = ['individual', 'corporate'] as const;
const customerSchema = z
  .object({
    type: z.enum(CUSTOMER_TYPES),
    name: z.string().trim().min(1),
    surname: z.string().trim().optional(),
    phone: z.string().trim().min(7),
    company_name: z.string().trim().max(200).optional(),
    tax_office: z.string().trim().max(150).optional(),
    tax_no: z
      .string()
      .optional()
      .refine((v) => !v || /^[0-9A-Z]{5,20}$/.test(normalizeIdentity(v))),
  })
  .superRefine((v, ctx) => {
    if (v.type === 'corporate' && !v.company_name) {
      ctx.addIssue({ code: 'custom', path: ['company_name'], message: 'required' });
    }
  });
type CustomerForm = z.infer<typeof customerSchema>;
const INLINE_FIELDS = ['name', 'surname', 'phone', 'company_name', 'tax_office', 'tax_no'] as const;
type InlineField = (typeof INLINE_FIELDS)[number];

/** Tax number like normalizeIdentityNumber on the server. */
function normalizeIdentity(raw: string): string {
  return raw.toLocaleUpperCase('en-US').replace(/[\s\-./]/g, '');
}

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
    defaultValues: {
      type: 'individual',
      name: looksLikePhone ? '' : initial,
      surname: '',
      phone: looksLikePhone ? initial : '',
      company_name: '',
      tax_office: '',
      tax_no: '',
    },
  });
  const type = useWatch({ control: form.control, name: 'type' });
  const create = useMutation({
    mutationFn: (v: CustomerForm) => {
      const corporate = v.type === 'corporate';
      const taxNo = v.tax_no ? normalizeIdentity(v.tax_no) : '';
      return unwrap(
        api.POST('/v1/customers', {
          body: {
            type: v.type,
            name: v.name,
            surname: v.surname || undefined,
            phone: v.phone,
            ...(corporate && v.company_name ? { company_name: v.company_name } : {}),
            ...(corporate && v.tax_office ? { tax_office: v.tax_office } : {}),
            ...(corporate && taxNo ? { tax_no: taxNo } : {}),
          },
        }),
      );
    },
    onSuccess: (c) => {
      void qc.invalidateQueries({ queryKey: ['customers'] });
      onCreated(c);
    },
    onError: (e) => {
      if (!isApiError(e)) return;
      for (const [field, message] of Object.entries(e.fieldErrors())) {
        if (INLINE_FIELDS.includes(field as InlineField)) form.setError(field as InlineField, { message });
      }
    },
  });
  const err = (f: keyof CustomerForm) => (form.formState.errors[f] ? t(`invalid.${f}`) : undefined);

  return (
    <View className="gap-3">
      <Text variant="headline">{t('customer_new')}</Text>
      <Segmented
        value={type}
        options={CUSTOMER_TYPES.map((v) => ({ value: v, label: t(`customer_type.${v}`) }))}
        onChange={(v) => form.setValue('type', v)}
      />
      {type === 'corporate' ? (
        <>
          <Controller
            control={form.control}
            name="company_name"
            render={({ field }) => (
              <Input
                label={t('company_name')}
                placeholder={t('placeholder_company_name')}
                value={field.value ?? ''}
                onChangeText={field.onChange}
                error={err('company_name')}
              />
            )}
          />
          <Controller
            control={form.control}
            name="tax_office"
            render={({ field }) => (
              <Input
                label={t('tax_office')}
                placeholder={t('placeholder_tax_office')}
                value={field.value ?? ''}
                onChangeText={field.onChange}
                error={err('tax_office')}
              />
            )}
          />
          <Controller
            control={form.control}
            name="tax_no"
            render={({ field }) => (
              <Input
                label={t('tax_no')}
                placeholder="1234567890"
                value={field.value ?? ''}
                onChangeText={field.onChange}
                error={err('tax_no')}
                autoCapitalize="characters"
              />
            )}
          />
        </>
      ) : null}
      <Controller
        control={form.control}
        name="name"
        render={({ field }) => (
          <Input
            label={t(type === 'corporate' ? 'contact_first_name' : 'first_name')}
            placeholder={t('placeholder_first_name')} value={field.value} onChangeText={field.onChange} error={err('name')} autoCapitalize="words" />
        )}
      />
      <Controller
        control={form.control}
        name="surname"
        render={({ field }) => (
          <Input
            label={t(type === 'corporate' ? 'contact_surname' : 'surname')}
            placeholder={t('placeholder_surname')} value={field.value ?? ''} onChangeText={field.onChange} autoCapitalize="words" />
        )}
      />
      <Controller
        control={form.control}
        name="phone"
        render={({ field }) => (
          <Input
            label={t('phone')}
            placeholder={t('placeholder_phone', { ns: 'common' })}
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

export function VehicleStep({
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
        {t('vehicles_of', { name: customerTitle(customer) })}
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
  // Brand → model → optional year from the shared vehicle picker.
  vehicle: z.custom<VehicleValue>((v) => isComplete(v as VehicleValue)),
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
    defaultValues: { plate: '', vehicle: EMPTY_VEHICLE, vin: '' },
  });
  const create = useMutation({
    mutationFn: (v: VehicleForm) =>
      unwrap(
        api.POST('/v1/vehicles', {
          body: {
            customer_uuid: customerUuid,
            plate: v.plate.toUpperCase(),
            car_model_uuid: v.vehicle.model?.uuid,
            model_year: v.vehicle.year ?? undefined,
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
      if (f.model_year || f.car_model_uuid) form.setError('vehicle', { message: f.model_year ?? f.car_model_uuid });
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
            placeholder={t('placeholder_plate', { ns: 'common' })}
            value={field.value}
            onChangeText={field.onChange}
            error={err('plate')}
            autoCapitalize="characters"
            autoCorrect={false}
            trailing={<PlateScanButton onResult={field.onChange} />}
          />
        )}
      />
      <Controller
        control={form.control}
        name="vehicle"
        render={({ field }) => <VehiclePicker value={field.value} onChange={field.onChange} error={err('vehicle')} />}
      />
      <Controller
        control={form.control}
        name="vin"
        render={({ field }) => (
          <Input
            label={t('vin_optional')}
            placeholder={t('placeholder_vin', { ns: 'common' })}
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
