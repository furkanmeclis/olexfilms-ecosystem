# Add-ons user guide

This guide covers the add-on modules of the Olexfilms panel: what each add-on does, who switches it on, how to request it, what to do on each screen and which permissions are needed. The guide is available to the whole network in the document center (**Documents → Document library**, folder "Kılavuzlar"). It exists in Turkish and English; other languages open the English version.

> **Screenshots:** boxes marked `[Screenshot: …]` are screenshot placeholders. Images will be added in a later version.

## 1. Module levels

Every feature of the panel is a **module**. Modules have three levels:

| Level | Meaning | Price |
| --- | --- | --- |
| Core | Always on, cannot be switched off (e.g. stock, orders, services, customers, accounting). | Free |
| Standard | On by default (e.g. contracts, measurements, leads, appointments, announcements, dealer accounting). | Free; no service record needed, does not recur |
| Add-on | Off by default. Requested, or switched on by a level above. | Mostly paid; the price is shown on the **Features** page |

The add-ons in this guide (module key in brackets):

| Add-on | Module key | Used by |
| --- | --- | --- |
| Dealer showcase and lead form | `dealer_showcase` | Dealer, distributor; review queue at the center |
| Fleet customers | `fleet` | Dealer, distributor, center; fleet manager in the portal |
| Certificates | `certificates` | Dealer, distributor, center |
| Stock forecast and order suggestions | `stock_forecast` | Dealer, distributor, center |
| Performance and targets (bonus included) | `performance` | Center, distributor, dealer |
| Efficiency and waste analysis | `efficiency` | Center, distributor, dealer |
| Photo standard | `photo_standard` | Center, distributor; applied at dealer intake |
| E-invoice (UBL-TR) | `e_invoice` | Center only, Türkiye only |
| Recommended retail price | not a module, permission: `pricing.recommended.read` | Center publishes; distributors and dealers see it |

## 2. Who switches an add-on on: chain rules

Whether an add-on is on for an organization is decided by a chain from top to bottom: **platform admin (center) → distributor → dealer**. Values are not copied; the chain is resolved on every read and a change reaches everyone within 30 seconds.

1. **Core modules** are always on.
2. A module **closed system wide** cannot be enabled anywhere. The platform admin can close a module system wide on the **Modules** page (`/platform/modules`) and change its default and paid flags.
3. **A distributor, the center and a dealer without a distributor** use their own value; without one, the module default applies.
4. For **a dealer under a distributor**, in priority order:
   1. A value the platform admin set for the dealer (applies even when the distributor has the module off).
   2. If the distributor has it off, it is off for the dealer and the module is **hidden** from the dealer ("Off at the level above").
   3. The dealer's own value (set by the distributor for that dealer).
   4. The distributor's **dealer standard**.
   5. The module default.

When a level above closes a module, nothing below is deleted; the dealer simply sees it off. When the level above opens it again, the dealer's earlier value applies again.

### 2.1 A distributor switching modules for its dealers

On **Features → Dealers** a distributor switches a module on or off directly for its dealers. Rules:

- A distributor cannot switch on for dealers a module that is off for itself.
- Only its own direct dealers can be selected; the whole selection is applied or none of it.
- A dealer whose value the platform admin set cannot be changed by the distributor (`MODULE_ADMIN_OVERRIDE`).
- "Clear" returns the dealer to the dealer standard.

### 2.2 Dealer standard

The **Features → Dealer standard** tab is the distributor's default module set for its dealers. Dealers without their own value, and newly created dealers, follow the standard immediately. A distributor cannot add a module that is off for itself.

### 2.3 Platform admin switching independently

The platform admin can switch a module on or off **independently** for any organization (source: "Platform admin"). This value applies to a dealer even when its distributor has the module off; it is refused only for a module closed system wide. "Clear" puts the organization back into the chain.

### 2.4 Module package

The center's service catalog has services of type **module package** (`module_bundle`). When a module package subscription is assigned to an organization, the modules of the package are switched on (source: "Service record", badge: "On by subscription"). A module closed at a level above cannot be opened by a package either; the assignment is then refused as a whole. When the subscription is cancelled or expires, the module is closed unless another active subscription covers it. Expiry is checked by an hourly job.

The price of an add-on comes from the module package on sale that contains it. If there are several, the one with the fewest modules, then the cheapest, is shown. If a distributor has a price override, the distributor and its dealers see that price. Without a package on sale, "Contact us for the price" is shown.

### 2.5 Source badges on the Features page

| Badge | Meaning |
| --- | --- |
| Core (always on) | Core module |
| Closed system wide | The platform admin closed it system wide |
| Default | The module's default value |
| Off at the level above | Off for the dealer because it is off for the distributor |
| Dealer standard | From the distributor's dealer standard |
| Platform admin | The admin's independent value |
| Distributor | The value the distributor set for this dealer |
| Service record | From a module package subscription |

## 3. Requesting an add-on

> **[Screenshot: features-page]** Features page: level groups, description, price label, status and source badge, "Request" button.

1. Open **Business → Features** in the side menu (`/t/{organization}/features`). Everyone with the `modules.read` permission sees the page.
2. Press **Request** on the row of an add-on that is off.
3. Optionally write a note (up to 1000 characters) and send.
4. The row shows a **Request pending** badge. Until it is decided you can cancel with **Withdraw request**.

Where the request goes:

- A request of a dealer under a distributor goes to the **distributor's** **Features → Requests** tab; the distributor owners are notified.
- A request of a distributor, or of a dealer without a distributor, goes to the **center**: the **Requests** tab of the **Modules** page; the platform admins are notified.

Rules:

- Core modules, modules already on and modules closed system wide cannot be requested.
- An organization has one open request per module; requesting again updates it.
- Statuses: **Pending**, **Approved**, **Rejected**, **Withdrawn**.
- If the module is switched on another way (admin, distributor, dealer standard, module package), the open request becomes **Approved** automatically ("Automatic").
- On approval or rejection the requesting user receives an in-app notification and an e-mail.

### 3.1 Requests queue (for approvers)

> **[Screenshot: module-requests]** Requests tab: requester, module, status, dates and decision columns, bulk approve.

- Distributor: **Features → Requests** (`modules.manage` permission). An approved module is switched on for that dealer. Approving a module that is off for the distributor itself is refused; the distributor must first request it from the level above.
- Center: **Modules → Requests** (`platform.modules.read` to view, `platform.modules.write` to decide). An approved module is switched on for that organization as a platform admin value.
- **Approve** or **Reject** from the row; select several requests and use **Approve selected** for bulk approval. An optional note can be written with the decision; it is shown in the **Decision** column.

## 4. Permissions overview

| Permission | Grants | Held by |
| --- | --- | --- |
| `modules.read` | View the Features page, request modules | Distributor and dealer roles |
| `modules.manage` | Dealers and dealer standard tabs, approving requests | Distributor owner |
| `platform.modules.read` / `platform.modules.write` | Modules page, center requests queue | Platform admin |

Each add-on's own permissions are listed in its section below. To see a screen, the add-on must be on and your role must hold the permission; when the add-on is off the menu item is hidden and the API answers `FEATURE_DISABLED`.

## 5. Dealer showcase and lead form (`dealer_showcase`)

**What it does:** Enriches the dealer's public page (`/bayi/{code}`): services, photo gallery, working hours, Google rating, quote form and WhatsApp button. A request from the quote form becomes a lead of the dealer.

**Who switches it on:** Add-on (paid). The distributor switches it on for its dealers or the dealer requests it; the center can switch it on independently. While the module is off, the public page keeps only the basics (name, address, map, WhatsApp).

**Screens:**

> **[Screenshot: showcase-editor]** My showcase: language content, working hours, services, gallery, Google rating.

- **My showcase** (`/t/{organization}/showcase`, dealer and distributor):
  - Language content: headline, about text, SEO keywords.
  - Working hours; "Copy from appointment hours" fills them quickly.
  - SEO and social links.
  - Services: add, hide, reorder.
  - Photo gallery: upload, caption, reorder (by default at most 12 photos, each up to 5 MB).
  - Google rating: manual entry (1.0–5.0) or a Places ID.
  - **Preview**, **Save draft**, **Submit for review** (**Publish** when no review is needed).
  - Statuses: Draft, Pending review, Published, Rejected (the rejection note is shown).
  - A distributor edits its dealers' showcases with the "Target dealer" picker.
- **Showcase review queue** (`/platform/showcases`, center): Inspect, **Approve**, **Reject** (a note is required).
- **Public dealer page** (`/bayi/{code}`): "Message on WhatsApp", Services, Gallery, Google rating and the **Request a quote** form (name, phone, e-mail, vehicle, services of interest, preferred contact, message, KVKK consent).

> **[Screenshot: showcase-public]** Public dealer page and quote form.

**Permissions:** `showcase.read` (center staff, distributor owner, dealer owner and staff), `showcase.write` (distributor owner, dealer owner), `platform.showcase.review` (center staff, center social media).

**FAQ:**

- *Is the showcase published immediately?* Center review is off by default; submitting publishes it. If the admin turns review on, the showcase becomes "Pending review" and the decision is sent to the dealer owners. On rejection the previously published version stays live.
- *Are prices shown on the showcase?* No. No price at all (recommended price included) is shown on the showcase.
- *How is the Google rating updated?* If a Google Places key is configured and a Places ID is entered, it is updated daily; otherwise the dealer enters it by hand.
- *Where do WhatsApp messages go?* To the center's number, tagged with the dealer code. Only the system admin sees the conversations; the lead opens at the dealer and the dealer is notified.

## 6. Fleet customers (`fleet`)

**What it does:** Manages corporate fleets (by tax number) as a separate account: vehicles, fleet users, bulk service plan, bulk vehicle intake, fleet account statement and a periodic PDF report. The fleet manager uses their own portal.

**Who switches it on:** Add-on (paid). Dealers, distributors and the center can use it; the chain rules apply. The fleet itself is a separate organization type, outside the dealer tree and the module chain.

**Screens:**

> **[Screenshot: fleet-card]** Fleet card: summary, vehicles, users, services, account statement, reports, service plans.

- **Fleets → Fleet list** (`/t/{organization}/fleets`): **New fleet** first asks for the tax number and checks whether the fleet already exists.
- **Fleet card** (`/t/{organization}/fleets/{fleet}`): tabs Summary, Vehicles (add, import, remove from fleet), Users (invite, disable), Services, Account statement (opening, services, collections, closing; exportable), Reports, Service plans.
- **Bulk service plan** wizard (`…/plans/new`): Vehicles → Service and schedule (start date, max vehicles per day, preferred hours) → Preview. The plan detail offers cancel and start intake.
- **Fleet portal** (for fleet users): Fleet vehicles, Services, Warranties, Account, Reports and approval of link requests from dealers.

**Permissions:** `fleets.read`, `fleets.manage` (center staff, distributor owner, dealer owner; dealer staff read only), `fleets.plan` (dealer owner and staff), `fleet.portal.read` (fleet role, own fleet only).

**FAQ:**

- *How is the same fleet linked to a second dealer?* When the second dealer enters the same tax number a link request is sent; the link is made once a fleet user accepts it in the portal. The requester is notified either way.
- *How does a fleet user sign in?* They set a password with the code in the invitation e-mail. The first user is the primary user and owns the fleet's vehicles.
- *How much accounting does the fleet portal show?* For each dealer only the fleet's account: services billed to the fleet, collections and balance.
- *When does the periodic report arrive?* Monthly by default; quarterly or off can be chosen. The PDF is produced in the fleet's language and e-mailed to the fleet users.

## 7. Certificates (`certificates`)

**What it does:** Keeps staff certificates (e.g. application training), tracks their validity and flags services done by uncertified staff on products or categories that require a certificate.

**Who switches it on:** Add-on (paid). The center defines certificate types; the dealer/distributor owner uploads certificates for their own staff; only the center, distributors and super_admin verify them.

**Screens:**

> **[Screenshot: certificates]** Staff certificates list and verification queue.

- **Certificates → Staff certificates** (`/t/{organization}/certificates`): **Upload certificate** (PDF only, up to 10 MB), status, PDF download, revoke, coverage view.
- **Verification queue** (`…/certificates/verification`, center and distributor): verify, or reject with a reason.
- **Certificate approvals** (`…/certificates/approvals`, center): approve or reject service status changes waiting because of a missing certificate.
- **Certificate types** (`/platform/certificate-types`, center): validity period and product/category bindings.

**Permissions:** `certificate_types.manage` and `certificates.approve_service` (center staff), `certificates.read` (center staff, distributor owner, dealer owner), `certificates.write` (distributor owner, dealer owner), `certificates.verify` (center staff, distributor owner).

**FAQ:**

- *What happens if uncertified staff perform a service?* The service gets an internal warning flag. No notification is ever sent to the customer. If the admin turns on "approval on status change", status changes other than cancel wait for center approval (off by default).
- *Is there a warning before a certificate expires?* A notification is sent 30 days before expiry (admin setting). On expiry a daily job marks the certificate expired.

## 8. Stock forecast and order suggestions (`stock_forecast`)

**What it does:** Calculates per product the daily consumption speed, the stock-out date and an order suggestion. The calculation is algorithmic (no AI).

**Who switches it on:** Add-on (paid). Center, distributors and dealers can use it.

**Screens:**

> **[Screenshot: stock-forecast]** Forecast list, product chart and order draft.

- **Stock → Forecast and suggestions** (`/t/{organization}/stock-forecast`): product list and search, product chart (actual / projected / threshold), **Create order draft** from the selection (quantities editable), threshold editing.
- **Network demand** tab (center, exportable) and **My dealers** tab (distributor).
- **Critical stock** card on the dashboard.

**Permissions:** `stock_forecast.read` (center staff and warehouse, distributor owner/staff/warehouse, dealer owner and staff), `stock_forecast.manage` (center staff and warehouse, distributor owner, dealer owner), `stock_forecast.network.read` (center).

**FAQ:**

- *Why do I see "insufficient data"?* The forecast needs at least 90 days of movements.
- *What are the thresholds?* Warning at 14 days of stock left, critical at 7 days. The suggestion targets 30 days of cover, minus stock on hand and open orders.
- *When is it calculated?* Every night at 03:00 (organization time zone). A notification is sent only when a product moves "OK → warning" or "warning → critical".

## 9. Performance, targets and bonus (`performance`)

**What it does:** Monthly performance metrics, ranking and comparison, region map, targets and achievement, weak dealer rules; within a dealer, staff targets and target-based bonuses.

**Who switches it on:** Add-on (paid). Center, distributors and dealers can use it. The bonus screens also require the **Dealer accounting** (`dealer_accounting`) module.

**Screens** (`/t/{organization}/performance`, menu **Performance**):

> **[Screenshot: performance]** Performance panel, ranking and region map.

- Center and distributor: **Panel**, **Ranking** (row actions "Set target", "Open task"; exportable), **Region map** (country/province/district, dealer points, regions without dealers), **Dashboard**, **Targets** (monthly, quarterly, yearly), **Weak dealer rules**.
- Dealer: **Panel** ("My position in the network"; no other dealer names), **Ranking**, **Dashboard**, **Team targets** (staff × month grid), **Bonus rules** (fixed amount or share of service revenue, achievement threshold; **Bonus payment day**), **Bonuses** (approve, change the amount — a note is required, cancel, bulk approve).

**Permissions:** `performance.read` (center staff, distributor owner and staff, dealer owner), `performance.targets.manage` and `performance.rules.manage` (center staff, distributor owner), `performance.staff_targets.manage` (dealer owner), `performance.bonus.manage` (dealer owner, dealer accounting).

**FAQ:**

- *When is the bonus paid?* It is calculated at month end, approved by the dealer owner and recorded as a planned staff payment on the bonus payment day of the following month (default the 5th, adjustable from 1 to 28).
- *What does a weak dealer rule do?* For a dealer matching the condition (target achievement, or below the network median), the center and distributor owners are notified. An automatic task is opened only for center rules (one task per dealer × rule × month). The dealer receives a neutral "below target" notification.

## 10. Efficiency and waste analysis (`efficiency`)

**What it does:** Compares the film meters used in completed services with the expected consumption and shows the waste ratio broken down by dealer, staff, product, body type, part and roll.

**Who switches it on:** Add-on (paid). Center, distributors and dealers can use it. The center manages the expected consumption table.

**Screens:**

> **[Screenshot: efficiency]** Efficiency summary, monthly trend and breakdown tabs.

- **Efficiency and waste** (`/t/{organization}/efficiency`): period selector; average waste, warning threshold, total meters, top-waste product; Comparison and Monthly trend; tabs Dealers, Staff, Products, Body types, Parts, Rolls (roll detail and the services that used it). Exportable.
- **Expected consumption** (`/platform/part-consumption-expectations`, center): expected meters per product/category × body type × part; add, edit, import.

**Permissions:** `efficiency.read` (center staff, distributor owner, dealer owner), `efficiency.expectations.manage` (center staff).

**FAQ:**

- *When is a waste warning raised?* At 15% above expected.
- *What if no expected consumption is defined?* The network median of the last 180 days is used (at least 20 samples); with fewer samples, waste is not calculated.
- *Who is a dealer compared with?* Its own distributor's network.

## 11. Photo standard (`photo_standard`)

**What it does:** Defines the photo angles to take at vehicle intake; while a required angle is missing, no contract can be created and the service cannot leave draft. The photos are embedded in the contract PDF.

**Who switches it on:** An add-on, but free: a switch the platform admin turns on or off. The center defines the angles, a distributor can adjust them for its network, and the dealer applies them at intake.

**Screens:**

> **[Screenshot: intake-photos]** Intake photos step in the service wizard (camera).

- **Photo angles** (`/platform/photo-angles`, center): the angle set and example images.
- **Photo standard** (`/t/{organization}/photo-standard`, distributor): Required, Hidden or Center default for each angle.
- **Intake photos** step (service wizard and service detail): **Take photo** opens the camera on a phone; up to 12 MB per photo.

**Permissions:** `photo_standard.manage` (center staff), `photo_standard.override` (distributor owner).

**FAQ:**

- *What if a required angle is missing?* No contract can be created; the service cannot leave draft or be completed. Cancelling is always possible.
- *Who sees the location data?* EXIF location and device data are visible only to the dealer owner, the center and super_admin (KVKK). The images in the contract PDF carry no EXIF data.

## 12. E-invoice (UBL-TR) (`e_invoice`)

**What it does:** For the center's sales, creates UBL-TR e-Fatura and e-Arşiv invoice drafts, validates them (XSD + Schematron), numbers them and archives them as XML and PDF. Nothing is sent to an integrator.

**Who switches it on:** Center only and Türkiye only (TRY). No invoice is issued to a buyer outside Türkiye.

**Screens** (menu **Accounting → e-Invoice (e-Fatura)**):

> **[Screenshot: einvoice]** Invoice list, billable records and invoice detail.

- **Invoice list** (`/t/{organization}/einvoices`): statuses Draft, Validation error, Archived, Voided; CSV download.
- **Billable records** (`…/einvoices/billable`): received distributor orders and dealer service catalog periods; **Create draft**. Missing fields of the buyer's invoice profile are listed.
- **Invoice detail**: Preview, Archive, Void (a reason is required), Download XML, Download PDF, Regenerate PDF; validation messages and status history.
- **E-invoice settings** (`…/einvoices/settings`): seller profile, e-Arşiv and e-Fatura series (3 characters; number = series + year + 9 digits, e.g. EAR2026000000001), counters, XSLT (GİB default or custom).
- The buyer's invoice profile is on the organization detail page.

**Permissions:** `einvoice.read`, `einvoice.manage` (center accounting; archive and void require identity re-verification), `einvoice.settings` (super_admin only).

**FAQ:**

- *Is a number used up on a validation error?* No. An invoice that fails validation is not archived and gets no number.
- *What does void do?* It marks the invoice voided; the source record can be invoiced again. Return invoices are not part of this version.
- *Default VAT?* 20%. Drafting and archiving stay disabled until the settings are saved.

## 13. Recommended retail price and price discipline

**What it does:** The center publishes recommended retail prices per product × country × currency; distributors and dealers see the recommended price and the deviation next to their own prices. After each publication a price list PDF is added to the document center.

**Who switches it on:** Not a separate add-on; it runs in the catalog core with permissions. The price list PDF is published when the document center (`announcements` module) is on.

**Screens:**

> **[Screenshot: recommended-prices]** Recommended prices: price list, publication basket, version history.

- **Recommended prices** (`/t/{organization}/catalog/recommended-prices`, center): Price list and Version history tabs, bulk change by percentage, **Publication basket** → **Publish** with an effective date (requires identity re-verification), CSV export and import.
- **Price discipline** (`…/catalog/price-discipline`, center and distributor): deviation list and the "Average deviation by country" chart.
- For distributors and dealers: a "Recommended price" column and deviation badge on **My sale prices** and in the catalog price table.
- **Documents → "Fiyat listeleri"** folder: a price list PDF per country/currency.

**Permissions:** `pricing.recommended.read` (center staff and accounting, distributor owner and accounting, dealer owner and accounting), `pricing.recommended.write` (publishing; center accounting only), `pricing.discipline.read` (center staff, distributor owner).

**FAQ:**

- *What is the deviation threshold?* ±15%. Threshold breaches are not reported one by one but in a weekly summary (Monday) to the center, and to each distributor for its own network.
- *When does a future-dated price apply?* On its effective day; versions are never overwritten.
- *Is the recommended price shown on the showcase?* No, only in the distributor and dealer panels.

## 14. General FAQ

- *I cannot see an add-on.* It may be off for your distributor ("Off at the level above") or closed system wide. Request it on the Features page; if it is off for your distributor too, the distributor must first request it from the center.
- *The add-on is on but not in the menu.* Your role lacks the permission; ask your organization owner.
- *My module package subscription ended; is my data deleted?* No. The module closes, the data is kept and comes back when the module is switched on again.
- *Where is this guide updated?* A new edition is added as a new version of the same document center item; if the content did not change, no new version is opened.
