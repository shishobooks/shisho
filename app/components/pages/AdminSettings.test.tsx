import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { ConfigResponse } from "@/types/generated/config";

import AdminSettings from "./AdminSettings";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const baseConfig: ConfigResponse = {
  database_connect_retry_count: 5,
  database_connect_retry_delay: 2_000_000_000,
  database_debug: false,
  database_file_path: "/config/shisho.db",
  database_busy_timeout: 5_000_000_000,
  database_max_retries: 5,
  server_host: "0.0.0.0",
  server_port: 3689,
  demo_mode: false,
  sync_interval_minutes: 60,
  worker_processes: 2,
  job_retention_days: 30,
  cache_dir: "/config/cache",
  download_cache_max_size_gb: 5,
  pdf_render_dpi: 200,
  pdf_render_quality: 85,
  plugin_dir: "/config/plugins/installed",
  plugin_data_dir: "/config/plugins/data",
  enrichment_confidence_threshold: 0.85,
  library_monitor_enabled: true,
  library_monitor_delay_seconds: 60,
  library_monitor_effective_delay_seconds: 60,
  supplement_exclude_patterns: [".*"],
  pdf_supplement_filenames: ["bonus"],
  session_duration_days: 30,
  test_mode: false,
  jwt_secret_too_short: false,
};

const renderPage = (config: Partial<ConfigResponse>) => {
  vi.spyOn(API, "request").mockResolvedValue({ ...baseConfig, ...config });
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AdminSettings />
    </QueryClientProvider>,
  );
};

/** The row labelled `label`, which the page exposes as a named group. */
const rowValue = async (label: string) =>
  within(await screen.findByRole("group", { name: label }));

describe("AdminSettings", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["config:read"] });
  });

  it("shows Disabled when scheduled scans and job cleanup are off", async () => {
    renderPage({ sync_interval_minutes: 0, job_retention_days: 0 });

    expect(
      (await rowValue("Sync Interval")).getByText("Disabled"),
    ).toBeVisible();
    expect(
      (await rowValue("Job Retention")).getByText("Disabled"),
    ).toBeVisible();
  });

  it("shows the configured intervals when they are on", async () => {
    renderPage({ sync_interval_minutes: 15, job_retention_days: 7 });

    expect(
      (await rowValue("Sync Interval")).getByText("15 minutes"),
    ).toBeVisible();
    expect((await rowValue("Job Retention")).getByText("7 days")).toBeVisible();
  });

  it("shows the effective monitor delay when the configured one is clamped", async () => {
    renderPage({
      library_monitor_delay_seconds: 2,
      library_monitor_effective_delay_seconds: 5,
    });

    const row = await rowValue("Monitor Delay");
    expect(row.getByText("5s")).toBeVisible();
    expect(row.getByText(/configured 2s/i)).toBeVisible();
  });

  it("describes retention as deleting completed and failed jobs", async () => {
    renderPage({});

    const row = await rowValue("Job Retention");
    expect(row.getByText(/completed and failed jobs/i)).toBeVisible();
  });

  it("reports test mode instead of a free-form environment", async () => {
    renderPage({ test_mode: false });

    expect((await rowValue("Test Mode")).getByText("No")).toBeVisible();
  });

  it("warns when the JWT secret is shorter than 32 characters", async () => {
    renderPage({ jwt_secret_too_short: true });

    const row = await rowValue("JWT Secret");
    expect(row.getByText("Too short")).toBeVisible();
    expect(row.getByText(/openssl rand -hex 32/)).toBeVisible();
  });

  it("does not show the JWT secret row when the secret is long enough", async () => {
    renderPage({});

    await screen.findByText("Session Duration");
    expect(
      screen.queryByRole("group", { name: "JWT Secret" }),
    ).not.toBeInTheDocument();
  });
});
