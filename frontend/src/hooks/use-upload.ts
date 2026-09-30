"use client";

import { useCallback, useState } from "react";

import type { UploadFile } from "@/components/upload/file-uploader";

export function useUpload() {
  const [files, setFiles] = useState<UploadFile[]>([]);

  const reset = useCallback(() => setFiles([]), []);

  return { files, setFiles, reset };
}
