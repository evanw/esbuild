# Principle 4: Test The Lies

**Core idea:** If you cannot write the test, you do not understand the problem. And if you do not understand the problem, you should not ask the AI to write the solution.

---

## The Three Lies Framework

### Small Lies -- Unit Tests

Unit tests catch the small lies. They verify that each piece works alone.
- A function returns the right value
- A validation rejects the wrong input
- A calculation produces the correct result

### Big Lies -- End-to-End Tests

E2E tests catch the big lies. They verify that the pieces work together.
- The user can navigate from login to checkout
- The payment flow handles declined cards
- The email change requires confirmation

### Human Lies -- Agent-Based Tests

Agent-based tests catch the human lies. They verify that real users can actually use the feature.
- The agent spins up a browser
- Logs in with test credentials
- Navigates to the feature
- Clicks buttons, fills forms
- Takes screenshots, compares with Figma
- Catches bugs that unit tests miss because it tests like a human tests

**When all three types pass, you have proof. Not hope. Proof.**

---

## The Hourglass Testing Model

The testing pyramid was designed for humans writing code by hand. AI changes the economics.

```
         +---------------------+
         |   E2E Specs (heavy)  |  <-- Humans write these
         |   User flows as       |
         |   Given/When/Then     |
         +----------+-----------+
                    |
            +-------v-------+
            |  UI Code       |  <-- AI generates this
            |  (thin,        |    Disposable.
            |   disposable)  |    Regenerate, don't fix.
            +-------+-------+
                    |
    +---------------v---------------+
    |  Business Logic Tests (heavy)  |  <-- MCP tools + unit tests
    |  Validation, contracts, state   |    Bulletproof.
    |  transitions, edge cases        |    Never skip.
    +-------------------------------+
```

### Distribution

| Layer | Share | What to test |
|-------|-------|-------------|
| MCP tool tests | 70% | Every user story becomes an MCP tool with typed schema, handler, and structured response. Tests run in milliseconds. They never flake. No DOM, no network, no animation timing. |
| E2E specs | 20% | Written in Given/When/Then format. Verify full user flows through actual UI, but only verify wiring. Business logic is already proven. |
| UI component tests | 10% | Only what is unique to UI: accessibility, responsive layout, animation behavior, keyboard navigation. If the test asserts a business rule, it belongs in the MCP tool test. |

**You have not lost any coverage. You have lost the browser.**

---

## Test Type Decision Guide

| Question | If Yes | If No |
|----------|--------|-------|
| Does it test a business rule? | MCP tool test | Continue |
| Does it test a user flow end-to-end? | E2E spec | Continue |
| Does it test UI-specific behavior? | UI component test | It probably does not need a test |
| Is the test flaky? | Fix it or delete it | Keep it |
| Does the test assert on DOM structure? | Move logic to MCP tool test | Keep as UI test |

---

## Actionable Takeaways

1. To write a test, you must understand the code. Tests prove understanding.
2. Use the Three Lies Framework: unit (small), E2E (big), agent-based (human).
3. Follow the Hourglass Model: heavy on business logic, light on UI.
4. 70% MCP tool tests, 20% E2E, 10% UI component tests.
5. If a test is flaky, fix it or delete it. Flaky tests gaslight the AI.

---

---

## Spike Land in Practice

**Chess tools follow the Hourglass perfectly.** The 4 chess MCP test files (`chess-game.test.ts`, `chess-player.test.ts`, `chess-challenge.test.ts`, `chess-replay.test.ts`) test pure business logic — no DOM, no network, no animation timing. They run in milliseconds and never flake. The chess UI in `src/app/apps/chess-arena/` is thin and disposable; all game logic lives in `src/lib/chess/` and is tested through MCP tool tests.

**94 of 98 tool files have tests.** This 96% file coverage means agents can safely refactor nearly any MCP tool — the test suite catches regressions. The 4 untested files are infrastructure utilities (`tool-factory.ts`, `tool-helpers.ts`, `bootstrap.ts`, `capabilities.ts`), not business logic. This matches the Hourglass model: heavy testing on business logic, minimal on infrastructure wiring.

**The `createMockRegistry()` pattern.** Every MCP tool test in the codebase uses this pattern: create a mock registry, register the tool under test, call it with typed inputs, assert on structured outputs. This is the "MCP tool test" layer of the Hourglass — 70% of all testing effort. No browser required, runs in CI in seconds.

---

## Bayesian Bugbook

### The Rule

**Bug appears twice → mandatory Bugbook entry.** No exceptions. If a bug recurs, it has earned a permanent record and a regression test.

### Confidence Lifecycle

| Event | Confidence | Status | Action |
|-------|-----------|--------|--------|
| First observed | 0.5 | CANDIDATE | Log conditions: what happened, where, when |
| Second occurrence | 0.6+ | ACTIVE | Full Bugbook entry (see format below) |
| Fix prevents recurrence | +0.1 per prevention | ACTIVE | Confidence grows as the fix proves itself |
| Irrelevant for 5+ sessions | -0.1 decay | Decaying | May no longer apply to current codebase |
| Below 0.3 | <0.3 | DEPRECATED | Archived, no longer checked actively |

### Bugbook Entry Format

```
## BUG-042: [Short descriptive name]

**Symptom:** What the user or agent observes when the bug triggers.
**Cause:** Root cause analysis — why it happens, not just what happens.
**Fix Steps:** Exact steps to fix, including file paths and code changes.
**Verification:** How to confirm the fix works (test name, manual steps, CI check).
**Confidence:** 0.6 (ACTIVE) — observed twice, fix applied, pending long-term verification.
```

### Integration with Three Lies

Every ACTIVE Bugbook entry requires a corresponding regression test:

| Bug Scope | Test Type | Why |
|-----------|-----------|-----|
| Single function / calculation | Unit test (Small Lie) | Fast feedback, catches the exact recurrence |
| Cross-component / user flow | E2E test (Big Lie) | Verifies the fix holds across integration boundaries |
| Usability / interaction pattern | Agent-based test (Human Lie) | Catches the bug the way a human encounters it |

**The Bugbook is not a graveyard. It is a living defense system.** Entries earn their place through recurrence and lose it through irrelevance.

---

*Sources: Blog posts 08 (How to Not Produce AI Slop), The Testing Pyramid Is Upside Down, Think Slowly Ship Fast, Asterobia BUGBOOK.md*
