import { beforeEach, describe, expect, it } from "vitest";

import {
  opportunityStoreKey,
  readOpportunityDemoData,
  writeOpportunityDemoData,
} from "@/lib/opportunities/demo-store";
import { installMemoryLocalStorage } from "@/test-utils/memory-local-storage";

beforeEach(() => {
  installMemoryLocalStorage();
});

describe("opportunity demo store", () => {
  it("seeds job and SIWES examples once and persists subsequent updates", () => {
    const initial = readOpportunityDemoData();
    expect(initial.opportunities.map((item) => item.type)).toContain("job");
    expect(initial.opportunities.map((item) => item.type)).toContain("siwes");

    const updated = { ...initial, opportunities: initial.opportunities.slice(1) };
    writeOpportunityDemoData(updated);

    expect(readOpportunityDemoData()).toEqual(updated);
  });

  it("rejects malformed saved demo data instead of hiding the issue", () => {
    window.localStorage.setItem(opportunityStoreKey, JSON.stringify({ opportunities: [], applications: [{}] }));
    expect(() => readOpportunityDemoData()).toThrow(/demo data is invalid/i);
  });
});
