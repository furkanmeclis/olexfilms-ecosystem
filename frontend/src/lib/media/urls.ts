/**
 * Media URL rules (EWC-compatible):
 * - Never build SeaweedFS/S3 URLs from object keys in the browser.
 * - Only use `logo_url` / `public_url` (or auth-stream paths) returned by the API.
 * - Preview blobs from local File picks may use URL.createObjectURL; revoke after use.
 */

export function assertServiceMediaURL(
  url: string | null | undefined,
): string | null {
  if (!url) return null;
  const trimmed = url.trim();
  if (!trimmed) return null;
  // Allow relative API streams and absolute https URLs returned by the backend.
  if (
    trimmed.startsWith("/") ||
    trimmed.startsWith("https://") ||
    trimmed.startsWith("http://")
  ) {
    return trimmed;
  }
  throw new Error(
    "media: reject non-service URL (do not assemble S3 keys client-side)",
  );
}
