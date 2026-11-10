import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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
  it("expands options inline and closes after selection", () => {
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
    fireEvent.click(trigger);
    const options = screen.getByRole("group", { name: "Task status options" });
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(options.parentElement).toBe(trigger.parentElement);

    fireEvent.click(screen.getByRole("button", { name: "Draft" }));
    expect(onChange).toHaveBeenCalledWith("draft");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();
  });

  it("submits a named selection through a hidden form value", () => {
    const onSelection = vi.fn();
    const { container } = render(<DepartmentSelectHarness onSelection={onSelection} />);

    fireEvent.click(screen.getByRole("button", { name: "Department" }));
    fireEvent.click(screen.getByRole("button", { name: "Finance & Accounts" }));

    expect(onSelection).toHaveBeenCalledWith("finance");
    expect(container.querySelector<HTMLInputElement>('input[name="department_id"]')?.value).toBe("finance");
  });

  it("explains when there are no options and closes with Escape", () => {
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
    fireEvent.keyDown(screen.getByRole("group", { name: "Department options" }), { key: "Escape" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveFocus();
  });
});
