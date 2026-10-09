import type { Locator } from "@playwright/test";

/**
 * Helpers for the shared shadcn pickers (Radix Select and the popover
 * comboboxes). Options render in a portal; SelectItem and cmdk items both
 * carry their value in `data-value`.
 */

/** Opens a Select / combobox and clicks the option with that value. */
export async function chooseOption(trigger: Locator, value: string) {
  await trigger.click();
  await trigger
    .page()
    .locator(`[role="option"][data-value="${value}"]`)
    .click();
}

/** Opens a Select / combobox and clicks the option with that exact label. */
export async function chooseOptionByLabel(trigger: Locator, label: string) {
  await trigger.click();
  await trigger
    .page()
    .getByRole("option", { name: label, exact: true })
    .click();
}

/** Opens a Select / combobox; returns the open options (close with Escape). */
export async function openOptions(trigger: Locator) {
  await trigger.click();
  return trigger.page().getByRole("option");
}
