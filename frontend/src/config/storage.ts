export const storageConfig = {
  maxFileSizeBytes: 10 * 1024 * 1024,
  /** Tenant / workspace logo upload cap (B033 / API multipart). */
  logoMaxFileSizeBytes: 2 * 1024 * 1024,
  acceptedImageTypes: ["image/jpeg", "image/png", "image/webp"] as const,
  /** Abstraction seam — MinIO/S3 uploads go through API endpoints */
  provider: "api" as const,
} as const;
