import type { LucideIcon } from "lucide-react";

import type { FormNavItem } from "@/components/forms/form-layout";

export type FormSectionDef = {
  /** Stable developer key — never used as a URL hash / DOM slug */
  key: string;
  label: string;
  description?: string;
  icon?: LucideIcon;
};

export type FormSections = {
  navItems: FormNavItem[];
  /** Resolve opaque DOM id for a section key */
  id: (key: string) => string;
};

/** FNV-1a 32-bit → base36, opaque & stable across locales */
function opaqueSectionId(key: string, index: number): string {
  let hash = 2166136261;
  const input = `${index}:${key}`;
  for (let i = 0; i < input.length; i += 1) {
    hash ^= input.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return `fs-${(hash >>> 0).toString(36)}`;
}

/**
 * Builds nav items with algorithmically generated DOM ids.
 * Labels stay human-readable; ids stay opaque (no `#iletisim` style hashes).
 */
export function createFormSections(defs: FormSectionDef[]): FormSections {
  const byKey = new Map<string, string>();

  const navItems: FormNavItem[] = defs.map((def, index) => {
    if (byKey.has(def.key)) {
      throw new Error(`Duplicate form section key: "${def.key}"`);
    }
    const id = opaqueSectionId(def.key, index);
    byKey.set(def.key, id);
    return {
      id,
      label: def.label,
      description: def.description,
      icon: def.icon,
    };
  });

  return {
    navItems,
    id: (key: string) => {
      const value = byKey.get(key);
      if (!value) {
        throw new Error(`Unknown form section key: "${key}"`);
      }
      return value;
    },
  };
}
