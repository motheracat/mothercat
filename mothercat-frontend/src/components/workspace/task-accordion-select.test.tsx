import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TaskAccordionSelect } from "@/components/workspace/task-accordion-select";

afterEach(cleanup);

function DepartmentSelectHarness({ onSelection }: { onSelection: (value: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <TaskAccordionSelect
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

describe("TaskAccordionSelect", () => {
  it("uses the shared portal dropdown and closes after selection", async () => {
    const onChange = vi.fn();
    render(
      <TaskAccordionSelect
        label="Task status"
        value=""
        placeholder="All statuses"
        options={[
          { value: "", label: "All statuses" },
          { value: "draft", label: "Draft" },
          { value: "assigned", label: "Assigned" },
        ]}
        onChange={onChange}
      />,
    );

    const trigger = screen.getByRole("button", { name: "Task status" });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    fireEvent.click(trigger);
    const options = screen.getByRole("listbox", { name: "Task status" });
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(options).toHaveClass("no-scrollbar");
    expect(options.parentElement).toBe(document.body);

    fireEvent.click(screen.getByRole("option", { name: "Draft" }));
    expect(onChange).toHaveBeenCalledWith("draft");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("submits a named selection through a hidden form value", () => {
    const onSelection = vi.fn();
    const { container } = render(<DepartmentSelectHarness onSelection={onSelection} />);

    fireEvent.click(screen.getByRole("button", { name: "Department" }));
    fireEvent.click(screen.getByRole("option", { name: "Finance & Accounts" }));

    expect(onSelection).toHaveBeenCalledWith("finance");
    expect(container.querySelector<HTMLInputElement>('input[name="department_id"]')?.value).toBe("finance");
  });

  it("supports type-ahead selection for long department names", () => {
    render(
      <TaskAccordionSelect
        label="Department"
        value=""
        placeholder="Select a department"
        options={[
          { value: "building", label: "Building & Structural Construction" },
          { value: "hse", label: "Health, Safety, and Environment (HSE)" },
        ]}
        onChange={() => undefined}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Department" }));
    fireEvent.keyDown(screen.getByRole("option", { name: "Building & Structural Construction" }), { key: "h" });
    const hseOption = screen.getByRole("option", { name: "Health, Safety, and Environment (HSE)" });
    fireEvent.keyDown(hseOption, { key: "e" });
    expect(hseOption).toHaveFocus();
  });

  it("explains when there are no options and closes with Escape", async () => {
    render(
      <TaskAccordionSelect
        label="Department"
        value=""
        placeholder="Select a department"
        emptyMessage="No departments are available."
        options={[]}
        onChange={() => undefined}
      />,
    );

    const trigger = screen.getByRole("button", { name: "Department" });
    fireEvent.click(trigger);
    expect(screen.getByRole("status")).toHaveTextContent("No departments are available.");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
