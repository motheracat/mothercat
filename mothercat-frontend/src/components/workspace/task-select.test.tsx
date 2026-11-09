import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TaskSelect } from "@/components/workspace/task-select";

afterEach(cleanup);

describe("TaskSelect", () => {
  it("opens its styled options and updates the selected value", () => {
    const onChange = vi.fn();
    render(
      <TaskSelect
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

    fireEvent.click(screen.getByRole("button", { name: "Task status" }));
    expect(screen.getByRole("listbox", { name: "Task status" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("option", { name: "Draft" }));

    expect(onChange).toHaveBeenCalledWith("draft");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("keeps a named selection in the form value", () => {
    render(
      <TaskSelect
        label="Department"
        name="department_id"
        value="department-2"
        placeholder="Select a department"
        options={[
          { value: "department-1", label: "Engineering" },
          { value: "department-2", label: "Operations" },
        ]}
        onChange={() => undefined}
      />,
    );

    expect(screen.getByRole("button", { name: "Department" })).toHaveTextContent("Operations");
    expect(document.querySelector<HTMLInputElement>('input[name="department_id"]')?.value).toBe("department-2");
  });
});
