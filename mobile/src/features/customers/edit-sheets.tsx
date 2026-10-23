import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { CustomerDetail, Vehicle } from '@/features/services/lib';
import { ModelPicker, type ModelValue, VIN_RE } from '@/features/services/new-service-screen';
import { Sheet } from '@/features/services/sheet';
import { PlateScanButton } from '@/features/plate-scan/plate-scan-button';
import { errorText } from '@/features/services/ui';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Edits name, e-mail and company data. The phone is the account key and is not
 * editable here. Fields of a customer shared with other dealers are only
 * filled when empty; the API lists the kept ones in ignored_fields.
 */
export function EditCustomerSheet({ customer, onClose }: { customer: CustomerDetail; onClose: () => void }) {
  const { t } = useTranslation('dealer');
  const qc = useQueryClient();
  const [name, setName] = useState(customer.name);
  const [surname, setSurname] = useState(customer.surname ?? '');
  const [email, setEmail] = useState(customer.email ?? '');
  const [company, setCompany] = useState(customer.company_name ?? '');
  const [taxOffice, setTaxOffice] = useState(customer.tax_office ?? '');
  const [errors, setErrors] = useState<Record<string, string>>({});

  const save = useMutation({
    mutationFn: () =>
      unwrap(
        api.PATCH('/v1/customers/{uuid}', {
          params: { path: { uuid: customer.uuid } },
          body: {
            name: name.trim(),
            surname: surname.trim() || null,
            email: email.trim() || null,
            ...(customer.type === 'corporate' || customer.company_name
              ? { company_name: company.trim() || null, tax_office: taxOffice.trim() || null }
              : {}),
          },
        }),
      ),
    onSuccess: (c) => {
      qc.setQueryData(['customers', 'detail', customer.uuid], c);
      void qc.invalidateQueries({ queryKey: ['customers', 'list'] });
      if (c.ignored_fields.length) Alert.alert(t('customer_saved'), t('customer_ignored_fields'));
      onClose();
    },
    onError: (e) => {
      if (isApiError(e) && Object.keys(e.fieldErrors()).length) setErrors(e.fieldErrors());
      else Alert.alert(t('action_failed'), errorText(e));
    },
  });

  const submit = () => {
    const next: Record<string, string> = {};
    if (!name.trim()) next.name = t('invalid.name');
    if (email.trim() && !EMAIL_RE.test(email.trim())) next.email = t('invalid.email');
    setErrors(next);
    if (Object.keys(next).length === 0) save.mutate();
  };

  return (
    <Sheet title={t('customer_edit')} busy={save.isPending} onClose={onClose} onSave={submit}>
      {!customer.identity_editable ? (
        <Text variant="footnote" tone="muted">
          {t('customer_shared_note')}
        </Text>
      ) : null}
      <Input label={t('first_name')} placeholder={t('placeholder_first_name')} value={name} onChangeText={setName} error={errors.name} autoCapitalize="words" />
      <Input label={t('surname')} placeholder={t('placeholder_surname')} value={surname} onChangeText={setSurname} error={errors.surname} autoCapitalize="words" />
      <Input
        label={t('email')}
        placeholder={t('placeholder_email', { ns: 'common' })}
        value={email}
        onChangeText={setEmail}
        error={errors.email}
        keyboardType="email-address"
        autoCapitalize="none"
        autoCorrect={false}
      />
      {customer.type === 'corporate' || customer.company_name ? (
        <>
          <Input label={t('company_name')} placeholder={t('placeholder_company_name')} value={company} onChangeText={setCompany} error={errors.company_name} />
          <Input label={t('tax_office')} placeholder={t('placeholder_tax_office')} value={taxOffice} onChangeText={setTaxOffice} error={errors.tax_office} />
        </>
      ) : null}
      {/* Read-only: the placeholder only shows when the customer has no phone, so it says that instead of an example number. */}
      <Input label={t('phone')} placeholder={t('phone_none')} value={customer.phone ?? ''} editable={false} hint={t('phone_locked')} />
    </Sheet>
  );
}

/** Edits plate, brand / model, year and VIN of a vehicle (PATCH /v1/vehicles/{uuid}). */
export function EditVehicleSheet({ vehicle, onClose }: { vehicle: Vehicle; onClose: () => void }) {
  const { t } = useTranslation('dealer');
  const qc = useQueryClient();
  const [plate, setPlate] = useState(vehicle.plate ?? '');
  const [model, setModel] = useState<ModelValue>(
    vehicle.car_model ? { uuid: vehicle.car_model.uuid, name: vehicle.car_model.name, brand: vehicle.car_brand?.name ?? '' } : null,
  );
  const [year, setYear] = useState(vehicle.model_year ? String(vehicle.model_year) : '');
  const [vin, setVin] = useState(vehicle.vin ?? '');
  const [errors, setErrors] = useState<Record<string, string>>({});

  const save = useMutation({
    mutationFn: () =>
      unwrap(
        api.PATCH('/v1/vehicles/{uuid}', {
          params: { path: { uuid: vehicle.uuid } },
          body: {
            plate: plate.trim().toUpperCase(),
            ...(model && model.uuid !== vehicle.car_model?.uuid ? { car_model_uuid: model.uuid } : {}),
            model_year: year ? Number(year) : null,
            vin: vin.trim() ? vin.trim().toUpperCase() : null,
          },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['vehicles'] });
      onClose();
    },
    onError: (e) => {
      if (!isApiError(e)) return Alert.alert(t('action_failed'), errorText(e));
      const f = e.fieldErrors();
      if (e.code === 'INVALID_PLATE' || f.plate) setErrors({ plate: t('invalid.plate') });
      else if (f.vin) setErrors({ vin: t('invalid.vin') });
      else if (f.model_year) setErrors({ year: t('invalid.year') });
      else Alert.alert(t('action_failed'), errorText(e));
    },
  });

  const submit = () => {
    const next: Record<string, string> = {};
    if (plate.trim().length < 2) next.plate = t('invalid.plate');
    if (!model) next.model = t('invalid.model');
    const y = Number(year);
    if (year && (!/^\d{4}$/.test(year) || y < 1900 || y > new Date().getFullYear() + 1)) next.year = t('invalid.year');
    if (vin.trim() && !VIN_RE.test(vin.trim().toUpperCase())) next.vin = t('invalid.vin');
    setErrors(next);
    if (Object.keys(next).length === 0) save.mutate();
  };

  return (
    <Sheet title={t('vehicle_edit')} busy={save.isPending} onClose={onClose} onSave={submit}>
      <Input
        label={t('plate')}
        placeholder={t('placeholder_plate', { ns: 'common' })}
        value={plate}
        onChangeText={setPlate}
        error={errors.plate}
        autoCapitalize="characters"
        autoCorrect={false}
        trailing={<PlateScanButton onResult={setPlate} />}
      />
      <ModelPicker value={model} onChange={setModel} error={errors.model} />
      <Input label={t('model_year')} placeholder={t('placeholder_year', { ns: 'common' })} value={year} onChangeText={setYear} error={errors.year} keyboardType="number-pad" maxLength={4} />
      <Input
        label={t('vin_optional')}
        placeholder={t('placeholder_vin', { ns: 'common' })}
        value={vin}
        onChangeText={setVin}
        error={errors.vin}
        autoCapitalize="characters"
        autoCorrect={false}
        maxLength={17}
      />
    </Sheet>
  );
}
