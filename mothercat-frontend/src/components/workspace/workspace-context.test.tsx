import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useWorkspace, WorkspaceContextProvider } from "@/components/workspace/workspace-context";

afterEach(cleanup);

function ToastActions() {
  const { notify } = useWorkspace();
  return (
    <>
      <button type="button" onClick={() => notify("Saved successfully.")}>Success</button>
      <button type="button" onClick={() => notify("The import needs review.", "warning")}>Warning</button>
      <button type="button" onClick={() => notify("The request failed.", "error")}>Error</button>
      <button type="button" onClick={() => notify("The result was loaded.", "info")}>Info</button>
    </>
  );
}

describe("WorkspaceContextProvider toasts", () => {
  it("uses an outcome-matched color and accessible announcement for each tone", () => {
    render(
      <WorkspaceContextProvider session={null} devOpen>
        <ToastActions />
      </WorkspaceContextProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Warning" }));
    expect(screen.getByRole("status")).toHaveClass("workspace-toast-warning");
    expect(screen.getByText("Needs attention")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Error" }));
    expect(screen.getByRole("alert")).toHaveClass("workspace-toast-error");
    expect(screen.getByText("Action failed")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Success" }));
    expect(screen.getByRole("status")).toHaveClass("workspace-toast-success");
    expect(screen.getByText("Success", { selector: ".workspace-toast-title" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Info" }));
    expect(screen.getByRole("status")).toHaveClass("workspace-toast-info");
    expect(screen.getByText("Information")).toBeInTheDocument();
  });

  it("lets users dismiss a toast", () => {
    render(
      <WorkspaceContextProvider session={null} devOpen>
        <ToastActions />
      </WorkspaceContextProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Success" }));
    fireEvent.click(screen.getByRole("button", { name: "Dismiss notification" }));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
