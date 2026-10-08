import { expect, test, type Route } from "@playwright/test";

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

test("dealer showcase quote form submits and shows success", async ({
  page,
}) => {
  let posted: Record<string, unknown> | null = null;
  await page.route(
    "**/api/v1/public/dealers/olex-kadikoy/leads",
    (route: Route) => {
      posted = route.request().postDataJSON() as Record<string, unknown>;
      return route.fulfill({ status: 202, json: envelope({ received: true }) });
    },
  );

  await page.goto("/bayi/olex-kadikoy?lang=en");

  await expect(
    page.getByRole("heading", { level: 1, name: "Olex Kadıköy" }),
  ).toBeVisible();
  await expect(page.locator('[data-slot="google-rating"]')).toContainText(
    "4.8",
  );
  await expect(page.locator('[data-slot="showcase-services"]')).toContainText(
    "Full body PPF",
  );

  const form = page.locator('[data-slot="showcase-lead-form"]');
  await expect(form).toBeVisible();
  const submit = form.getByRole("button", { name: "Send request" });
  await expect(submit).toBeDisabled();

  await form.locator('input[name="name"]').fill("Ayşe Yılmaz");
  await form.locator('input[name="phone"]').fill("0532 123 45 67");
  await form.getByRole("checkbox", { name: "KVKK" }).click();
  await expect(submit).toBeEnabled();
  await submit.click();

  await expect(
    page.locator('[data-screen="showcase-lead-success"]'),
  ).toContainText("Request received");
  expect(posted).toMatchObject({
    name: "Ayşe Yılmaz",
    phone: "+905321234567",
    kvkk_consent: true,
    language: "en",
    form_token: "e2e-showcase-token",
    website: "",
  });
});
