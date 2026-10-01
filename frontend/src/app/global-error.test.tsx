// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

const captureException = vi.fn();
const captureRequestError = vi.fn();
const init = vi.fn();

vi.mock("@sentry/nextjs", () => ({
  captureException,
  captureRequestError,
  init,
}));

describe("error tracking wiring", () => {
  it("global-error reports the error with its digest", async () => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    const { default: GlobalError } = await import("./global-error");
    const error = Object.assign(new Error("boom"), { digest: "d1" });
    const container = document.createElement("div");
    const root = createRoot(container);
    // global-error renders <html>; jsdom tolerates it inside a div.
    vi.spyOn(console, "error").mockImplementation(() => {});
    await act(async () => {
      root.render(createElement(GlobalError, { error, reset: () => {} }));
    });
    expect(captureException).toHaveBeenCalledWith(error, {
      tags: { digest: "d1" },
    });
    act(() => root.unmount());
  });

  it("instrumentation exports onRequestError and inits disabled without DSN", async () => {
    vi.stubEnv("NEXT_RUNTIME", "nodejs");
    vi.stubEnv("SENTRY_DSN", "");
    vi.stubEnv("NEXT_PUBLIC_SENTRY_DSN", "");
    const mod = await import("../instrumentation");
    expect(mod.onRequestError).toBe(captureRequestError);
    await mod.register();
    expect(init).toHaveBeenCalledTimes(1);
    expect(init.mock.calls[0][0]).toMatchObject({
      enabled: false,
      sendDefaultPii: false,
    });
    vi.unstubAllEnvs();
  });
});
