---
name: bazdmeg
description: "BAZDMEG Method workflow checkpoint system for AI-assisted development. Enforce quality gates at three phases: pre-code, post-code, and pre-PR. Use when: (1) starting a new feature or bug fix, (2) finishing AI-generated code before review, (3) preparing a pull request, (4) running a planning interview, (5) auditing automation readiness, (6) preventing AI slop, (7) session bootstrap, (8) source rank, (9) domain gates, (10) bugbook. Triggers: 'bazdmeg', 'pre-code checklist', 'post-code checklist', 'pre-PR checklist', 'planning interview', 'quality gates', 'session bootstrap', 'source rank', 'domain gates', 'bugbook'."
---

# The BAZDMEG Method

Eight principles for AI-assisted development. Born from pain. Tested in production.

## Quick Reference

| # | Principle | One-Liner | Deep Dive |
|---|-----------|-----------|-----------|
| 1 | Requirements Are The Product | The code is just the output | [references/01-requirements.md](references/01-requirements.md) |
| 2 | Discipline Before Automation | You cannot automate chaos | [references/02-discipline.md](references/02-discipline.md) |
| 3 | Context Is Architecture | What the model knows when you ask | [references/03-context.md](references/03-context.md) |
| 4 | Test The Lies | Unit tests, E2E tests, agent-based tests | [references/04-testing.md](references/04-testing.md) |
| 5 | Orchestrate, Do Not Operate | Coordinate agents, not keystrokes | [references/05-orchestration.md](references/05-orchestration.md) |
| 6 | Trust Is Earned In PRs | Not in promises, not in demos | [references/06-trust.md](references/06-trust.md) |
| 7 | Own What You Ship | If you cannot explain it at 3am, do not ship it | [references/07-ownership.md](references/07-ownership.md) |
| 8 | Sources Have Rank | Canonical spec > audit > chat | [references/08-sources-have-rank.md](references/08-sources-have-rank.md) |

## Effort Split

| Activity | Time | Why |
|----------|------|-----|
| Planning | 30% | Understanding the problem, planning interview, verifying understanding |
| Testing | 50% | Writing tests, running agent-based tests, verifying everything works |
| Quality | 20% | Edge cases, maintainability, polish |
| Coding | ~0% | AI writes the code; you make sure the code is right |

---

## Workflow: Planning Interview

Run this interview BEFORE any code is written. The agent asks the developer these questions and does not proceed until all are answered.

1. **What problem are we solving?** -- State the problem in your own words, not the ticket's words.
2. **What data already exists?** -- What is the server-side source of truth? What APIs exist? What state is already managed?
3. **What is the user flow?** -- Walk through every step the user takes, including edge cases and error states.
4. **What should NOT change?** -- Identify existing behavior, contracts, or interfaces that must be preserved.
5. **What happens on failure?** -- Network errors, invalid input, race conditions, missing data.
6. **How will we verify it works?** -- Name the specific tests: unit, E2E, agent-based. What constitutes "done"?
7. **Can I explain this to a teammate?** -- If you cannot explain the approach to someone else, stop and learn more.

**Stopping rules:**
- If any answer is "I don't know" -- stop and research before proceeding.
- If the developer defers to "the AI will figure it out" -- stop. The requirement IS the product.
- If no test plan exists -- stop. Untested code is unshippable code.

---

## Checkpoint 0: Session Bootstrap

Run BEFORE anything else in a new session.

- [ ] Read project status doc (STATUS_WALKTHROUGH, README, or equivalent)
- [ ] Check task list / mailbox for pending work
- [ ] Confirm current branch, latest commit, CI status
- [ ] Identify what changed since last session (git log, diff)
- [ ] Read agent-specific notes file (if multi-agent)
- [ ] Execute the "NOW" section — do not ask questions first

**If you cannot confirm the current state, stop. You are operating on stale context.**

---

## Checkpoint 1: Pre-Code Checklist

Run this BEFORE the AI writes any code.

- [ ] Can I explain the problem in my own words?
- [ ] Has the AI interviewed me about the requirements?
- [ ] Do I understand why the current code exists?
- [ ] Have I checked my documentation for relevant context?
- [ ] Is my CLAUDE.md current?
- [ ] Are my tests green and non-flaky?
- [ ] Is CI running in under 10 minutes?

**If any box is unchecked, do not proceed to implementation.**

---

## Checkpoint 2: Post-Code Checklist

Run this AFTER the AI writes code, BEFORE creating a PR.

- [ ] Can I explain every line to a teammate?
- [ ] Have I verified the AI's assumptions against the architecture?
- [ ] Do I know why the AI chose this approach over alternatives?
- [ ] Have the agents tested it like a human would?
- [ ] Do MCP tool tests cover the business logic at 100%?

**If any box is unchecked, go back and understand before proceeding.**

---

## Checkpoint 3: Pre-PR Checklist

Run this BEFORE submitting the pull request.

- [ ] Do my unit tests prove the code works?
- [ ] Do my E2E tests prove the feature works?
- [ ] Does TypeScript pass with no errors in strict mode?
- [ ] Can I answer "why" for every decision in the diff?
- [ ] Would I be comfortable debugging this at 3am?
- [ ] Does the PR description explain the thinking, not just the change?

**If any answer is "no," stop. Go back. Learn more.**

---

## Automation-Ready Audit

Before adding AI agents to a workflow, verify these six gates pass.

| Gate | Requirement | Current (Feb 2026) | Why |
|------|-------------|-------------------|-----|
| CI Speed | Under 10 min (under 10s = branchless) | ~3 min tests, build OOM intermittent | Fast CI = fast agent iterations. If CI completes in under 10 seconds, skip branches entirely — commit to main (trunk-based dev). |
| Flaky Tests | Zero | Zero known | Flaky tests gaslight the AI into chasing phantom bugs |
| Coverage | 100% on business logic | 80% lines (CI-enforced), 96% MCP file coverage (94/98) | Untested code is invisible to agents; they will refactor through it |
| TypeScript | Strict mode enabled | Strict, zero `any`, zero `eslint-disable` | Claude Code integrates with the TS Language Server; strict mode = level zero |
| CLAUDE.md | Current and complete | Updated Feb 16, 2026 (7 pkgs, 98 tools, ~170 routes) | Stops the AI from guessing; it follows the playbook instead |
| Domain Gates | Project-specific executable quality gates exist | (project-specific) | Generic checklists miss domain invariants; executable gates catch what generic gates cannot |

See [references/02-discipline.md](references/02-discipline.md) for the full breakdown.

---

## The 10-Second Rule: Trunk-Based Development

If your CI pipeline (lint + typecheck + tests on changed files) consistently completes in under 10 seconds:

**Skip feature branches.** Commit directly to main. The feedback loop is fast enough that broken commits are caught and fixed immediately.

This is **trunk-based development** — the same pattern used by Google, Meta, and other high-velocity engineering orgs. Prerequisites:
- Fast, reliable CI (under 10 seconds for affected tests)
- Zero flaky tests
- `vitest --changed COMMIT_HASH` to only run affected tests
- `file_guard` MCP tool to pre-validate changes

**When to still use branches:**
- CI takes more than 10 seconds
- Multiple agents work on the same codebase simultaneously
- Regulatory/compliance requirements mandate review

**The math:** 50 commits/day at 5s CI each = 4 minutes waiting. Branching overhead at 5 min/change = 250 minutes of ceremony. The choice is clear.

---

## Hourglass Testing Model

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

| Layer | Share | What to test |
|-------|-------|-------------|
| MCP tool tests | 70% | Business logic, validation, contracts, state transitions |
| E2E specs | 20% | Full user flows (Given/When/Then), wiring verification only |
| UI component tests | 10% | Accessibility, responsive layout, keyboard navigation |

See [references/04-testing.md](references/04-testing.md) for the Three Lies Framework and test type decision guide.

---

## Bayesian Bugbook

**Bug appears twice → mandatory Bugbook entry.** Bugs earn their record through recurrence and lose it through irrelevance.

| Event | Confidence | Status |
|-------|-----------|--------|
| First observed | 0.5 | CANDIDATE — log conditions |
| Second occurrence | 0.6+ | ACTIVE — full entry + regression test |
| Fix prevents recurrence | +0.1 per prevention | ACTIVE — confidence grows |
| Irrelevant 5+ sessions | -0.1 decay | Decaying → DEPRECATED below 0.3 |

Every ACTIVE entry requires a regression test matched to scope: unit test for single-function bugs, E2E test for cross-component bugs, agent-based test for usability bugs.

See [references/04-testing.md](references/04-testing.md) for the full Bugbook entry format and Three Lies integration.
