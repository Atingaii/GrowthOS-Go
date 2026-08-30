// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MOCK_SNAPSHOT_LABEL, mockUser } from "../../../mocks/growthOsMockData";
import { useAppStore } from "../../../stores/appStore";
import { UserProfilePage } from "./UserProfilePage";

afterEach(() => {
  cleanup();
  useAppStore.setState({ user: mockUser });
});

describe("UserProfilePage", () => {
  it("renders the mock identity as a semantic definition list", () => {
    const { container } = render(<UserProfilePage />);

    expect(screen.getByRole("heading", { level: 1, name: "个人中心" })).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2, name: "身份资料" })).toBeTruthy();
    expect(screen.getByRole("heading", { level: 3, name: mockUser.name })).toBeTruthy();
    expect(screen.getByRole("img", { name: `${mockUser.name} 的头像` })).toBeTruthy();

    expect(container.querySelectorAll("dl")).toHaveLength(1);
    expect(container.querySelectorAll("dt")).toHaveLength(6);
    expect(container.querySelectorAll("dd")).toHaveLength(6);
    expect(screen.getByText(mockUser.id)).toBeTruthy();
    expect(screen.getAllByText(mockUser.email)).toHaveLength(2);
    expect(screen.getByText("管理员")).toBeTruthy();
    expect(screen.getByText("铂金成长会员")).toBeTruthy();
    expect(screen.getByText("12,450 PTS")).toBeTruthy();
    expect(screen.getByText("3 条")).toBeTruthy();
  });

  it("states the data boundary without inventing account security capabilities", () => {
    render(<UserProfilePage />);

    expect(screen.getByText(/本地 mockUser 资料/)).toBeTruthy();
    expect(screen.getByText(new RegExp(MOCK_SNAPSHOT_LABEL))).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2, name: "数据边界" })).toBeTruthy();
    expect(screen.getByText(/不输出任何无法由数据证明的认证结论/)).toBeTruthy();
    expect(screen.queryByText("已认证")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
