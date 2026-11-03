import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SignInForm } from "@/components/auth/sign-in-form";

const goodCredentials = { staffId: "1234567", password: "AvalidPassword1!" };

function fillForm(password = goodCredentials.password) {
  fireEvent.change(screen.getByLabelText("Staff ID"), { target: { value: goodCredentials.staffId } });
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: password } });
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("SignInForm", () => {
  it("clears only the password and focuses it after invalid credentials", async () => {
    const onLogin = vi.fn().mockRejectedValue({ status: 401, code: "invalid_credentials" });
    render(<SignInForm onLogin={onLogin} />);
    fillForm();
    fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    await screen.findByRole("alert");
    expect(screen.getByLabelText("Staff ID")).toHaveValue(goodCredentials.staffId);
    expect(screen.getByLabelText("Password")).toHaveValue("");
    await waitFor(() => expect(screen.getByLabelText("Password")).toHaveFocus());
    expect(screen.getByLabelText("Staff ID")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Password")).toHaveAttribute("aria-invalid", "true");
  });

  it("disables sign-in and counts down after a rate limit", async () => {
    vi.useFakeTimers();
    const onLogin = vi.fn().mockRejectedValue({ status: 429, code: "rate_limited", retryAfterSeconds: 2 });
    render(<SignInForm onLogin={onLogin} />);
    fillForm();
    fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    await act(async () => Promise.resolve());
    expect(screen.getByRole("button", { name: "Sign in" })).toBeDisabled();
    expect(screen.getByText("You can try again in 00:02.")).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(screen.getByText("You can try again in 00:01.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeDisabled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(screen.getByRole("button", { name: "Sign in" })).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("shows a safe network message when the request rejects", async () => {
    const onLogin = vi.fn().mockRejectedValue(new TypeError("fetch failed"));
    render(<SignInForm onLogin={onLogin} />);
    fillForm();
    fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    expect(await screen.findByText("Can't reach the server. Check your connection and try again.")).toBeInTheDocument();
  });

  it("only renders a reference for server errors", async () => {
    const onLogin = vi.fn().mockRejectedValue({ status: 503, requestId: "1234567890abcdef" });
    render(<SignInForm onLogin={onLogin} />);
    fillForm();
    fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    expect(await screen.findByText("Reference: 1234567890ab…")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy" })).toBeInTheDocument();
  });

  it("announces login failures through an assertive alert", async () => {
    const onLogin = vi.fn().mockRejectedValue({ status: 403, code: "inactive" });
    render(<SignInForm onLogin={onLogin} />);
    fillForm();
    fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveAttribute("aria-live", "assertive");
    expect(alert).toHaveTextContent("Sign-in failed");
    expect(alert).toHaveTextContent("This account can't sign in right now. Contact your administrator.");
  });

  it("shows a fixed ID prefix and only accepts a seven-digit numeric staff ID", async () => {
    const onLogin = vi.fn();
    const user = userEvent.setup();
    render(<SignInForm onLogin={onLogin} />);
    const staffIdInput = screen.getByLabelText("Staff ID");
    expect(screen.getByText("ID:")).toBeInTheDocument();
    expect(staffIdInput).toHaveAttribute("maxLength", "7");

    await user.type(staffIdInput, "12A3456789");
    expect(staffIdInput).toHaveValue("1234567");
    await user.type(screen.getByLabelText("Password"), goodCredentials.password);
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(onLogin).toHaveBeenCalledWith(goodCredentials);
  });

  it("rejects IDs that do not contain exactly seven digits", async () => {
    const onLogin = vi.fn();
    const user = userEvent.setup();
    render(<SignInForm onLogin={onLogin} />);
    await user.type(screen.getByLabelText("Staff ID"), "123456");
    await user.type(screen.getByLabelText("Password"), goodCredentials.password);
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(onLogin).not.toHaveBeenCalled();
    expect(screen.getByText("Enter the 7-digit number after ID:.")).toBeInTheDocument();
  });
});
