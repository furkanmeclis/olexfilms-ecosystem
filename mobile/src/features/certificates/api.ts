import { api, unwrap, uploadForm } from '@/core/api/client';
import { type Page, PAGE_SIZE } from '@/features/admin/kit';

import type { Certificate, CertificateWarning } from './lib';

type Call<T> = Promise<{ data?: { data: T }; error?: unknown; response: Response }>;

/**
 * The certificate routes have no response schema in the OpenAPI document yet,
 * so the typed client cannot describe them: this is the same client with the
 * shapes of the web service (`features/certificates`).
 */
const loose = api as unknown as {
  GET: <T>(path: string, init: { params: { query: Record<string, unknown> } }) => Call<T>;
  POST: <T>(path: string, init: { params: { path: Record<string, string> } }) => Call<T>;
};

/** The API filters nothing by status, so pages are larger to keep a filtered view useful. */
export const CERT_PAGE_SIZE = PAGE_SIZE * 2;

export const listCertificates = (offset: number) =>
  unwrap(loose.GET<Page<Certificate>>('/v1/certificates', { params: { query: { limit: CERT_PAGE_SIZE, offset } } }));

export const listWarnings = (offset: number) =>
  unwrap(
    loose.GET<Page<CertificateWarning>>('/v1/certificate-warnings', {
      params: { query: { limit: CERT_PAGE_SIZE, offset } },
    }),
  );

export const verifyCertificate = (uuid: string) =>
  unwrap(loose.POST<Certificate>('/v1/certificates/{uuid}/verify', { params: { path: { uuid } } }));

function form(field: string, value: string) {
  const f = new FormData();
  f.append(field, value);
  return f;
}

export const rejectCertificate = (uuid: string, reason: string) =>
  uploadForm<Certificate>(`/v1/certificates/${encodeURIComponent(uuid)}/reject`, form('reason', reason));

export const decideWarning = (uuid: string, approve: boolean, note: string) =>
  uploadForm<CertificateWarning>(
    `/v1/certificate-warnings/${encodeURIComponent(uuid)}/${approve ? 'approve' : 'reject'}`,
    form('note', note),
  );
