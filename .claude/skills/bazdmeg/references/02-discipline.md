# Principle 2: Discipline Before Automation

**Core idea:** You cannot automate chaos.

---

## The Problem

If your CI takes 45 minutes, agents sit idle for 45 minutes on every iteration. That is not a productivity gain. That is paying for cloud compute to stare at a progress bar.

If your tests flake randomly, agents will chase phantom bugs. They will spend hours trying to fix something that is not broken. You are, quite literally, gaslighting your AI.

If your business logic has no test coverage, it does not exist as far as the agent is concerned. Untested features are invisible features. The agent will refactor right through them without a second thought.

---

## The Five Automation Gates

Before adding a single AI agent to your workflow, audit this list:

### Gate 1: CI Speed -- Under 10 Minutes

Every minute you shave off CI is a minute saved on every single agent iteration.
- 10-minute CI loop = agents iterate 4-6 times per hour
- 45-minute CI loop = agents iterate once per hour

**Action:** Profile your CI. Identify the slowest steps. Shard tests. Cache builds. Remove unnecessary steps.

### Gate 2: Zero Flaky Tests

Fix them or delete them. There is no middle ground.

A flaky test is worse than no test when agents are involved. It introduces false signal into the feedback loop. One flaky test can send an agent down a 30-minute rabbit hole of "fixes" to code that was perfectly fine.

**Action:** Run your test suite 10 times. Any test that fails even once is flaky. Fix it or delete it.

### Gate 3: 100% Coverage on Business Logic

Not vanity coverage. Not padding with trivial assertions. Real coverage on the code paths that matter.

Coverage is the specification that makes autonomous refactoring safe. Without it, agents have no guardrails.

**Action:** Identify your critical business logic. Ensure every code path has a meaningful test.

### Gate 4: TypeScript Strict Mode

This is level zero of the test pyramid. Claude Code integrates with the TypeScript Language Server. It sees type errors in real time.

If you are not on strict mode, that is your first task.

**Action:** Enable `strict: true` in tsconfig.json. Fix every error. Do not suppress with `any`.

### Gate 5: CLAUDE.md Is Current

Every project has a file that contains everything the AI needs to know. Team conventions. Architectural decisions. Common pitfalls.

When the AI reads this file, it stops guessing. It follows the playbook.

**Action:** Review your CLAUDE.md. Is it up to date? Does it reflect current architecture? Does it include common pitfalls?

---

## "Gaslighting Your AI"

When flaky tests produce false failures, agents interpret them as real bugs. The agent will:
1. Investigate the "failure"
2. Hypothesize a cause
3. Implement a "fix" that changes working code
4. Possibly break something else in the process

This is gaslighting -- presenting false information that causes the AI to question reality and make incorrect decisions.

---

## Actionable Takeaways

1. Fix your CI speed before adding agents. Target under 10 minutes.
2. Eliminate every flaky test. Zero tolerance.
3. Measure real coverage on business logic, not vanity metrics.
4. Enable TypeScript strict mode. No `any`.
5. Keep CLAUDE.md current. It is the AI's playbook.
6. Fix the foundation before you build the house.

---

---

## Spike Land in Practice

**98 MCP tools / 94 test files = 96% file coverage.** Every MCP tool file in `src/lib/mcp/server/tools/` follows the `createMockRegistry()` test pattern. This discipline means agents can refactor any tool with confidence — the tests catch regressions immediately. The 4 untested files are infrastructure helpers (`tool-factory`, `tool-helpers`, `bootstrap`, `capabilities`), not business logic.

**OOM build as a discipline violation.** The Next.js production build occasionally hits memory limits in CI. This is a Gate 1 violation — CI that fails unpredictably gaslights agents into thinking their code changes caused the failure. Fixing CI reliability is a prerequisite for scaling agent usage.

**TypeScript strict mode enforced project-wide.** Zero `any`, zero `eslint-disable`, zero `@ts-ignore`. This is non-negotiable in CLAUDE.md and enforced by CI. When agents generate code, the TS Language Server catches type errors in real time — strict mode is the foundation that makes all other automation safe.

---

*Sources: Blog posts 03 (The Last Two Days of Every Sprint), 08 (How to Not Produce AI Slop), 16 (How I Vibe-Coded a Production SaaS), You Cannot Automate Chaos*
