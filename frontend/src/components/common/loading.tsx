import { cn } from "@/lib/utils";

export function Loading({
  className,
  label,
}: {
  className?: string;
  label?: string;
}) {
  return (
    <div
      className={cn(
        "text-muted-foreground flex items-center justify-center gap-2 text-sm",
        className,
      )}
    >
      <span className="border-muted-foreground h-4 w-4 animate-spin rounded-full border-2 border-t-transparent" />
      {label}
    </div>
  );
}

export function PageLoader({ label }: { label?: string }) {
  return (
    <div className="flex min-h-[40vh] items-center justify-center">
      <Loading label={label} />
    </div>
  );
}
