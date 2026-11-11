import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GlassSelect } from "@/components/workspace/glass-select";
import { DepartmentSelect } from "@/components/workspace/department-select";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  document.body.style.overflow = "";
});

const options = [
  { value: "", label: "All statuses" },
  { value: "draft", label: "Draft" },
  { value: "assigned", label: "Assigned" },
];

function DepartmentSelectHarness({ onSelection }: { onSelection: (value: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <GlassSelect
      label="Department"
      name="department_id"
      value={value}
      placeholder="Select a department"
      options={[
        { value: "engineering", label: "Engineering" },
        { value: "finance", label: "Finance & Accounts" },
      ]}
      onChange={(nextValue) => {
        setValue(nextValue);
        onSelection(nextValue);
      }}
    />
  );
}

function setViewport(width: number) {
  vi.stubGlobal("matchMedia", vi.fn().mockImplementation((query: string) => ({
    matches: query === "(max-width: 767px)" && width < 768,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })));
  Object.defineProperty(window, "innerWidth", { configurable: true, value: width });
  Object.defineProperty(window, "innerHeight", { configurable: true, value: 900 });
}

describe("GlassSelect", () => {
  it("renders a fixed desktop popover in a portal and filters via the selected value", async () => {
    setViewport(1366);
    const onChange = vi.fn();
    render(
      <GlassSelect
        label="Status"
        value=""
        placeholder="All statuses"
        options={options}
        onChange={onChange}
      />,
    );
    const trigger = screen.getByRole("button", { name: "Status" });
    vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue({
      x: 240,
      y: 120,
      top: 120,
      right: 480,
      bottom: 160,
      left: 240,
      width: 240,
      height: 40,
      toJSON: () => ({}),
    });

    fireEvent.click(trigger);
    const listbox = await screen.findByRole("listbox", { name: "Status" });
    expect(listbox.parentElement).toBe(document.body);
    expect(listbox).toHaveClass("workspace-modal-panel", "workspace-select-panel");
    expect(listbox).toHaveStyle({ position: "fixed", zIndex: "1300", left: "240px", width: "352px" });
    expect(screen.getByRole("option", { name: "Draft" })).toHaveClass("workspace-modal-link", "workspace-select-option");

    fireEvent.click(screen.getByRole("option", { name: "Draft" }));
    expect(onChange).toHaveBeenCalledWith("draft");
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("keeps its desktop popover inside an open dialog", async () => {
    setViewport(1366);
    render(
      <dialog open>
        <GlassSelect
          label="Task priority"
          value="normal"
          placeholder="Select priority"
          options={[
            { value: "low", label: "Low" },
            { value: "normal", label: "Normal" },
          ]}
          onChange={() => undefined}
        />
      </dialog>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Task priority" }));
    const listbox = await screen.findByRole("listbox", { name: "Task priority" });
    expect(listbox.parentElement?.tagName).toBe("DIALOG");
  });

  it("uses the profile-menu panel style on mobile and dismisses when clicking outside", async () => {
    setViewport(390);
    setViewport(390);
    render(
      <GlassSelect
        label="Department"
        value=""
        placeholder="Select a department"
        options={[
          { value: "engineering", label: "Engineering" },
          { value: "finance", label: "Finance & Accounts" },
        ]}
        onChange={() => undefined}
      />,
    );
    const trigger = screen.getByRole("button", { name: "Department" });

    fireEvent.click(trigger);
    const listbox = await screen.findByRole("listbox", { name: "Department" });
    expect(listbox).toHaveClass("workspace-modal-panel", "workspace-select-panel");
    expect(listbox.parentElement).toBe(document.body);
    fireEvent.pointerDown(document.body);
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
  });

  it("supports keyboard selection and submits the hidden form value", async () => {
    setViewport(1366);
    const onChange = vi.fn();
    const { container } = render(<DepartmentSelectHarness onSelection={onChange} />);

    const trigger = screen.getByRole("button", { name: "Department" });
    fireEvent.keyDown(trigger, { key: "ArrowDown" });
    const engineeringOption = await screen.findByRole("option", { name: "Engineering" });
    fireEvent.keyDown(engineeringOption, { key: "End" });
    fireEvent.keyDown(screen.getByRole("option", { name: "Finance & Accounts" }), { key: "Enter" });

    expect(onChange).toHaveBeenCalledWith("finance");
    expect(container.querySelector<HTMLInputElement>('input[name="department_id"]')?.value).toBe("finance");
  });

  it("supports type-ahead to find a department by its visible name", async () => {
    setViewport(1366);
    render(
      <GlassSelect
        label="Department"
        value=""
        placeholder="All departments"
        options={[
          { value: "building", label: "Building & Structural Construction" },
          { value: "hse", label: "Health, Safety, and Environment (HSE)" },
        ]}
        onChange={() => undefined}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Department" }));
    let activeOption = await screen.findByRole("option", { name: "Building & Structural Construction" });
    fireEvent.keyDown(activeOption, { key: "h" });
    activeOption = screen.getByRole("option", { name: "Health, Safety, and Environment (HSE)" });
    fireEvent.keyDown(activeOption, { key: "e" });
    expect(screen.getByRole("option", { name: "Health, Safety, and Environment (HSE)" })).toHaveFocus();
  });

  it("shows all department names through the shared department selector", async () => {
    setViewport(1440);
    render(
      <DepartmentSelect
        label="Filter by department"
        value=""
        departments={[
          { id: "building", name: "Building & Structural Construction" },
          { id: "civil", name: "Civil Engineering" },
          { id: "finance", name: "Finance & Accounts" },
          { id: "hse", name: "Health, Safety, and Environment (HSE)" },
          { id: "hr", name: "Human Resources & Administration" },
          { id: "procurement", name: "Procurement & Logistics" },
        ]}
        placeholder="All departments"
        emptyOptionLabel="All departments"
        onChange={() => undefined}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Filter by department" }));
    expect(await screen.findByRole("listbox", { name: "Filter by department" })).toHaveClass("no-scrollbar");
    for (const name of [
      "Building & Structural Construction",
      "Civil Engineering",
      "Finance & Accounts",
      "Health, Safety, and Environment (HSE)",
      "Human Resources & Administration",
      "Procurement & Logistics",
    ]) {
      expect(await screen.findByRole("option", { name })).toBeInTheDocument();
    }
  });

  it.each([320, 375, 768, 1024, 1440])("keeps the popover inside a %ipx viewport", async (width) => {
    setViewport(width);
    render(
      <GlassSelect
        label="Department"
        value=""
        placeholder="All departments"
        options={[{ value: "building", label: "Building & Structural Construction" }]}
        onChange={() => undefined}
      />,
    );
    const trigger = screen.getByRole("button", { name: "Department" });
    vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue({
      x: width - 108,
      y: 120,
      top: 120,
      right: width - 8,
      bottom: 160,
      left: width - 108,
      width: 100,
      height: 40,
      toJSON: () => ({}),
    });

    fireEvent.click(trigger);
    const listbox = await screen.findByRole("listbox", { name: "Department" });
    const left = Number.parseFloat(listbox.style.left);
    const menuWidth = Number.parseFloat(listbox.style.width);
    expect(left).toBeGreaterThanOrEqual(12);
    expect(left + menuWidth).toBeLessThanOrEqual(width - 12);
    cleanup();
  });

  it("opens an empty options panel with an explanation instead of appearing unresponsive", async () => {
    setViewport(1366);
    render(
      <GlassSelect
        label="Department"
        value=""
        placeholder="Select a department"
        options={[]}
        emptyMessage="No departments are available."
        onChange={() => undefined}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Department" }));
    expect(await screen.findByRole("status")).toHaveTextContent("No departments are available.");
  });
});
