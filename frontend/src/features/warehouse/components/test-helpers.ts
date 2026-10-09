import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, createElement, type ReactElement } from "react";
import { createRoot, type Root } from "react-dom/client";

import { chooseValue, installRadixPolyfills } from "@/test/form-controls";

/** Test-only helpers for the warehouse component tests (jsdom). */

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;
installRadixPolyfills();

export type Mounted = {
  container: HTMLDivElement;
  root: Root;
};

export function mount(): Mounted {
  const container = document.createElement("div");
  document.body.appendChild(container);
  return { container, root: createRoot(container) };
}

export function unmount(m: Mounted) {
  act(() => m.root.unmount());
  m.container.remove();
}

export async function flush(rounds = 5) {
  for (let i = 0; i < rounds; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}

export async function render(m: Mounted, node: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  await act(async () => {
    m.root.render(createElement(QueryClientProvider, { client }, node));
  });
  await flush();
}

/**
 * Sets an input value the way React notices it; on a Select / combobox
 * trigger it opens the list and picks the option with that value.
 */
export async function fill(el: Element | null, value: string) {
  if (!el) throw new Error("element not found");
  if (!(el instanceof HTMLInputElement || el instanceof HTMLSelectElement)) {
    await chooseValue(el, value);
    return;
  }
  const proto =
    el instanceof HTMLSelectElement
      ? HTMLSelectElement.prototype
      : HTMLInputElement.prototype;
  await act(async () => {
    Object.getOwnPropertyDescriptor(proto, "value")?.set?.call(el, value);
    el.dispatchEvent(
      new Event(el instanceof HTMLSelectElement ? "change" : "input", {
        bubbles: true,
      }),
    );
  });
}

export async function click(el: Element | null) {
  if (!el) throw new Error("element not found");
  await act(async () => {
    (el as HTMLElement).click();
  });
  await flush();
}

export async function submit(form: Element | null) {
  if (!form) throw new Error("form not found");
  await act(async () => {
    (form as HTMLFormElement).requestSubmit();
  });
  await flush();
}
