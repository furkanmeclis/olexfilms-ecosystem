"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ChevronLeft,
  ChevronRight,
  ImageIcon,
  Loader2,
  Trash2,
  Upload,
} from "lucide-react";
import { useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { catalogKeys } from "@/features/catalog/hooks/use-catalog-access";
import {
  catalogService,
  PRODUCT_IMAGE_MAX,
  PRODUCT_IMAGE_MAX_BYTES,
  PRODUCT_IMAGE_TYPES,
  productImageUrl,
  type CatalogProduct,
  type CatalogProductImage,
} from "@/features/catalog/services/catalog.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function sortedImages(
  images: CatalogProductImage[] | undefined,
): CatalogProductImage[] {
  return [...(images ?? [])].sort((a, b) => a.sort - b.sort);
}

/** Moves the item at from to to (both clamped indices). */
export function moveKey(keys: string[], from: number, to: number): string[] {
  const next = [...keys];
  const [item] = next.splice(from, 1);
  if (item === undefined) return keys;
  next.splice(Math.max(0, Math.min(to, next.length)), 0, item);
  return next;
}

/**
 * Product images (TEC-152): drag-and-drop or picked JPEG/PNG/WebP uploads,
 * preview, removal and reordering. Every change goes straight to the image
 * routes; the product form never sends `images`.
 */
export function ProductImagesField({
  product,
  readOnly = false,
}: {
  product?: CatalogProduct;
  readOnly?: boolean;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [dragKey, setDragKey] = useState<string | null>(null);
  const [current, setCurrent] = useState<CatalogProduct | undefined>(product);
  const [prevProduct, setPrevProduct] = useState(product);
  if (product !== prevProduct) {
    setPrevProduct(product);
    setCurrent(product);
  }

  const images = sortedImages(current?.images);
  const uuid = current?.uuid;

  const applied = async (next: CatalogProduct) => {
    setCurrent(next);
    queryClient.setQueryData(catalogKeys.product(next.uuid), next);
    await queryClient.invalidateQueries({ queryKey: catalogKeys.all });
  };

  const failed = (error: unknown) => {
    if (isApiError(error)) {
      if (error.status === 413)
        return appToast.error(t("catalog.images.too_large"));
      if (error.status === 400)
        return appToast.error(t("catalog.images.invalid_type"));
      if (error.status === 422) {
        return appToast.error(
          t("catalog.images.too_many", { max: PRODUCT_IMAGE_MAX }),
        );
      }
      return appToast.error(error.message);
    }
    appToast.error(t("catalog.toast.failed"));
  };

  const upload = useMutation({
    mutationFn: async (files: File[]) => {
      let last: CatalogProduct | undefined;
      for (const file of files) {
        last = await catalogService.uploadProductImage(uuid ?? "", file);
        await applied(last);
      }
      return last;
    },
    onSuccess: (last) => {
      if (last) appToast.success(t("catalog.images.uploaded"));
    },
    onError: failed,
  });

  const remove = useMutation({
    mutationFn: (key: string) =>
      catalogService.deleteProductImage(uuid ?? "", key),
    onSuccess: async (next) => {
      await applied(next);
      appToast.success(t("catalog.images.removed"));
    },
    onError: failed,
  });

  const reorder = useMutation({
    mutationFn: (keys: string[]) =>
      catalogService.reorderProductImages(uuid ?? "", keys),
    onSuccess: applied,
    onError: failed,
  });

  const busy = upload.isPending || remove.isPending || reorder.isPending;
  const disabled = readOnly || !uuid || busy;

  const ingest = (list: FileList | null) => {
    if (!list?.length || disabled) return;
    const files = Array.from(list);
    if (files.some((f) => !PRODUCT_IMAGE_TYPES.includes(f.type))) {
      appToast.error(t("catalog.images.invalid_type"));
      return;
    }
    if (files.some((f) => f.size > PRODUCT_IMAGE_MAX_BYTES)) {
      appToast.error(t("catalog.images.too_large"));
      return;
    }
    if (images.length + files.length > PRODUCT_IMAGE_MAX) {
      appToast.error(t("catalog.images.too_many", { max: PRODUCT_IMAGE_MAX }));
      return;
    }
    upload.mutate(files);
  };

  const keys = images.map((img) => img.key);
  const move = (from: number, to: number) => {
    if (to < 0 || to >= keys.length || from === to) return;
    reorder.mutate(moveKey(keys, from, to));
  };

  if (!uuid) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("catalog.images.save_first")}
      </p>
    );
  }

  return (
    <div className="space-y-3">
      {images.length ? (
        <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          {images.map((img, index) => {
            const url = productImageUrl(img.key);
            return (
              <li
                key={img.key}
                draggable={!disabled}
                onDragStart={() => setDragKey(img.key)}
                onDragEnd={() => setDragKey(null)}
                onDragOver={(e) => {
                  if (dragKey) e.preventDefault();
                }}
                onDrop={(e) => {
                  if (!dragKey) return;
                  e.preventDefault();
                  e.stopPropagation();
                  move(keys.indexOf(dragKey), index);
                  setDragKey(null);
                }}
                className={cn(
                  "bg-card relative overflow-hidden rounded-md border",
                  dragKey === img.key && "opacity-50",
                )}
              >
                <div className="bg-muted flex aspect-square items-center justify-center">
                  {url ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={url}
                      alt={t("catalog.images.alt", { n: index + 1 })}
                      loading="lazy"
                      className="size-full object-cover"
                    />
                  ) : (
                    <ImageIcon
                      className="text-muted-foreground size-6"
                      aria-label={img.key}
                    />
                  )}
                </div>
                {index === 0 ? (
                  <Badge className="absolute start-2 top-2">
                    {t("catalog.images.cover")}
                  </Badge>
                ) : null}
                {readOnly ? null : (
                  <div className="flex items-center justify-between gap-1 p-1">
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      aria-label={t("catalog.images.move_earlier")}
                      disabled={disabled || index === 0}
                      onClick={() => move(index, index - 1)}
                    >
                      <ChevronLeft className="size-4 rtl:rotate-180" />
                    </Button>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      aria-label={t("catalog.images.remove")}
                      disabled={disabled}
                      onClick={() => remove.mutate(img.key)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      aria-label={t("catalog.images.move_later")}
                      disabled={disabled || index === images.length - 1}
                      onClick={() => move(index, index + 1)}
                    >
                      <ChevronRight className="size-4 rtl:rotate-180" />
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      ) : null}

      {readOnly || images.length >= PRODUCT_IMAGE_MAX ? null : (
        <button
          type="button"
          disabled={disabled}
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            if (dragKey) return;
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            if (dragKey) return;
            e.preventDefault();
            setDragging(false);
            ingest(e.dataTransfer.files);
          }}
          className={cn(
            "border-border text-muted-foreground flex w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-6 py-8 text-center text-sm transition-colors disabled:opacity-60",
            dragging && "border-primary bg-muted/40",
          )}
        >
          {upload.isPending ? (
            <Loader2 className="size-5 animate-spin" />
          ) : (
            <Upload className="size-5" />
          )}
          <span>
            {upload.isPending
              ? t("catalog.images.uploading")
              : t("catalog.images.drop")}
          </span>
        </button>
      )}
      <input
        ref={inputRef}
        type="file"
        className="hidden"
        multiple
        accept={PRODUCT_IMAGE_TYPES.join(",")}
        data-testid="product-image-input"
        onChange={(e) => {
          ingest(e.target.files);
          e.target.value = "";
        }}
      />
    </div>
  );
}
