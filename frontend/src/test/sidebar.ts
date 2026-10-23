/**
 * jsdom helper for components that render inside the shadcn sidebar
 * (SidebarMenuButton needs the SidebarProvider context, which reads the
 * viewport through matchMedia).
 */
import { createElement, type ReactNode } from "react";

import { SidebarProvider } from "@/components/ui/sidebar";

/** jsdom has no matchMedia: report a desktop viewport. */
export function installMatchMedia() {
  window.matchMedia ??= ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

/** Wraps node in a SidebarProvider (installs matchMedia first). */
export function withSidebar(node: ReactNode) {
  installMatchMedia();
  return createElement(SidebarProvider, null, node);
}
