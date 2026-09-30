import type { ReactNode } from "react";

import {
  Breadcrumb,
  type BreadcrumbItem,
} from "@/components/layout/breadcrumb";

type PageHeaderProps = {
  title: string;
  description?: string;
  icon?: ReactNode;
  breadcrumbs?: BreadcrumbItem[];
  actions?: ReactNode;
};

export function PageHeader({
  title,
  description,
  icon,
  breadcrumbs,
  actions,
}: PageHeaderProps) {
  return (
    <div className="mb-2 space-y-3">
      {breadcrumbs?.length ? <Breadcrumb items={breadcrumbs} /> : null}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h1 className="font-display flex items-center gap-2.5 text-2xl font-semibold tracking-tight">
            {icon ? (
              <span className="text-muted-foreground inline-flex shrink-0">
                {icon}
              </span>
            ) : null}
            {title}
          </h1>
          {description ? (
            <p className="text-muted-foreground">{description}</p>
          ) : null}
        </div>
        {actions ? (
          <div className="flex items-center gap-2">{actions}</div>
        ) : null}
      </div>
    </div>
  );
}
