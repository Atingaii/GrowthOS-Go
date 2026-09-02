// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router";

import { readCurrentSession } from "../api/sessionApi";
import { appRoutes } from "./appRouter";

vi.mock("../api/sessionApi", async (importOriginal) => {
  const original = await importOriginal<typeof import("../api/sessionApi")>();
  return { ...original, readCurrentSession: vi.fn() };
});

const mockedReadCurrentSession = vi.mocked(readCurrentSession);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  document.documentElement.classList.remove("dark");
  document.body.style.overflow = "";
});

describe("Lesson 32 route boundary", () => {
  it.each(["/home", "/admin/dashboard", "/mcp/dashboard", "/agent/workspace"])(
    "keeps %s outside session checking and redirects",
    async (pathname) => {
      const router = createMemoryRouter(appRoutes, { initialEntries: [pathname] });
      const { container } = render(<RouterProvider router={router} />);

      await screen.findByRole("main");

      expect(router.state.location.pathname).toBe(pathname);
      expect(mockedReadCurrentSession).not.toHaveBeenCalled();
      expect(container.textContent).not.toContain("csrf-route-test-sentinel");
    },
  );
});
