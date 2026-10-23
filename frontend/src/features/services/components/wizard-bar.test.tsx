// @vitest-environment jsdom
import { act, createElement, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params ? `${key} ${JSON.stringify(params)}` : key,
    locale: "tr",
  }),
}));

import type { WizardStep } from "@/features/services/lib/wizard";

import {
  WizardActions,
  WizardActionsProvider,
  WizardBottomBar,
} from "./wizard-bar";

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

const STEPS: readonly WizardStep[] = [
  "customer_vehicle",
  "photos",
  "parts",
  "measurement",
  "contract",
  "stock",
];

type Props = {
  current?: WizardStep;
  hasService?: boolean;
  contractLocked?: boolean;
  photosLocked?: boolean;
  onSelect?: (step: WizardStep) => void;
  onBack?: () => void;
  onNext?: () => void;
  nextDisabled?: boolean;
};

function Harness({
  current = "parts",
  hasService = true,
  contractLocked = false,
  photosLocked = false,
  onSelect = () => {},
  onBack = () => {},
  onNext = () => {},
  nextDisabled = false,
}: Props) {
  const [slot, setSlot] = useState<HTMLDivElement | null>(null);
  return createElement(
    "div",
    null,
    createElement(
      WizardActionsProvider,
      { value: slot },
      createElement(
        "section",
        { "data-testid": "step-body" },
        createElement(
          WizardActions,
          null,
          createElement(
            "button",
            { type: "button", "data-testid": "back", onClick: onBack },
            "back",
          ),
          createElement(
            "button",
            {
              type: "button",
              "data-testid": "next",
              disabled: nextDisabled,
              onClick: onNext,
            },
            "next",
          ),
        ),
      ),
    ),
    createElement(WizardBottomBar, {
      steps: STEPS,
      current,
      hasService,
      contractLocked,
      photosLocked,
      onSelect,
      actionsRef: setSlot,
    }),
  );
}

function render(props: Props = {}) {
  act(() => root.render(createElement(Harness, props)));
}

const $ = (sel: string) => container.querySelector(sel);
const step = (s: WizardStep) =>
  $(`[data-testid="wizard-stepper"] [data-step="${s}"]`) as HTMLButtonElement;
const click = (el: Element | null) =>
  act(() =>
    (el as HTMLElement).dispatchEvent(
      new MouseEvent("click", { bubbles: true }),
    ),
  );

describe("WizardBottomBar (TEC-865)", () => {
  it("shows the progress and every step inside a nav landmark", () => {
    render({ current: "parts" });
    const nav = $('[data-testid="wizard-bar"] nav');
    expect(nav?.getAttribute("aria-label")).toBe("services.wizard.bar.label");
    expect($('[data-testid="wizard-progress"]')?.textContent).toContain(
      'services.wizard.bar.progress {"current":3,"total":6}',
    );
    expect($('[data-testid="wizard-progress"]')?.textContent).toContain(
      "services.wizard.steps.parts",
    );
    const buttons = Array.from(
      container.querySelectorAll('[data-testid="wizard-stepper"] [data-step]'),
    ).map((b) => b.getAttribute("data-step"));
    expect(buttons).toEqual([...STEPS]);
    expect(step("parts").getAttribute("aria-current")).toBe("step");
    expect(step("photos").getAttribute("aria-current")).toBeNull();
  });

  it("portals the step's actions into the bar and keeps their handlers", () => {
    const onBack = vi.fn();
    const onNext = vi.fn();
    render({ onBack, onNext });
    const actions = $('[data-testid="wizard-actions"]');
    expect(actions?.querySelector('[data-testid="back"]')).not.toBeNull();
    expect(actions?.querySelector('[data-testid="next"]')).not.toBeNull();
    // Not duplicated in the step body.
    expect(
      $('[data-testid="step-body"]')?.querySelector('[data-testid="next"]'),
    ).toBeNull();
    click($('[data-testid="back"]'));
    click($('[data-testid="next"]'));
    expect(onBack).toHaveBeenCalledTimes(1);
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  it("keeps a disabled action disabled", () => {
    const onNext = vi.fn();
    render({ onNext, nextDisabled: true });
    const next = $('[data-testid="next"]') as HTMLButtonElement;
    expect(next.disabled).toBe(true);
    click(next);
    expect(onNext).not.toHaveBeenCalled();
  });

  it("only opens step 1 before the draft exists", () => {
    const onSelect = vi.fn();
    render({ current: "customer_vehicle", hasService: false, onSelect });
    expect(step("customer_vehicle").disabled).toBe(false);
    for (const s of STEPS.slice(1)) expect(step(s).disabled).toBe(true);
    click(step("parts"));
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("jumps to an open step and follows the contract and photo locks", () => {
    const onSelect = vi.fn();
    render({ contractLocked: true, onSelect });
    expect(step("contract").disabled).toBe(false);
    expect(step("stock").disabled).toBe(true);
    click(step("measurement"));
    expect(onSelect).toHaveBeenCalledWith("measurement");

    render({ current: "photos", photosLocked: true, onSelect });
    expect(step("parts").disabled).toBe(true);
    expect(step("parts").getAttribute("data-blocked")).toBe("photos");
    expect(step("customer_vehicle").disabled).toBe(false);
  });

  it("marks the steps before the current one done", () => {
    render({ current: "measurement" });
    expect(step("parts").querySelector("svg")).not.toBeNull();
    expect(step("measurement").textContent).toBe("4");
    expect(step("stock").textContent).toBe("6");
  });
});

describe("WizardActions without a bar", () => {
  it("renders the actions inline", () => {
    act(() =>
      root.render(
        createElement(
          WizardActions,
          null,
          createElement("button", { "data-testid": "only" }, "x"),
        ),
      ),
    );
    expect($('[data-testid="only"]')).not.toBeNull();
  });
});
