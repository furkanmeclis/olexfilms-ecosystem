import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api";
import { withStepUpRetry } from "@/features/step-up-engine/lib/step-up-interceptor";

type PlatformRequestOptions = {
  query?: Record<string, string | number | undefined>;
  body?: unknown;
};

/** BFF fetch for platform endpoints not yet in generated OpenAPI paths. */
export async function platformRequest<T>(
  method: string,
  path: string,
  options?: PlatformRequestOptions,
): Promise<T> {
  return withStepUpRetry(async () => {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const pathname = path.startsWith("/") ? path : `/${path}`;
    let url = `${base}${pathname}`;

    if (options?.query) {
      const params = new URLSearchParams();
      for (const [key, value] of Object.entries(options.query)) {
        if (value !== undefined && value !== "") {
          params.set(key, String(value));
        }
      }
      const qs = params.toString();
      if (qs) url += `?${qs}`;
    }

    const response = await fetch(url, {
      method,
      credentials: "include",
      headers: {
        Accept: "application/json",
        ...(options?.body ? { "Content-Type": "application/json" } : {}),
      },
      body: options?.body ? JSON.stringify(options.body) : undefined,
    });

    const payload = await response.json().catch(() => undefined);
    return unwrap<T>({ data: payload, response });
  });
}
