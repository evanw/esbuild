# Principle 8: Sources Have Rank

**Core idea:** When sources conflict, the hierarchy decides. Not the loudest voice. Not the most recent message. The highest-ranked source wins.

---

## Authority Hierarchy

| Rank | Source | Examples | Wins Over |
|------|--------|----------|-----------|
| 1 | Canonical spec | CLAUDE.md, design docs, API contracts, Zod schemas | Everything |
| 2 | Quality audits / gate results | CI output, test results, gate checklists, audit notes | Rank 3-4 |
| 3 | Agent notes / persistent memory | MEMORY.md, agent-specific notes, learning files | Rank 4 |
| 4 | Chat / conversation | Slack, PR comments, verbal instructions, ad-hoc messages | Nothing |

**The rule is simple:** higher rank always wins. If chat says "skip the test" but the canonical spec requires it, the spec wins. If an agent's memory says "use approach X" but a quality audit shows X fails, the audit wins.

---

## DOC-ANSWER Gate

Before any plan or decision, the agent must pass this gate:

1. **Cite sources read.** List the file paths you consulted. If you did not read anything, you are guessing.
2. **Describe search performed.** What did you grep for? Which directories did you explore? How did you confirm you found the right source?
3. **Only escalate if unresolved.** If the docs answer the question, use the docs. Do not ask the human to repeat what is already written.

**Format:**
```
Sources consulted:
- docs/CURRENT_SYSTEM_SPEC.md (lines 42-67)
- src/lib/schemas/user.ts (Zod schema)

Search performed:
- Grepped for "user validation" across src/
- Read CLAUDE.md section on data contracts

Decision: Use Zod schema as authoritative contract (Rank 1).
No escalation needed.
```

**If the agent cannot fill this format, it has not done its homework.**

---

## Anti-Patterns

| Anti-Pattern | What Happens | Fix |
|-------------|-------------|-----|
| Chat overrides documented decisions | A casual message in Slack contradicts the spec. Agent follows chat. | Update the spec first. Then follow the spec. |
| Stale memory contradicts updated specs | Agent memory says "use REST" but spec was updated to GraphQL. Agent follows memory. | Memory (Rank 3) loses to spec (Rank 1). Re-read the spec. |
| Two agents follow different sources | Agent A reads the spec. Agent B reads an old chat thread. They produce conflicting work. | All agents must cite sources. Conflicts are resolved by rank. |
| Asking questions already answered in docs | Agent asks human "what is the API format?" when it is documented in CLAUDE.md. | DOC-ANSWER Gate: search first, ask only if unresolved. |

---

## Actionable Takeaways

1. Define the authority hierarchy in your CLAUDE.md. Make it explicit and unambiguous.
2. Higher rank always wins. No exceptions, no "but the PM said."
3. Require source citations in plans and decisions. No citation = no credibility.
4. Update the spec before overriding it via chat. Chat is ephemeral. Specs persist.
5. When agents disagree, compare source ranks. The agent citing the higher-ranked source wins.

---

---

## Spike Land in Practice

**CLAUDE.md is Rank 1.** The 170+ route inventory, 98 MCP tool definitions, and Zod schemas in `src/lib/schemas/` are the canonical source of truth. When an agent needs to know the API contract for a user operation, it reads the Zod schema — not a chat message, not a memory file. The schema is the spec. Period.

**Zod schemas as authoritative contracts.** Every MCP tool in Spike Land has a typed Zod input schema and a typed response structure. These schemas are Rank 1 sources: they define what the tool accepts and returns. An agent that generates code conflicting with a Zod schema is wrong by definition, regardless of what chat or memory says.

**DOC-ANSWER Gate in practice.** Before modifying any MCP tool, the agent reads the tool's test file, its Zod schema, and the relevant section of CLAUDE.md. This triple-read pattern ensures the agent operates from Rank 1 sources, not from stale context or guesswork.

---

*Sources: Asterobia PLANNING_PROTOCOL.md, Asterobia CANONICAL_SOURCES_INDEX.md, Blog posts 03 (Context Engineering Replaced Coding), 08 (How to Not Produce AI Slop)*
