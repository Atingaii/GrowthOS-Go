# Design QA — Lesson 32 Real Session Authentication

**Status:** PASSED

**QA date:** 2026-09-02

**Scope:** public login boundary, current-session view, session-state transitions, desktop and mobile presentation

## 1. Acceptance question and boundary

This review answers one concrete question: does Lesson 32 turn the public `credit.linux.do` visual direction into a coherent, responsive GrowthOS authentication experience, while making the real server-session state understandable and preserving honest product boundaries?

Included:

- the shared public `AuthLayout` header, introduction, trust points, login card, current-session card, checking state, unavailable state and signed-out notice;
- the real `/login` → `/session` → `/login` path through Nginx, the Go identity boundary and MySQL;
- ordinary reload, explicit session re-check, logout, backend failure and recovery behavior;
- a same-input desktop comparison at 1719 × 862 CSS px, a 390 × 844 mobile review and a 1280 × 720 authenticated-session review;
- keyboard focus, semantic labels, live status/error communication and reduced-motion behavior;
- final frontend tests, type checking, production build, formatting and diff checks.

Excluded:

- authorization decisions, role/permission models, business-route protection or per-role navigation. Those belong to Lesson 33 RBAC, Lesson 34 frontend permission projection and Lesson 35 privilege-escalation/E2E acceptance;
- password recovery, self-registration, SSO, MFA, device/session management and business workspace screens;
- pixel-for-pixel copying of Linux DO branding, its balance illustration, text or account data;
- direct inspection of an HttpOnly cookie from browser script, or claims based on reading browser storage internals;
- a complete browser/device/assistive-technology certification or production Core Web Vitals measurement.

This is therefore an authentication-boundary acceptance, not a claim that the complete GrowthOS permission system has shipped.

## 2. Visual truth and comparison setup

### 2.1 Evidence priority

| Priority | Evidence | Purpose | Boundary |
| --- | --- | --- | --- |
| 1 | Public reference at [`https://credit.linux.do/`](https://credit.linux.do/) and `credit-linux-do-reference-1719x862.png` | Primary truth for the white dual-column first fold, restrained purple accent, typography hierarchy, whitespace and elevated right-hand focal surface | Reference is a public marketing entry, not a GrowthOS authentication contract |
| 2 | `reference-vs-growthos-login-1719.png` | Required same-input visual comparison at the same 1719 × 862 viewport | Temporary manual-QA montage, not a checked-in asset |
| 3 | `growthos-login-latest-1719x862.png` | Final desktop login state | Temporary browser capture |
| 4 | `growthos-login-390x844-full.png` | Final narrow/mobile login state | Responsive acceptance; there is no matching mobile reference state |
| 5 | `growthos-current-session-1280x720.png` | Authenticated session hierarchy, identity projection, expiry information, re-check and logout actions | Contains only disposable test-state evidence and is not a product-data fixture |
| 6 | Real browser task flow and source/tests | Observable interaction, focus, routing, failure/recovery and semantic behavior | This visual review does not bypass browser privacy boundaries |

The captures currently live under the disposable directory `/tmp/growthos-lesson32-design-evidence.ljwm79/`. They are retained only until the Lesson 32 freeze is complete so the final comparison can be independently rechecked; they must then be removed and are not durable repository evidence.

### 2.2 Same-input comparison method

The full reference and final `/login` capture were placed into one comparison image before judging the result. Both desktop frames use 1719 × 862 CSS px. Inspection covered:

- global header height and first-fold vertical start;
- left narrative/right focal-surface proportion;
- headline scale, line length, baseline rhythm and supporting-copy contrast;
- primary violet, neutral borders, background glow and shadow restraint;
- input, password reveal and primary-action hierarchy;
- large-screen negative-space balance and mobile reading/tab order;
- whether visual confidence was supported by truthful session behavior rather than decorative security claims alone.

The objective was visual-language fidelity, not duplication. The implementation intentionally replaces the reference balance card and marketing CTAs with the real login/current-session task while retaining its core composition: quiet white canvas, two-column first fold, oversized concise headline, a single elevated focal card and a limited violet accent.

### 2.3 Final visible differences and rationale

| Area | Reference | GrowthOS Lesson 32 | Decision |
| --- | --- | --- | --- |
| Header | Landing page has no persistent product header in the captured fold | Compact 64 px GrowthOS identity and a system-status escape hatch | Accepted: establishes product provenance and gives authentication failures a useful adjacent path |
| Left column | Product promise, two CTAs and three platform qualities | Authentication purpose and three concrete session-safety boundaries | Accepted: preserves hierarchy while matching the task's security intent |
| Right column | Decorative balance surface and income chip | Real labelled login form or current-session projection | Accepted: functional focal surface replaces illustration without changing the visual grammar |
| Accent | Purple headline/action and soft blue-violet glow | Violet eyebrow/CTA/focus plus restrained radial violet/blue field | Accepted: recognizable direction without copying brand assets |
| Density | Sparse marketing first fold | Slightly denser form/session details | Accepted: necessary task information remains contained in one card |
| Mobile | No equivalent captured reference | Narrative precedes the form in DOM and visual order; secondary trust-point row is omitted | Accepted: preserves the task narrative and removes nonessential first-fold density |

## 3. Visual and product rubric

| Category | Final evidence | Result | Open severity |
| --- | --- | --- | --- |
| Composition | Desktop preserves the reference's balanced dual-column first fold; login/session card is the unambiguous task focus | Passed | none |
| Typography | Large Chinese headline, compact mono eyebrow, 24 px card title, readable 14–18 px support copy and mono principal identifier create clear levels | Passed | none |
| Spacing | Shared max-width, responsive 20/32/40 px gutters, 48–80 px page padding, consistent card/input/action gaps | Passed | none |
| Color | White/zinc foundation, restrained `#625df5` accent, emerald authenticated state, amber indeterminate state and rose authentication errors | Passed | none |
| Surfaces | One bordered `rounded-xl` task card with a restrained shadow; status blocks use borders and tint instead of competing floating cards | Passed | none |
| Iconography | Lucide icons use a consistent stroke language and are hidden when decorative; controls have textual or accessible names | Passed | none |
| Content truth | Copy distinguishes authenticated, anonymous, checking and unavailable states; a technical failure is never presented as “logged out” | Passed | none |
| Responsiveness | 1719 × 862 desktop and 390 × 844 mobile inspected; no visible clipping or page-level horizontal overflow | Passed | none |
| Interaction hierarchy | One primary login action; authenticated state separates neutral re-check from high-commitment logout | Passed | none |
| Accessibility | Skip link, main landmark, labelled form, autocomplete hints, 44–48 px controls, visible focus, status/alert semantics and focus handoff | Passed | none |
| Motion | Global `prefers-reduced-motion: reduce` rule reduces animation/transition duration and iteration; loading semantics remain available without depending on motion | Passed | none |
| Scope honesty | UI projects authentication facts only; it does not display role, scope or permission vocabulary before those lessons exist | Passed | none |

## 4. Interaction and state-machine acceptance

### 4.1 Real browser path

| Scenario | Observable result | Result |
| --- | --- | --- |
| Initial anonymous load | Current-session check resolves anonymous and presents the login form, not protected workspace content | Passed |
| Valid login | Form submits once, navigates with replacement from `/login` to `/session`, and renders the expected public principal projection | Passed |
| Post-login focus | Focus moves to the `当前会话` heading so a keyboard or assistive-technology user receives the route/state change | Passed |
| Authenticated reload | Reload remains on `/session` and reconstructs the visible session from the server boundary | Passed |
| Explicit re-check | `重新核查` re-reads the session without manufacturing a new authenticated state | Passed |
| Logout | Confirmed logout returns to `/login`, shows an explicit signed-out notice and does not leave the session view visible | Passed |
| Post-logout reload | Reload remains anonymous on `/login`; the prior principal is not rendered | Passed |
| Identity-store failure with a valid session | With MySQL unavailable, reload stays on the session route and renders `暂时无法确认登录状态`; it shows neither a login form nor stale principal data | Passed |
| Recovery | After MySQL recovery, explicit `重新核查` restores the authenticated principal projection | Passed |
| Final cleanup path | A final logout returns the browser to the anonymous login state | Passed |

The failure drill is an important product decision: `unavailable` is a separate state from `anonymous`. Treating a dependency failure as logout would create a misleading UI, invite duplicate login attempts and obscure an operational incident.

### 4.2 Form and action behavior

- The login form has an accessible name, programmatic labels, username/current-password autocomplete tokens and native required/format constraints.
- The password-reveal control exposes a stable action name and `aria-pressed`; its icon is decorative.
- Password input is not mirrored into React component state and is cleared from the form immediately after the one-shot request starts.
- Submission and logout expose busy text/live status, disable duplicate activation and retain visible focus on the active action.
- Authentication errors map to user-safe copy and move focus to the alert; a request/support identifier may be displayed without exposing backend detail.
- An ordinary logout failure keeps the authenticated snapshot visible because revocation was not proved; it does not falsely announce success or auto-retry a mutation.
- The signed-out/session-ended notices receive focus when introduced, making the state transition perceivable beyond color.

### 4.3 Concurrency correctness visible at the UI boundary

The final state machine uses abort signals and monotonically increasing check/action generations. A previously started `GET` cannot restore an old authenticated snapshot after logout begins or completes. Login, logout and re-check ownership are mutually bounded, so late promises cannot overwrite the newer state.

This is verified by source-level tests and was repaired before the final browser pass; the visual review records the observable consequence rather than claiming that a screenshot alone proves concurrency safety.

## 5. Desktop and mobile findings

### Desktop — 1719 × 862

Passed:

- the compact header leaves the first fold focused on authentication;
- both columns align around a shared vertical center and retain generous but intentional whitespace;
- the headline remains the strongest object, while the form card is the clearest action destination;
- the 28 rem card supports labels, 48 px fields and action feedback without appearing oversized;
- trust points stay secondary and do not compete with login;
- background glow is subtle enough that field borders, focus rings and body copy remain legible.

### Mobile — 390 × 844

Passed:

- the layout becomes one column with 20 px outer gutters and no horizontal clipping;
- the narrative appears before the form in both DOM and visual order, keeping keyboard traversal consistent with reading order;
- the headline scales down and wraps intentionally instead of clipping;
- the system-status link retains the accessible name `查看系统状态` even when its visible text is hidden;
- form controls remain full-width and at least 44 px high;
- the three desktop trust points are hidden to protect task density, while essential security/authorization context remains in the form card.

The mobile outcome is responsive acceptance, not reference pixel parity, because no equivalent 390 × 844 reference screen was available.

### Authenticated session — 1280 × 720

Passed:

- the card retains the same shell and visual position as login, preventing a layout jump between identity states;
- `已认证`, principal, identity type and both expiry boundaries form a readable top-to-bottom hierarchy;
- semantic description-list and `time` elements support the visible grouping;
- long principal identifiers can wrap without escaping the card;
- re-check and logout are visually separated and become a stacked action order at narrow widths.

## 6. Defects repaired before the final pass

| Finding | Severity | Repair | Final re-check |
| --- | --- | --- | --- |
| A late current-session `GET` could republish the old session after logout | P1 | Logout now advances the check generation, aborts the active read and owns the transition | Unit concurrency coverage plus real login/logout/reload path passed |
| Icon-only mobile system-status link lost a useful accessible name | P2 | Added the stable name `查看系统状态` to the link | Narrow viewport semantic check passed |
| Mobile CSS ordering could diverge from DOM/tab order | P2 | Narrative and task card now share the same logical and visual order | 390 × 844 reading and keyboard-order review passed |
| Route transition did not reliably announce the authenticated destination | P2 | Current-session heading accepts programmatic focus on entry | Real browser login confirmed heading focus |

There are no open P0, P1 or P2 findings in the Lesson 32 visual/interaction scope.

## 7. Quality gates

The authoritative frontend checkpoint recorded:

- 23 Vitest files passed;
- 250 tests passed;
- TypeScript type checking passed;
- production build passed;
- formatting check passed;
- diff/whitespace check passed.

The production entry bundle was 460.30 kB, 133.53 kB gzip. These figures describe one build artifact only; they are not claims about runtime parse cost, field performance, Core Web Vitals or a production network transfer budget.

Browser acceptance was performed against the rebuilt Lesson 32 web image and the actual same-origin session endpoints. The browser review exercised visible UI and routing behavior. It did **not** read an HttpOnly cookie from JavaScript or inspect private browser storage to manufacture proof. Cookie flags and transport contracts belong to HTTP/API acceptance and source tests, while this document records what a user can observe.

## 8. Residual boundaries and next lessons

No visual defect at P0/P1/P2 remains, but the following are deliberate scope boundaries rather than hidden completion claims:

| Follow-up | Why it is not Lesson 32 | Planned ownership |
| --- | --- | --- |
| Shared authorization vocabulary and model | Authentication establishes who the principal is; it does not decide what that principal may do | Lesson 33 RBAC |
| Permission-aware navigation, routes and controls | Client projection must derive from a server-owned authorization contract | Lesson 34 frontend permission projection |
| Direct URL/API privilege-escalation and browser E2E matrix | Hiding a control is not authorization; negative server and browser proof must follow server enforcement | Lesson 35 authorization acceptance |
| Full device/browser/screen-reader matrix and screenshot regression | Two representative viewports and targeted semantics are not certification | Later frontend hardening |
| Field performance budget | A successful build and bundle size do not establish real-user performance | Later observability/performance work |

## 9. Evidence retention and cleanup

The current `/tmp/growthos-lesson32-design-evidence.ljwm79/` reference, desktop, mobile, session and combined comparison images are disposable verification artifacts. They stay available only through the Lesson 32 freeze/review checkpoint. After the durable lesson documentation and Git tip are frozen, the exact temporary directory will be removed; no pre-existing user image, source asset or reusable project dependency is part of that cleanup.

The lasting audit trail is this document, the implementation, automated tests, recorded viewport/state dimensions and the public reference URL. No test password, private enrollment material or reusable account secret is recorded here.

## 10. Final decision

Lesson 32 passes its visual and interaction boundary. It translates the reference's quiet dual-column visual language into a real GrowthOS login/current-session journey; the responsive hierarchy, focus management, honest failure state and server-backed browser flow agree with one another.

This decision is intentionally limited to real session authentication. It does not pre-approve RBAC, permission-filtered navigation or privilege-escalation resistance in later lessons.

final result: passed
