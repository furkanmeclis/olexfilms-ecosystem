// Widget bağlantıları. Saf dosya: tek şema `olexfilms://`, yollar `src/app/(app)` altındaki rotalardır.

export const SCHEME = 'olexfilms://';

const url = (path: string) => `${SCHEME}${path}`;

export const links = {
  home: (path = '') => url(path),
  vehicle: (uuid: string) => url(`musteri/vehicle/${uuid}`),
  service: (uuid: string) => url(`musteri/service/${uuid}`),
  bayi: {
    home: () => url('bayi'),
    scan: () => url('bayi/scan'),
    newService: () => url('bayi/services/new'),
    measure: () => url('bayi/measure'),
    customers: () => url('bayi/menu/customers'),
    services: () => url('bayi/services'),
    service: (uuid: string) => url(`bayi/services/${uuid}`),
    reports: () => url('bayi/menu/reports'),
    claims: () => url('bayi/menu/warranty_claims'),
    warranties: () => url('bayi/menu/warranties'),
    accounting: () => url('bayi/menu/accounting'),
  },
};
