import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Markdown, parseMarkdown } from "./markdown";
import { signInErrorCode } from "./portal-client";

describe("legal text markdown", () => {
  it("parses headings, paragraphs and lists", () => {
    const blocks = parseMarkdown(
      "## Başlık\n\nBir satır\nikinci satır\n\n- a\n- b\n\n1. x\n2. y",
    );
    expect(blocks).toEqual([
      { type: "heading", level: 2, text: "Başlık" },
      { type: "paragraph", text: "Bir satır ikinci satır" },
      { type: "list", ordered: false, items: ["a", "b"] },
      { type: "list", ordered: true, items: ["x", "y"] },
    ]);
  });

  it("never renders raw HTML or unsafe links", () => {
    const html = renderToStaticMarkup(
      <Markdown
        source={
          "<script>alert(1)</script> **kalın** [x](javascript:alert(1)) [ok](https://olexfilms.app)"
        }
      />,
    );
    expect(html).not.toContain("<script>");
    expect(html).toContain("&lt;script&gt;");
    expect(html).toContain("<strong>kalın</strong>");
    expect(html).not.toContain("javascript:");
    expect(html).toContain('href="https://olexfilms.app"');
  });
});

describe("portal sign-in result", () => {
  it("maps Auth.js callback answers to error codes", () => {
    expect(signInErrorCode(200, "http://localhost/portal")).toBeNull();
    expect(
      signInErrorCode(
        200,
        "http://localhost/portal/login?error=CredentialsSignin&code=INVALID_OTP_CODE",
      ),
    ).toBe("INVALID_OTP_CODE");
    expect(
      signInErrorCode(
        200,
        "http://localhost/portal/login?error=CredentialsSignin",
      ),
    ).toBe("CredentialsSignin");
    expect(signInErrorCode(502, null)).toBe("unavailable");
  });
});
