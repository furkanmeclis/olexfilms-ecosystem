import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

export type SearchSpecDef = {
  id: string;
  labelKey: string;
  icon?: LucideIcon;
  permission?: string | string[];
  anyPermission?: string[];
  /** Local-only specs (nav pages) skip remote API. */
  local?: boolean;
};

export type RemoteSearchSpec = {
  id: string;
  label_key: string;
  permission?: string;
  icon?: string;
  tenant_scoped?: boolean;
  /** Documents carry brand_id; search filters on the active brand. */
  brand_scoped?: boolean;
};

export type SearchHit = {
  spec: string;
  id: string;
  title: string;
  subtitle?: string;
  href: string;
  icon?: string;
  score?: number;
};

export type PaletteItem = {
  id: string;
  spec: string;
  label: string;
  description?: string;
  href: string;
  icon?: ReactNode;
  /** Serializable icon id for localStorage restore (see resolveSearchIcon). */
  iconKey?: string;
  group: string;
};

export type SearchCatalog = {
  id: string;
  specs: SearchSpecDef[];
};
