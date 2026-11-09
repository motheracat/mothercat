import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GlassSelect } from "@/components/workspace/glass-select";

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
    expect(listbox.className).toBeTruthy();
    expect(listbox).toHaveStyle({ position: "fixed", zIndex: "1300", left: "240px", width: "240px" });

    fireEvent.click(screen.getByRole("option", { name: "Draft" }));
    expect(onChange).toHaveBeenCalledWith("draft");
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("opens a mobile bottom sheet, locks page scroll, and closes from the backdrop", async () => {
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
    expect(await screen.findByRole("dialog", { name: "Department" })).toBeInTheDocument();
    expect(document.body.style.overflow).toBe("hidden");
    expect(screen.getByRole("listbox", { name: "Department" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Close Department options" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(document.body.style.overflow).toBe(""));
    await waitFor(() => expect(trigger).toHaveFocus());
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
