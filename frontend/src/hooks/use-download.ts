"use client";

import { useCallback } from "react";

import { download } from "@/lib/utils";

export function useDownload() {
  return useCallback((blob: Blob, filename: string) => {
    download(blob, filename);
  }, []);
}
