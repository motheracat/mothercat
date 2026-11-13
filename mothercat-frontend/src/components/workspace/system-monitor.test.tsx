import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SystemMonitor } from "@/components/workspace/system-monitor";

const mocks = vi.hoisted(() => ({
  getEndpointAnalytics: vi.fn(),
  getMonitorEndpoints: vi.fn(),
  getMonitorEvents: vi.fn(),
  getMonitorHistory: vi.fn(),
  getMonitorOverview: vi.fn(),
  getMonitorSecurityStatus: vi.fn(),
}));

vi.mock("@/lib/system-monitor/api", () => ({
  beginMFAEnrollment: vi.fn(),
  confirmMFAEnrollment: vi.fn(),
  disableMonitorMFA: vi.fn(),
  getEndpointAnalytics: mocks.getEndpointAnalytics,
  getMonitorEndpoints: mocks.getMonitorEndpoints,
  getMonitorEvents: mocks.getMonitorEvents,
  getMonitorHistory: mocks.getMonitorHistory,
  getMonitorOverview: mocks.getMonitorOverview,
  getMonitorSecurityStatus: mocks.getMonitorSecurityStatus,
  getSecurityConsole: vi.fn(),
  revokeMonitorSession: vi.fn(),
  setEndpointControl: vi.fn(),
  SystemMonitorApiError: class extends Error {},
}));

const endpoint = {
  method: "GET",
  route: "/api/v1/tasks/{taskID}",
  description: "Get task",
  controller: "Tasks",
  pausable: true,
  health: "healthy",
  requests: 2,
  failed_requests: 0,
  error_rate: 0,
  status_distribution: {},
  latency_buckets: {},
  paused: false,
};

beforeEach(() => {
  mocks.getEndpointAnalytics.mockResolvedValue([{
    bucket: "2026-11-13T10:00:00Z",
    requests: 2,
    successes: 2,
    client_errors: 0,
    server_errors: 0,
    error_rate: 0,
    average_latency_ms: 12,
    p95_latency_ms: 50,
    maximum_latency_ms: 15,
    status_distribution: { "200": 2 },
    latency_distribution: { "<50ms": 2 },
  }]);
  mocks.getMonitorEndpoints.mockResolvedValue([endpoint]);
  mocks.getMonitorEvents.mockResolvedValue([]);
  mocks.getMonitorHistory.mockResolvedValue([]);
  mocks.getMonitorOverview.mockResolvedValue({
    started_at: "2026-11-13T00:00:00Z",
    uptime_seconds: 3600,
    database_status: "healthy",
    database: {
      open_connections: 1,
      in_use: 0,
      idle: 1,
      wait_count: 0,
      wait_duration_ms: 0,
    },
    endpoint_count: 1,
    analytics_pending_buckets: 0,
    analytics_dropped_requests: 0,
  });
  mocks.getMonitorSecurityStatus.mockResolvedValue({ mfa_enabled: false });
  Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
    configurable: true,
    value: vi.fn(),
  });
});

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  vi.clearAllMocks();
});

describe("SystemMonitor endpoint analytics", () => {
  it("loads and reveals analytics when an endpoint's Analytics button is clicked", async () => {
    render(<SystemMonitor />);

    fireEvent.click(await screen.findByRole("button", { name: /Endpoints/ }));
    const analyticsButton = await screen.findByRole("button", {
      name: "Load analytics for GET /api/v1/tasks/{taskID}",
    });
    fireEvent.click(analyticsButton);

    await waitFor(() => {
      expect(mocks.getEndpointAnalytics).toHaveBeenCalledWith(
        "GET",
        "/api/v1/tasks/{taskID}",
      );
    });
    expect(await screen.findByText("12.0", { exact: false })).toBeInTheDocument();

    const analyticsPanel = screen.getByRole("heading", {
      name: "GET /api/v1/tasks/{taskID}",
    }).closest("section");
    expect(analyticsPanel?.scrollIntoView).toHaveBeenCalledWith({
      behavior: "smooth",
      block: "center",
    });
  });
});
