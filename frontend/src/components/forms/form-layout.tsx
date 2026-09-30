"use client";

import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { useActiveSection } from "@/components/forms/use-active-section";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export type FormNavItem = {
  id: string;
  label: string;
  description?: string;
  icon?: LucideIcon;
};

type FormLayoutProps = {
  children: ReactNode;
  /** Preferred: auto-renders FormNav in sidebar + mobile strip */
  navItems?: FormNavItem[];
  /** Optional nav header label (defaults to i18n form.on_this_page) */
  navTitle?: string;
  /** Escape hatch for fully custom aside content */
  aside?: ReactNode;
  className?: string;
};

/**
 * Two-column form page shell: optional aside + main content.
 * Use with FormSection + FormActions for create/edit screens.
 */
export function FormLayout({
  children,
  navItems,
  navTitle,
  aside,
  className,
}: FormLayoutProps) {
  const showNav = Boolean(navItems?.length);
  const showAside = showNav || Boolean(aside);

  return (
    <div
      className={cn(
        "grid gap-6",
        showAside
          ? "lg:grid-cols-[240px_minmax(0,1fr)] xl:grid-cols-[260px_minmax(0,1fr)]"
          : null,
        className,
      )}
    >
      {showAside ? (
        <aside className="hidden lg:block">
          <div className="sticky top-20">
            {showNav ? (
              <FormNav items={navItems!} title={navTitle} variant="sidebar" />
            ) : (
              aside
            )}
          </div>
        </aside>
      ) : null}

      <div className="min-w-0 space-y-6">
        {showNav ? (
          <div className="lg:hidden">
            <FormNav items={navItems!} title={navTitle} variant="pills" />
          </div>
        ) : null}
        {children}
      </div>
    </div>
  );
}

type FormNavProps = {
  items: FormNavItem[];
  title?: string;
  variant?: "sidebar" | "pills";
  className?: string;
};

export function FormNav({
  items,
  title,
  variant = "sidebar",
  className,
}: FormNavProps) {
  const { t } = useLocale();
  const activeId = useActiveSection(items.map((item) => item.id));
  const heading = title ?? t("form.on_this_page");

  const scrollTo = (id: string) => {
    const el = document.getElementById(id);
    if (!el) return;
    // Scroll only — never write readable labels into the URL hash
    el.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  if (variant === "pills") {
    return (
      <nav
        aria-label={heading}
        className={cn(
          "-mx-1 flex [scrollbar-width:none] gap-1.5 overflow-x-auto px-1 pb-1 [&::-webkit-scrollbar]:hidden",
          className,
        )}
      >
        {items.map((item) => {
          const Icon = item.icon;
          const active = activeId === item.id;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => scrollTo(item.id)}
              aria-current={active ? "true" : undefined}
              className={cn(
                "inline-flex shrink-0 items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition-colors",
                active
                  ? "border-primary/40 bg-primary/10 text-primary font-medium"
                  : "border-border bg-card text-muted-foreground hover:border-border hover:bg-muted hover:text-foreground",
              )}
            >
              {Icon ? <Icon className="size-3.5 shrink-0" aria-hidden /> : null}
              {item.label}
            </button>
          );
        })}
      </nav>
    );
  }

  return (
    <nav
      aria-label={heading}
      className={cn(
        "border-border bg-card rounded-xl border p-2 shadow-sm",
        className,
      )}
    >
      <p className="text-muted-foreground px-2.5 pt-1.5 pb-2 text-[11px] font-semibold tracking-wider uppercase">
        {heading}
      </p>
      <ul className="space-y-0.5">
        {items.map((item) => {
          const Icon = item.icon;
          const active = activeId === item.id;
          return (
            <li key={item.id}>
              <button
                type="button"
                onClick={() => scrollTo(item.id)}
                aria-current={active ? "true" : undefined}
                className={cn(
                  "group relative flex w-full items-start gap-2.5 rounded-lg px-2.5 py-2 text-start transition-colors",
                  active
                    ? "bg-primary/10 text-primary"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <span
                  className={cn(
                    "bg-primary absolute inset-y-1.5 start-0 w-0.5 rounded-full transition-opacity",
                    active ? "opacity-100" : "opacity-0",
                  )}
                  aria-hidden
                />
                <span
                  className={cn(
                    "mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md border transition-colors",
                    active
                      ? "border-primary/30 bg-primary/15 text-primary"
                      : "border-border bg-background text-muted-foreground group-hover:border-border group-hover:text-foreground",
                  )}
                >
                  {Icon ? (
                    <Icon className="size-3.5" aria-hidden />
                  ) : (
                    <span className="size-1.5 rounded-full bg-current" />
                  )}
                </span>
                <span className="min-w-0 flex-1">
                  <span
                    className={cn(
                      "block text-sm leading-snug",
                      active ? "font-semibold" : "font-medium",
                    )}
                  >
                    {item.label}
                  </span>
                  {item.description ? (
                    <span
                      className={cn(
                        "mt-0.5 block text-xs leading-snug",
                        active ? "text-primary/80" : "text-muted-foreground",
                      )}
                    >
                      {item.description}
                    </span>
                  ) : null}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
