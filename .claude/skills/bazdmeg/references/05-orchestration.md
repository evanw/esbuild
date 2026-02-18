# Principle 5: Orchestrate, Do Not Operate

**Core idea:** You do not have a copilot anymore. You have a whole dev team. Coordinate agents, not keystrokes.

---

## The Production Pipeline

| Phase | Who Does It | Why |
|-------|------------|-----|
| **Planning** | Claude Code (multiple agents) | Explores codebase, interviews you, considers edge cases |
| **Implementation** | Jules | Follows the plan exactly, adds the tests the plan specifies |
| **CI/CD** | Your pipeline | Fast feedback, sharded tests, cached builds |
| **Code Review** | Claude Code (Opus) | Strict. Consistently catches real issues |
| **Fixes** | Jules | Iterates until CI and review both pass |
| **Merge** | Automated | When all checks are green |

**Your job:** Define what you want. Verify it works. That is it.

---

## BMAD: Agent Personas with Context Isolation

BMAD -- Breakthrough Method of Agile AI-Driven Development -- defines six agent personas:
1. **PM** -- Requirements and priorities
2. **Architect** -- Technical design and constraints
3. **Developer** -- Implementation
4. **QA** -- Testing and verification
5. **Scrum Master** -- Process and coordination
6. **Product Owner** -- User value and acceptance

Each persona gets a fresh context window with only the artifacts it needs. No accumulated junk. No 50K-token mega-prompts.

### Epic Sharding

Instead of stuffing an entire feature into one agent context, break it into vertical slices. Each slice gets its own ticket, its own context, its own agent.

**Token reduction: 74-90% fewer tokens** per agent context compared to monolithic prompts.

---

## Darwinian Fix Loops

The agent loop is natural selection for code:

1. **Generate** (mutation) -- AI produces code
2. **Transpile** (environmental test) -- Does it compile?
3. **Fix** (adaptation) -- Address errors
4. **Learn** (heritable memory) -- Extract and store lessons

Up to 3 iterations per request.

### Bayesian Memory System

The memory system prevents errors from recurring across all future generations:

1. Every error gets extracted as a learning note by Haiku
2. Each note starts as a **CANDIDATE** with 0.5 confidence
3. Notes that help get promoted: **ACTIVE** at >0.6 confidence after 3+ helps
4. Notes that fail get deprecated: **DEPRECATED** below 0.3 after 5+ observations

**Results:**
- First-try success rate: ~40% -> ~65%
- Success after retries: ~55% -> ~85%

Natural selection, running on softmax.

---

## Multi-Agent Role Separation

When multiple AI agents collaborate on the same codebase, ambiguity kills. Each agent needs a defined role with clear boundaries.

### Three-Role Pattern

| Role | Defines | Cannot | Authority |
|------|---------|--------|-----------|
| Spec Guardian | WHAT to build (requirements, contracts, acceptance criteria) | Write production code | Defines the target. Overrides implementer on "what." |
| Quality Gate | IF it ships (tests pass, gates clear, coverage met) | Do deep refactors or change requirements | Defines the bar. Overrides implementer on "whether." |
| Implementer | HOW to build it (architecture, code, patterns) | Change specs or lower quality bar | Owns the code. Free to choose approach within constraints. |

### Authority Chain

Decisions flow: **Spec Guardian → Quality Gate → Implementer.**

- Spec Guardian decides what to build. Quality Gate and Implementer follow.
- Quality Gate decides if the implementation meets the bar. Implementer iterates until it does.
- Implementer decides the approach. Free within the constraints set above.
- Disagreements escalate to the human. Always. No agent overrides another agent's domain.

This follows the source ranking from Principle 8: specs (Rank 1) beat audit results (Rank 2) beat agent notes (Rank 3) beat chat (Rank 4).

### Two-Instance Conflict Prevention

When two agents work concurrently:

1. **File partitioning.** Agent A owns `src/**`. Agent B owns `docs/**`. Boundaries are explicit and non-overlapping.
2. **Never edit the same file concurrently.** If both agents need to modify `config.ts`, one waits. Lock contention is cheaper than merge conflicts.
3. **Declare ownership in commits.** Co-Author lines identify which agent made which change. Blame stays traceable.

### Mailbox Protocol

Structured handoff between agents:

```
REQUEST #042
From: Quality Gate
To: Implementer
Priority: P1
What needed: 3 unit tests fail after refactor — fix regressions
Where to reply: PR #187 comments
```

- Mailbox is for status and requests only. Not for discussion.
- Priority levels: **P0** (blocks everything, immediate), **P1** (blocks release, within session), **P2** (improvement, next session).
- If no reply within the session, escalate to human.

---

## Actionable Takeaways

1. Stop operating (typing code). Start orchestrating (defining and verifying).
2. Use the production pipeline: plan -> implement -> test -> review -> fix -> merge.
3. Give each agent persona its own context. Avoid mega-prompts.
4. Shard epics into vertical slices. Each slice = one ticket, one agent.
5. Let fix loops iterate. Up to 3 tries before escalating.
6. Build a learning memory system so errors do not recur.
7. Define explicit roles (Spec Guardian, Quality Gate, Implementer) when using multiple agents.
8. Partition files between concurrent agents. Never edit the same file simultaneously.

---

---

## Spike Land in Practice

**AI Orchestrator app's 8 MCP tools match BMAD personas.** The AI Orchestrator store app (`src/lib/mcp/server/tools/orchestrator.ts` + `swarm.ts`) implements spawn_agent, list_agents, stop_agent, broadcast, redirect, pack_context, run_sandbox, and decompose_task. These map directly to BMAD's persona model — each spawned agent gets isolated context with only the artifacts it needs, preventing the 50K-token mega-prompt problem.

**QA Studio enables agent-based testing.** The QA Studio app's 10 MCP tools (`browser_navigate`, `browser_screenshot`, `browser_click`, `browser_type`, `accessibility_audit`, `run_tests`, etc.) are the infrastructure for "Human Lies" testing from the Three Lies Framework. An AI agent can navigate to a page, interact with it, screenshot results, and run accessibility audits — all without human intervention.

**Epic sharding in practice.** When building the 23 store apps, each app was a vertical slice: own data definition in `store-apps.ts`, own MCP tools in `src/lib/mcp/server/tools/`, own tests, own UI route. No single agent needed context about all 23 apps simultaneously. Token reduction: each app slice was ~200 lines of context vs. ~5,000 for the full store.

**Asterobia's three-AI model implements the Three-Role Pattern.** In the Asterobia game project, Claude Code acts as the Implementer (writes code per canonical specs), Antigravity (Gemini) acts as the Quality Gate (repo operator, quality gatekeeper, doc maintainer), and ChatGPT acts as the Spec Guardian (spec guardian, prompt writer, read-only on GitHub). Each agent has explicit boundaries: the Spec Guardian cannot write code, the Quality Gate cannot do deep refactors, and the Implementer cannot change specs. Disagreements escalate to the human owner. This pattern prevents the "two agents editing the same file" disaster and enforces source ranking (Principle 8) naturally.

---

*Sources: Blog posts 04 (2025: The Year Agents Outperformed), 07 (Context Engineering Replaced Coding), The Vibe Coding Paradox, How to Automate Your Dev Team with AI Agents, Asterobia CLAUDE.md (Multi-Agent Workflow), Asterobia PLANNING_PROTOCOL.md*
