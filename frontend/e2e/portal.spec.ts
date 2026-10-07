import { expect, test, type Page } from "@playwright/test";

import { E2E_PORTAL, E2E_UPSTREAM_PORT } from "./support/constants";
import {
  PORTAL_DEALERS,
  PORTAL_SERVICES,
  PORTAL_VEHICLE,
  PortalMock,
  routePortal,
} from "./support/portal-mock";

/**
 * TEC-246 (TEC-108 acceptance): the customer portal end to end against a
 * mocked BFF. The owner signs in with a WhatsApp code (fake sender, no
 * wuzapi; the Next.js server verifies it against the upstream mock), sees
 * one vehicle whose services from two dealers are in one list, opens a
 * service, transfers the vehicle with the two codes, and the buyer signs in
 * with their own code and finds the vehicle (with its history) in their
 * list.
 */

type User = (typeof E2E_PORTAL)[keyof typeof E2E_PORTAL];

async function signInWithOTP(page: Page, api: PortalMock, user: User) {
  await page.goto("/portal/login");
  await page.locator("#portal-phone").fill(user.typed);
  await page.getByRole("button", { name: "Send code via WhatsApp" }).click();
  await expect(page.locator("#portal-otp")).toBeVisible();
  const code = api.lastCode(user.phone, "customer_login");
  expect(code, `WhatsApp login code to ${user.phone}`).toBeTruthy();
  await page.locator("#portal-otp").fill(code ?? "");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/portal$/);
  await expect(page.getByTestId("portal-nav")).toBeVisible();
}

test("portal: two dealers' services in one list, then vehicle transfer", async ({
  browser,
  baseURL,
  page,
  context,
  request,
}) => {
  const api = new PortalMock();
  const { owner, buyer } = E2E_PORTAL;

  // A wrong code does not open a session.
  await routePortal(context, api, owner);
  await page.goto("/portal/login");
  await page.locator("#portal-phone").fill(owner.typed);
  await page.getByRole("button", { name: "Send code via WhatsApp" }).click();
  await page.locator("#portal-otp").fill("000000");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page).toHaveURL(/\/portal\/login/);

  await signInWithOTP(page, api, owner);
  const upstream = (await (
    await request.get(`http://127.0.0.1:${E2E_UPSTREAM_PORT}/__calls`)
  ).json()) as string[];
  expect(upstream).toContain("POST /v1/auth/otp/verify");

  // My vehicles: one vehicle, two services.
  await page.getByTestId("portal-vehicles-link").click();
  const cards = page.getByTestId("portal-vehicle");
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toContainText("BMW M3");
  await expect(cards.first()).toContainText("Services: 2");
  await cards.first().click();

  // Vehicle detail: both dealers' services in one history list.
  await expect(page).toHaveURL(
    new RegExp(`/portal/vehicles/${PORTAL_VEHICLE}$`),
  );
  const history = page.getByTestId("portal-vehicle-services").locator("li");
  await expect(history).toHaveCount(2);
  await expect(history.nth(0)).toContainText(PORTAL_DEALERS.cankaya.name);
  await expect(history.nth(1)).toContainText(PORTAL_DEALERS.kadikoy.name);
  await expect(page.getByTestId("portal-active-warranty")).toHaveCount(2);

  // Service detail of the second dealer.
  await history.nth(0).getByRole("link").click();
  await expect(page).toHaveURL(
    new RegExp(`/portal/services/${PORTAL_SERVICES.cankaya}$`),
  );
  await expect(page.getByTestId("portal-dealer")).toContainText(
    PORTAL_DEALERS.cankaya.name,
  );
  await expect(page.getByTestId("portal-service-products")).toContainText(
    "Olex Ceramic Pro",
  );
  await expect(page.getByTestId("portal-service-warranty")).toHaveCount(1);
  await page.goto(`/portal/vehicles/${PORTAL_VEHICLE}`);

  // Transfer: the buyer's phone, then the two WhatsApp codes.
  await page.getByTestId("portal-transfer-open").click();
  await page.locator("#portal-transfer-phone").fill(buyer.phone);
  await page.getByRole("button", { name: "Send codes" }).click();
  await expect(page.getByTestId("portal-transfer-code-step")).toBeVisible();
  const fromCode = api.lastCode(owner.phone, "vehicle_transfer") ?? "";
  const toCode = api.lastCode(buyer.phone, "vehicle_transfer") ?? "";
  expect(fromCode).toMatch(/^\d{6}$/);
  expect(toCode).toMatch(/^\d{6}$/);
  expect(toCode).not.toBe(fromCode);

  // A wrong buyer code: the owner's side is verified, attempts drop.
  await page.locator("#portal-transfer-from-code").fill(fromCode);
  await page.locator("#portal-transfer-to-code").fill("000000");
  await page.getByTestId("portal-transfer-verify").click();
  await expect(page.getByTestId("portal-transfer-error")).toContainText(
    "Attempts left: 4",
  );
  await expect(
    page.getByTestId("portal-transfer-from-code-verified"),
  ).toBeVisible();
  await page.locator("#portal-transfer-to-code").fill(toCode);
  await page.getByTestId("portal-transfer-verify").click();
  await expect(page.getByTestId("portal-transfer-done")).toContainText(
    "Warranties moved: 2",
  );

  // Closing the dialog goes back to the (now empty) list.
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(/\/portal\/vehicles$/);
  await expect(page.getByTestId("portal-vehicles-empty")).toBeVisible();

  // The buyer signs in with their own code and owns the vehicle.
  const buyerContext = await browser.newContext({
    baseURL,
    locale: "en-US",
    timezoneId: "Europe/Istanbul",
  });
  try {
    await routePortal(buyerContext, api, buyer);
    const buyerPage = await buyerContext.newPage();
    await signInWithOTP(buyerPage, api, buyer);
    await buyerPage.goto("/portal/vehicles");
    const buyerCards = buyerPage.getByTestId("portal-vehicle");
    await expect(buyerCards).toHaveCount(1);
    await expect(buyerCards.first()).toHaveAttribute(
      "data-uuid",
      PORTAL_VEHICLE,
    );
    await buyerCards.first().click();
    await expect(
      buyerPage.getByTestId("portal-vehicle-services").locator("li"),
    ).toHaveCount(2);
    await expect(buyerPage.getByTestId("portal-active-warranty")).toHaveCount(
      2,
    );
  } finally {
    await buyerContext.close();
  }

  expect(api.unknown).toEqual([]);
});

/**
 * TEC-327: the owner books an appointment (dealer from the nearby list,
 * the only vehicle, a free slot; the full first day offers none), lands on
 * "my appointments" and cancels it.
 */
test("portal: book an appointment and cancel it", async ({ page, context }) => {
  const api = new PortalMock();
  const { owner } = E2E_PORTAL;
  await routePortal(context, api, owner);
  const dealer = PORTAL_DEALERS.kadikoy;
  await context.route("**/portal/dealers/nearby**", (route) =>
    route.fulfill({
      json: {
        success: true,
        data: {
          items: [
            {
              uuid: dealer.uuid,
              slug: "kadikoy",
              name: dealer.name,
              city: dealer.city,
              district: dealer.district,
              latitude: 40.99,
              longitude: 29.03,
              distance_km: 3.2,
              accepts_appointments: true,
              whatsapp: dealer.whatsapp,
            },
            {
              uuid: PORTAL_DEALERS.cankaya.uuid,
              slug: "cankaya",
              name: PORTAL_DEALERS.cankaya.name,
              city: "Ankara",
              district: "Çankaya",
              latitude: 39.9,
              longitude: 32.86,
              distance_km: 350,
              accepts_appointments: false,
              whatsapp: null,
            },
          ],
        },
      },
    }),
  );

  await signInWithOTP(page, api, owner);
  await page
    .getByTestId("portal-nav")
    .getByRole("link", { name: "My appointments" })
    .click();
  await expect(page.getByTestId("portal-appointments-empty")).toBeVisible();
  await page.getByTestId("portal-appointment-book").click();
  await expect(page).toHaveURL(/\/portal\/appointments\/new$/);

  // Only the dealer that takes portal bookings is offered.
  const dealers = page.getByTestId("booking-dealer");
  await expect(dealers).toHaveCount(1);
  await dealers.first().getByRole("button").click();
  await expect(page.getByTestId("booking-dealer-selected")).toContainText(
    dealer.name,
  );
  await expect(page.getByTestId("booking-vehicle")).toHaveAttribute(
    "aria-checked",
    "true",
  );

  // The first day is full: no slot; the next day has two.
  const days = page.getByTestId("booking-day");
  await expect(days.first()).toBeDisabled();
  await days.nth(1).click();
  await expect(page.getByTestId("booking-slot")).toHaveCount(2);
  await page.getByTestId("booking-slot").nth(1).click();
  await page.getByTestId("booking-submit").click();

  await expect(page).toHaveURL(/\/portal\/appointments$/);
  const card = page.getByTestId("portal-appointment");
  await expect(card).toHaveCount(1);
  await expect(card).toContainText(dealer.name);
  await expect(card).toContainText("Scheduled");

  await card.getByTestId("portal-appointment-cancel").click();
  await page.getByTestId("portal-appointment-cancel-confirm").click();
  await expect(card).toContainText("Cancelled");
  expect(api.appointments).toHaveLength(1);
  expect(api.unknown).toEqual([]);
});

/**
 * TEC-353: the WhatsApp review link (`/portal/services/{uuid}/review
 * ?source=whatsapp_link`) of a signed-out customer goes through the login
 * and lands on the service with the review form in view; the admin
 * questions (a required dealer rating, a required product rating per
 * product) gate the submit button, and the body carries the answers,
 * is_anonymous and source=whatsapp_link.
 */
test("portal: WhatsApp review link with admin questions, sent anonymously", async ({
  page,
  context,
}) => {
  const api = new PortalMock();
  const { owner } = E2E_PORTAL;
  const service = PORTAL_SERVICES.cankaya;
  const staff = "0b9c4c1e-0000-4000-8000-000000003531";
  const film = "0b9c4c1e-0000-4000-8000-000000003532";
  api.reviewQuestions = [
    {
      uuid: film,
      question_key: "film_quality",
      question_type: "rating_1_5",
      target: "product",
      is_required: true,
      sort_order: 20,
      text: "Film quality",
    },
    {
      uuid: staff,
      question_key: "staff",
      question_type: "rating_1_5",
      target: "dealer",
      is_required: true,
      sort_order: 10,
      text: "How was the staff?",
    },
  ];
  await routePortal(context, api, owner);

  await page.goto(`/portal/services/${service}/review?source=whatsapp_link`);
  await expect(page).toHaveURL(/\/portal\/login\?next=/);
  await page.locator("#portal-phone").fill(owner.typed);
  await page.getByRole("button", { name: "Send code via WhatsApp" }).click();
  await expect(page.locator("#portal-otp")).toBeVisible();
  await page
    .locator("#portal-otp")
    .fill(api.lastCode(owner.phone, "customer_login") ?? "");
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page).toHaveURL(
    new RegExp(`/portal/services/${service}\\?source=whatsapp_link$`),
  );
  const form = page.getByTestId("portal-review-form");
  await expect(form).toBeInViewport();
  await expect(form).toContainText("How was the staff?");
  await expect(form).toContainText("Film quality");
  const submit = page.getByTestId("portal-review-submit");

  await form.locator("[data-rating=platform] [data-star='4']").click();
  await form.locator("[data-rating=product] [data-star='5']").click();
  await expect(submit).toBeDisabled();
  await form.locator(`[data-rating='${staff}'] [data-star='3']`).click();
  const product = `${service.slice(0, -4)}7777`;
  await form
    .locator(`[data-rating='${film}:${product}'] [data-star='5']`)
    .click();
  await expect(submit).toBeEnabled();
  await page.getByTestId("portal-review-anonymous").click();
  await submit.click();

  await expect(page.getByTestId("portal-review-done")).toBeVisible();
  expect(api.reviews).toEqual([
    {
      platform_rating: 4,
      product_rating: 5,
      comment: null,
      is_anonymous: true,
      source: "whatsapp_link",
      answers: [
        { question_uuid: staff, rating: 3 },
        { question_uuid: film, product_uuid: product, rating: 5 },
      ],
    },
  ]);
});
