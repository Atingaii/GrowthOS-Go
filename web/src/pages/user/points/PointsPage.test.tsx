// @vitest-environment jsdom

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  MOCK_SNAPSHOT_LABEL,
  mockPointTransactions,
  mockUser,
} from "../../../mocks/growthOsMockData";
import { useAppStore } from "../../../stores/appStore";
import { PointsPage } from "./PointsPage";

afterEach(() => {
  cleanup();
  useAppStore.setState({ user: mockUser });
});

describe("PointsPage", () => {
  it("presents the balance as a local demonstration rather than real-time accounting", () => {
    const { container } = render(<PointsPage />);

    expect(screen.getByRole("heading", { level: 1, name: "积分资产中心" })).toBeTruthy();
    expect(screen.getByText("演示数据")).toBeTruthy();
    expect(screen.getByText(/本地 Mock 快照/)).toBeTruthy();
    expect(screen.getByText(new RegExp(MOCK_SNAPSHOT_LABEL))).toBeTruthy();
    expect(screen.getByText(/不代表实时账户、清算结果或可兑付资产/)).toBeTruthy();
    expect(screen.queryByText(/实时对接/)).toBeNull();

    const summary = screen.getByLabelText("演示积分摘要");
    expect(summary.tagName).toBe("DL");
    expect(container.querySelectorAll("dt")).toHaveLength(4);
    expect(container.querySelectorAll("dd")).toHaveLength(4);
    expect(within(summary).getByText("12,450 PTS")).toBeTruthy();
    expect(within(summary).getByText("+1,750 PTS")).toBeTruthy();
    expect(within(summary).getByText("-2,000 PTS")).toBeTruthy();
    expect(within(summary).getByText("4 条")).toBeTruthy();
  });

  it("renders a labelled semantic ledger with every mock transaction", () => {
    render(<PointsPage />);

    const table = screen.getByRole("table", { name: "本地 Mock 积分账单" });
    expect(within(table).getAllByRole("columnheader")).toHaveLength(5);
    expect(within(table).getAllByRole("row")).toHaveLength(mockPointTransactions.length + 1);

    for (const transaction of mockPointTransactions) {
      expect(within(table).getByText(transaction.id)).toBeTruthy();
      expect(within(table).getByText(transaction.title)).toBeTruthy();
    }

    expect(screen.getByText("非实时账务 · 不可用于对账")).toBeTruthy();
  });
});
