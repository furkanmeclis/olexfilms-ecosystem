import { describe, expect, it } from "vitest";

import { apiPath } from "./api-path";

describe("apiPath (TEC-755)", () => {
  it("returns a static template as is", () => {
    expect(apiPath("put", "/v1/stock-forecasts/thresholds")).toBe(
      "/v1/stock-forecasts/thresholds",
    );
  });

  it("fills and encodes path parameters", () => {
    expect(
      apiPath("get", "/v1/stock-forecasts/{product_uuid}", {
        product_uuid: "a/b",
      }),
    ).toBe("/v1/stock-forecasts/a%2Fb");
  });

  it("rejects paths and methods the OpenAPI document does not declare", () => {
    // The checks below run in `pnpm typecheck`; at runtime they only build strings.
    // @ts-expect-error the backend never served /widget
    apiPath("get", "/v1/stock-forecasts/widget");
    // @ts-expect-error the threshold endpoint is collection-level
    apiPath("put", "/v1/stock-forecasts/{product_uuid}/threshold", {
      product_uuid: "p",
    });
    // @ts-expect-error thresholds is PUT only
    apiPath("post", "/v1/stock-forecasts/thresholds");
    const missing = () =>
      // @ts-expect-error {product_uuid} needs a value
      apiPath("get", "/v1/stock-forecasts/{product_uuid}");
    expect(missing).toThrow();
  });
});
