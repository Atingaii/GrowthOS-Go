# Design QA — Lesson 22 Credits Workspace

**Status:** PASSED
**QA date:** 2026-08-30
**Scope:** GrowthOS user workspace, Lottery workbench, Admin/MCP/Agent operator workspaces, desktop and mobile shell behavior

## 1. Scope and acceptance question

This review answers one concrete question: does the Lesson 22 frontend now express the high-density, flat, account-workspace language requested from `credit.linux.do`, while keeping GrowthOS branding, real feature boundaries, responsive behavior, and accessible interaction intact?

Included:

- shared `WorkspaceShell` used by User, Admin, MCP, and Agent layouts;
- user Home, Growth Feed, Campaigns/list-detail, Points, Coupons, Profile, and Lottery;
- operator dashboards, data views, generic unavailable modules, and local-only Agent task behavior;
- search palette, sidebar collapse, mobile drawer, notification sample, theme, full-width, profile and primary navigation actions;
- a 1719 × 862 desktop comparison and a 390 × 844 mobile implementation review;
- semantic, interaction, content-truth, and build checks.

Excluded:

- pixel-for-pixel cloning of Linux DO branding, account data, logo, or proprietary authenticated content;
- formal usability research, screen-reader certification, a full browser/device matrix, and automated visual regression infrastructure;
- production authentication, live account data, server notifications, formal Lottery Draw/issuance, and operator write APIs.

## 2. Visual truth and comparison setup

### 2.1 Source priority

| Priority | Source | Use in this review | Limitation |
| --- | --- | --- | --- |
| 1 | User-provided authenticated screenshot, `codex-clipboard-e48ac4a1-5416-4566-8c65-8d8c64fcdbea.png` | Primary truth for authenticated home geometry, density, hierarchy, chart/summary relationship, sidebar and topbar | Session attachment; not a repository dependency |
| 2 | [`https://credit.linux.do/`](https://credit.linux.do/) | Public live behavior, basic responsive/context check | Unauthenticated access cannot reproduce the supplied authenticated home state |
| 3 | [`linux-do/credit`](https://github.com/linux-do/credit) | Official public implementation context and provenance | Repository availability does not make every visual state public or stable |
| 4 | GrowthOS source and requirements | Product copy, local data truth, capabilities, brand color, navigation | Must not be overridden merely to imitate the reference |

The user's earlier GrowthOS screenshot, `codex-clipboard-63b547ac-74a4-41ec-8811-9de65277e6af.png`, was treated as the implementation baseline: it showed an over-wide horizontal navigation, excessive blank space, and large coupon-card composition that did not match the requested workspace model.

### 2.2 State, dimensions, and density normalization

| Dimension | Source | Implementation |
| --- | --- | --- |
| Page state | Authenticated home, light theme, expanded sidebar | `/home`, light theme, expanded sidebar, local mock snapshot |
| Source/implementation viewport | 1719 × 862 CSS px | 1719 × 862 CSS px |
| Raster | 3438 × 1724 px source bitmap | Browser capture at the 1719 × 862 CSS viewport |
| Density treatment | Source normalized from 2× to 1× CSS dimensions | Implementation kept at its native CSS crop |
| Comparison | Full-view stitched pair, followed by focused inspection of shell and data regions | Same comparison input as source |

- **Source visual-truth record:** the user attachment basename above, plus the public URL and official repository; the attachment remains conversation input rather than a checked-in asset.
- **Implementation screenshot path:** a disposable QA browser-capture path, deliberately removed after review and therefore not published as a stable repository path or link.
- **Comparison input path:** a disposable same-input montage path, likewise removed after manual inspection and not retained as deliverable evidence.

The full reference and implementation were put into the same temporary comparison input. Focused inspection covered:

- sidebar width, navigation rhythm, product identity and footer;
- 72 px topbar, search position and action density;
- page start line, trend chart, right-hand summary and first fold;
- recent overview section, row density, borders and chart colors;
- typography scale, numeric alignment, icons, empty space and surface treatment.

The comparison inputs and browser captures were disposable QA artifacts. They were manually inspected but are intentionally not retained, embedded, or linked from the repository.

### 2.3 Geometry target

The accepted desktop geometry is:

```text
desktop sidebar      231 px
topbar                72 px
shell max width     1320 px
large-screen padding  48 px × 2
net content width   1224 px
```

`WorkspaceShell` owns these values for both header and main. Individual pages no longer add a second global max-width or shell padding. At smaller breakpoints the horizontal padding reduces to 32, 24, and 16 px; below `md`, the fixed desktop sidebar is removed from layout and replaced by a drawer.

## 3. Visual rubric

| Category | Acceptance evidence | Result | Severity after final pass |
| --- | --- | --- | --- |
| Typography | System font stack, compact Chinese hierarchy, 11–14 px supporting text, tabular/monospace IDs and metrics, no accidental marketing-display typography in data views | Passed | none |
| Spacing/layout | Shared 231/72/1320/1224 geometry; compact rows; consistent 16/24/32/48 responsive gutters; no page-level double container | Passed | none |
| Color/tokens | White/zinc canvases, 1 px zinc borders, restrained violet primary, blue trend, green positive, red expense/risk; dark theme remains legible | Passed | none |
| Surface language | Flat grouped regions, light row fills and small 6/8/12 px radii; shadows restricted mainly to overlays/tooltips | Passed | none |
| Image/asset fidelity | GrowthOS identity retained; no Linux DO logo/data copied; Lucide icons share a consistent stroke; mock avatar remains explicitly demo content | Passed with boundary | P3 external-avatar resilience |
| Copy/content | `2026-03-14 12:00 CST` snapshot stated; Lottery says candidate, ephemeral, non-persistent; local/disabled operator capabilities are explicit | Passed | none |
| Iconography | Action icons have labels or accessible names; decorative icons are hidden from assistive technology; no emoji or arbitrary inline icon mix | Passed | none |
| Density | Home trend/summary and recent overview occupy the first fold without the previous large coupon-card voids | Passed | none |
| Responsiveness | 1719 × 862 desktop and 390 × 844 mobile inspected; drawer replaces fixed sidebar; local table overflow stays local | Passed | none |
| Interaction/accessibility | Skip link, main landmark, visible focus, keyboard search, dialog focus containment, Escape/overlay close, focus return, pressed/expanded states, reduced-motion hooks | Passed | none |
| Product honesty | Real actions work; mocks are dated; unavailable writes are disabled; notification and Agent task state are clearly local | Passed | none |

## 4. Iteration and comparison history

| Iteration | Finding | Severity | Change | Re-check |
| --- | --- | --- | --- | --- |
| Baseline | Horizontal mega-navigation, large blank canvas and oversized coupon cards did not express the reference's account workspace | P1 | Replaced the global composition with a shared sidebar/topbar/content shell | Desktop full-view rerun |
| Shell v1 | User pages could align, but independent Admin/MCP/Agent shells would drift in padding, search and actions | P1 | Made all four layouts compose `WorkspaceShell` with navigation data only | Cross-workspace route review |
| Geometry | Repeated max-width/padding risked nested 1320 px containers and undersized content | P1 | Made shell the only global geometry owner; fixed 231/72/1320/1224 model | Same-viewport overlay inspection |
| Content density | Home and supporting pages still needed row/table hierarchy rather than repeated promotional cards | P2 | Rebuilt Home, Feed, Campaigns, Points, Coupons and Profile with flat groups and compact metrics | Full page and first-fold review |
| Product truth | Decorative actions and undated numbers could imply live services | P1 | Added snapshot label, disabled unavailable writes, real routes/Clipboard, local notification disclosure | Interaction and copy review |
| Lottery v1 | Standalone theater treatment could feel detached from the Credits workspace; disclosure was too repetitive on narrow screens | P2 | Fit Lottery into shared content geometry and separated responsive disclosures | Desktop/mobile Lottery rerun |
| Interaction hardening | Search path duplication, modal focus, drawer body scroll/focus return, and notification semantics needed explicit handling | P1 | De-duplicated search items; added containment, Escape, lock, return and ARIA states | Keyboard and mobile rerun |
| Operator unification | Generic operator pages could present empty actions as implemented features | P2 | Used compact capability-boundary pages and disabled unavailable writes | Admin/MCP/Agent route review |
| Performance review | Initial production build emitted a nonvisual >500 kB single-chunk warning | P3 | Lazy-loaded Home/Admin dashboard routes and isolated the shared ProductPage/Recharts layer in `06a4a38` | Clean production build without the warning |
| Final | No visible P0/P1/P2 remained in the compared desktop state or representative mobile state | — | No further visual correction required for Lesson 22 scope | Passed |

Every P0/P1/P2 listed above was followed by a fresh implementation capture or interaction pass. The final pass did not reuse the baseline judgment.

## 5. Functional acceptance

### 5.1 Shared shell

- Sidebar collapse changes both the visual width and main offset; expanded state is exposed through `aria-expanded`.
- Mobile menu opens a labelled modal drawer, locks background scrolling, traps focus, closes via Escape/overlay/close button, and returns focus to the opener.
- `Cmd/Ctrl + K` opens search; duplicate aliases with the same path are de-duplicated; selecting a result navigates and closes.
- Theme changes the document theme and exposes pressed state.
- Full-width toggles between the 1320 px container and unconstrained shell width.
- Notification opens a labelled local-sample region; mark-read only mutates browser state and says so.
- Settings, primary action, user avatar/home identity, and cross-workspace navigation resolve to actual routes.

### 5.2 User pages

- Home: chart, summaries, recent overview, income/expense and campaign snapshot render at desktop density without hiding the snapshot boundary.
- Growth Feed: content is readable; publish/like/comment/share remain visibly unavailable instead of feigning success.
- Campaigns: search/status filtering and detail navigation work; no fake enrollment, qualification, budget write or reward issuance.
- Points: semantic ledger table and local horizontal overflow; numbers do not claim real account value.
- Coupons: status filtering and real Clipboard API feedback; copying does not mutate coupon status.
- Profile: semantic read-only identity fields; no fake save action.

### 5.3 Operator pages

- Admin, MCP and Agent routes use the same shell and maintain their own navigation labels.
- Admin/MCP values say they are snapshots, not live operational telemetry.
- Agent can add an in-memory local task; it does not call an Agent, MCP tool, or write API, and it does not claim persistence.
- Approval/publish/create actions without backends are disabled or rendered as unavailable capabilities.

### 5.4 Lottery API

- Canonical uint64 Strategy IDs are sent as strings to the real same-origin ephemeral endpoint.
- The request has the required demo header, no query/body/idempotency key, and no automatic retry.
- Loading blocks duplicate submission; stale promises cannot replace the current state.
- `reward` means selected reward candidate; `no_reward` is a successful selection.
- Invalid contract, 404, 502/503/504, network, timeout and cancellation stay distinguishable.
- Request ID is correlation only; refresh does not recover a selection.

## 6. Viewport coverage

### Desktop — 1719 × 862

Passed:

- source and implementation share the same CSS viewport;
- expanded 231 px sidebar and 72 px header align with the target hierarchy;
- header/main use the same 1320 px outer width and 1224 px net content width;
- trend and summary remain in the first fold;
- primary user and operator routes maintain consistent page starts and gutters;
- overlays stay within the viewport and do not shift underlying geometry.

### Mobile — 390 × 844

Passed:

- no fixed desktop sidebar offset remains;
- topbar actions remain reachable without page-level horizontal overflow;
- drawer uses at most 84vw/292 px, leaving a visible dismissal area;
- headings/actions stack, charts resize, Lottery panels become one column, and wide tables scroll locally;
- interactive targets and visible focus remain usable.

There was no authenticated reference screenshot for the same mobile state. Mobile is therefore marked as responsive/product acceptance, not reference pixel parity.

## 7. Quality gates and residual findings

At documentation time:

- all 19 Vitest files and all 152 tests passed; the final checkpoint rerun remains authoritative;
- `tsc --noEmit` passed;
- Vite production build passed without the previous >500 kB single-chunk warning;
- real-browser desktop and mobile passes completed;
- the normalized, stitched desktop comparison was manually inspected.

The final build split recorded by the production gate is:

- entry: 433.34 kB;
- shared ProductPage/Recharts layer: 353.32 kB;
- Home route: 52.07 kB;
- Admin dashboard route: 2.08 kB.

These are build artifact sizes, not claims about compressed transfer cost, runtime parse cost, Core Web Vitals, or production network performance.

Residual findings:

| Severity | Finding | Disposition |
| --- | --- | --- |
| P3 | Mock avatar depends on an external Unsplash URL | Keep explicit mock status; add a local fallback before offline/production hardening |
| P3 | Two representative viewports and manual QA are not a full regression matrix | Add automated screenshots, screen-reader passes and browser/device coverage later |
| P3 | A clean chunk-size build is not a field-performance result | Add a performance budget plus real-browser Core Web Vitals/network profiling before production claims |

There are no open P0, P1, or P2 findings in this review scope.

## 8. Evidence boundary and cleanup

No disposable output screenshot is a deliverable. The reference, implementation, and focused regions were combined into temporary same-input comparison artifacts and manually inspected. Those files are intentionally absent from Git and from this document's links, and are removed during final task cleanup.

The durable audit trail is this document plus source code, tests, viewport/state/dimension records, public reference URLs, and the user-provided requirement context. This avoids leaving generated images as stale evidence while preserving enough information to reproduce the review.

## 9. Final decision

The implementation satisfies the requested visual direction at the product-language level: dense and flat account workspace, shared geometry, clear hierarchy, responsive shell, functional controls, and explicit data/capability boundaries. It does not claim to be a Linux DO clone or a production-complete GrowthOS system.

The earlier bundle-size warning was nonvisual and has now been eliminated by route-level splitting. Desktop, mobile, interaction, content-truth and clean-build evidence is sufficient for the stated Lesson 22 scope.

final result: passed
