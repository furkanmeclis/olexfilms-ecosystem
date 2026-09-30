import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api";

/** Multipart BFF fetch for platform endpoints not yet in generated OpenAPI paths. */
export async function platformFormRequest<T>(
  method: string,
  path: string,
  formData: FormData,
): Promise<T> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const pathname = path.startsWith("/") ? path : `/${path}`;
  const url = `${base}${pathname}`;

  const response = await fetch(url, {
    method,
    credentials: "include",
    body: formData,
  });

  const payload = await response.json().catch(() => undefined);
  return unwrap<T>({ data: payload, response });
}

/** Binary download via BFF (export sample / completed job). */
export async function platformDownloadRequest(
  path: string,
  query?: Record<string, string | undefined>,
): Promise<Blob> {
  const { blob } = await platformDownloadFile(path, query);
  return blob;
}

function parseContentDispositionFilename(header: string | null): string | null {
  if (!header) return null;
  const star = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (star?.[1]) {
    try {
      return decodeURIComponent(star[1].trim());
    } catch {
      return star[1].trim();
    }
  }
  const plain = /filename="([^"]+)"/i.exec(header);
  if (plain?.[1]) return plain[1];
  const unquoted = /filename=([^;]+)/i.exec(header);
  return unquoted?.[1]?.trim() ?? null;
}

/** Download binary file with filename from Content-Disposition when available. */
export async function platformDownloadFile(
  path: string,
  query?: Record<string, string | undefined>,
): Promise<{ blob: Blob; filename: string | null }> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  let url = `${base}${path.startsWith("/") ? path : `/${path}`}`;
  if (query) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value) params.set(key, value);
    }
    const qs = params.toString();
    if (qs) url += `?${qs}`;
  }
  const response = await fetch(url, { credentials: "include" });
  if (!response.ok) {
    throw new Error(`Download failed (${response.status})`);
  }
  const blob = await response.blob();
  const filename = parseContentDispositionFilename(
    response.headers.get("Content-Disposition"),
  );
  return { blob, filename };
}

export function triggerBrowserDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
