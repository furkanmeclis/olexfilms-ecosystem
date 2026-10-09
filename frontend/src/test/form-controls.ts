/**
 * jsdom helpers for the shared shadcn form controls (Radix Select, the
 * popover comboboxes and the date/time/month pickers). Tests in this repo
 * drive the DOM directly, so these keep the open → pick sequence in one place.
 */
import { act } from "react";

/** Radix Select / Popover need pointer-capture, scrollIntoView and ResizeObserver. */
export function installRadixPolyfills() {
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.hasPointerCapture ??= () => false;
  proto.setPointerCapture ??= () => {};
  proto.releasePointerCapture ??= () => {};
  proto.scrollIntoView ??= () => {};
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

function optionText(el: Element): string {
  return (el.textContent ?? "").replace(/\s+/g, " ").trim();
}

function findOption(label: string | RegExp, root: ParentNode = document) {
  const options = [...root.querySelectorAll('[role="option"]')];
  return options.find((el) =>
    typeof label === "string"
      ? optionText(el) === label || optionText(el).startsWith(`${label} `)
      : label.test(optionText(el)),
  );
}

/** Lists the visible option labels of the currently open listbox(es). */
export function openOptions(): string[] {
  return [...document.querySelectorAll('[role="option"]')].map(optionText);
}

/** Opens a Select / combobox trigger. */
export async function openPicker(trigger: Element | null) {
  if (!trigger) throw new Error("picker trigger not found");
  await act(async () => {
    (trigger as HTMLElement).click();
  });
}

/**
 * Opens a Select / combobox trigger and clicks the option whose text matches
 * `label` (exact text, or a RegExp).
 */
export async function pickOption(
  trigger: Element | null,
  label: string | RegExp,
) {
  await openPicker(trigger);
  const option = findOption(label);
  if (!option) {
    throw new Error(
      `option ${String(label)} not found; open options: ${openOptions().join(" | ")}`,
    );
  }
  await act(async () => {
    (option as HTMLElement).click();
  });
}

function lastListboxes(count: number): Element[] {
  const all = [...document.querySelectorAll('[role="listbox"]')];
  return all.slice(-count);
}

/** Opens a TimePicker and picks `HH:mm`. */
export async function pickTime(trigger: Element | null, value: string) {
  const [hour, minute] = value.split(":");
  await openPicker(trigger);
  await clickTimeColumns(hour, minute);
  // Close the popover again so the next picker opens cleanly.
  await act(async () => {
    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
  });
}

async function clickTimeColumns(hour: string, minute: string) {
  const [hours, minutes] = lastListboxes(2);
  const h = findOption(hour, hours);
  if (!h) throw new Error(`hour ${hour} not found`);
  await act(async () => {
    (h as HTMLElement).click();
  });
  const [, minutesAfter] = lastListboxes(2);
  const m = findOption(minute, minutesAfter ?? minutes);
  if (!m) throw new Error(`minute ${minute} not found`);
  await act(async () => {
    (m as HTMLElement).click();
  });
}

/** Opens a MonthPicker and picks `yyyy-MM`. */
export async function pickMonth(trigger: Element | null, value: string) {
  await openPicker(trigger);
  const year = value.slice(0, 4);
  for (let i = 0; i < 50; i++) {
    const grid = [...document.querySelectorAll('[role="listbox"]')].at(-1);
    const shown = grid?.getAttribute("aria-label");
    if (shown === year) break;
    const nav = grid?.parentElement?.querySelectorAll("button");
    const button = Number(shown) < Number(year) ? nav?.[1] : nav?.[0];
    await act(async () => {
      button?.click();
    });
  }
  const month = document.querySelector(`[data-value="${value}"]`);
  if (!month) throw new Error(`month ${value} not found`);
  await act(async () => {
    (month as HTMLElement).click();
  });
}

/**
 * Opens a DateTimePicker and picks `yyyy-MM-ddTHH:mm`: the day in the
 * calendar (must be in the month shown) then the time columns.
 */
export async function pickDateTime(trigger: Element | null, value: string) {
  const [date, time] = value.split("T");
  await openPicker(trigger);
  await clickDay(date);
  const [hour, minute] = time.split(":");
  await clickTimeColumns(hour, minute);
  await act(async () => {
    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
  });
}

/** Opens a DatePicker and picks `yyyy-MM-dd` (must be in the month shown). */
export async function pickDate(trigger: Element | null, value: string) {
  await openPicker(trigger);
  await clickDay(value);
}

async function clickDay(date: string) {
  const cell = document.querySelector(`[data-day="${date}"]`);
  const button = cell?.querySelector("button") ?? cell;
  if (!button) throw new Error(`day ${date} not shown in calendar`);
  await act(async () => {
    (button as HTMLElement).click();
  });
}
