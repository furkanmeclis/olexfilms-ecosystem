// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  createMember: vi.fn(),
  createDealer: vi.fn(),
  listWarehouses: vi.fn(),
}));

vi.mock("@/providers/locale-provider", () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/providers/permission-provider", () => ({
  usePermission: () => ({ can: () => true }),
}));
vi.mock("@/components/entity", () => ({
  EntityPage: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@/hooks/use-active-organization", () => ({
  useActiveOrganization: () => ({ slug: "olex", type: "center" }),
}));
vi.mock("@/features/accounts/services/accounts.service", () => ({
  accountsService: {
    createMember: api.createMember,
    createDealer: api.createDealer,
  },
}));
vi.mock("@/features/warehouse/services/warehouse.service", () => ({
  warehouseKeys: { warehouses: ["warehouse", "warehouses"] },
  warehouseService: { listWarehouses: api.listWarehouses },
}));

import {
  fill,
  mount,
  render,
  submit,
  unmount,
  type Mounted,
} from "@/features/warehouse/components/test-helpers";

import { DealerCreatePage } from "./dealer-create-page";
import { TeamMemberCreatePage } from "./team-member-create-page";

let m: Mounted;
beforeEach(() => {
  m = mount();
  api.listWarehouses.mockResolvedValue({
    items: [
      {
        uuid: "w-1",
        name: "AFM Florya Merkez",
        active: true,
        is_primary: true,
      },
      { uuid: "w-2", name: "Yedek", active: true, is_primary: false },
    ],
  });
});
afterEach(() => {
  unmount(m);
  vi.clearAllMocks();
});

const q = (sel: string) => m.container.querySelector(sel);

describe("team member form", () => {
  it("sends the phone, the warehouse role and the primary warehouse", async () => {
    api.createMember.mockResolvedValue({
      user_uuid: "u-1",
      name: "Depo",
      surname: "-",
      phone: "+905551112233",
      role_slugs: ["center_warehouse"],
      warehouse_uuids: ["w-1"],
      existing: false,
    });
    await render(m, <TeamMemberCreatePage slug="olex" />);
    await fill(q("#member-name"), "Depo");
    await fill(q("#member-phone"), "+905551112233");
    await submit(q("form"));
    expect(api.createMember).toHaveBeenCalledWith({
      name: "Depo",
      phone: "+905551112233",
      role_slugs: ["center_warehouse"],
      warehouse_uuids: ["w-1"],
    });
    expect(q('[data-testid="account-created"]')?.textContent).toContain(
      "+905551112233",
    );
  });

  it("needs a name and a phone", async () => {
    await render(m, <TeamMemberCreatePage slug="olex" />);
    await submit(q("form"));
    expect(api.createMember).not.toHaveBeenCalled();
    expect(m.container.textContent).toContain(
      "organizations.accounts.errors.phone_required",
    );
  });
});

describe("dealer form", () => {
  it("opens the dealer with its owner", async () => {
    api.createDealer.mockResolvedValue({
      organization: { name: "Demo Bayi" },
      owner: { name: "Bayi", phone: "+905552223344", existing: false },
    });
    await render(m, <DealerCreatePage slug="olex" />);
    await fill(q("#dealer-name"), "Demo Bayi");
    await fill(q("#dealer-city"), "İstanbul");
    await fill(q("#owner-name"), "Bayi");
    await fill(q("#owner-phone"), "05552223344");
    await submit(q("form"));
    expect(api.createDealer).toHaveBeenCalledWith({
      name: "Demo Bayi",
      city: "İstanbul",
      owner: { name: "Bayi", phone: "05552223344" },
    });
    expect(q('[data-testid="account-created"]')?.textContent).toContain(
      "Demo Bayi",
    );
  });
});
