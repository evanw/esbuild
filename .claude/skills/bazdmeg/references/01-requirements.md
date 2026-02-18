# Principle 1: Requirements Are The Product

**Core idea:** The code is just the output. The requirement is the product.

---

## The Lesson

A developer joined a new team and asked Claude to help track analytics for a retention offer. Claude generated code that called the Basket API to fetch basket data. It looked perfect. It passed tests. The linter was happy.

The basket was server-side. The backend was the single source of truth. There was no need to call the Basket API at all.

The PR was essentially garbage. The developer had no answer for why the code existed -- because they did not write it. They approved it without understanding.

**When AI hallucinates confidently, it makes you hallucinate too.**

---

## The Effort Split

| Activity | Time | Why |
|----------|------|-----|
| Planning | 30% | Understanding the problem, planning interview, verifying understanding |
| Testing | 50% | Writing tests, running agent-based tests, verifying everything works |
| Quality | 20% | Edge cases, maintainability, polish |
| Coding | ~0% | AI writes the code; you make sure the code is right |

Notice what is missing. Coding. The actual typing of code takes almost no time. The value is in understanding and verification.

---

## The Planning Interview

This is the single biggest improvement. Before any code is written, the agent interviews the developer about the problem.

Key questions:
1. What is the user flow here?
2. What data already exists on the server?
3. Why does this ID appear in the URL?
4. What happens if this fails?

**Stopping rule:** If you cannot answer a question, stop. Go back to the documentation. Do not proceed until you understand.

If the agent had interviewed the developer before the basket PR, it would have asked: "What data already exists on the server?" And there would have been no answer. That would have stopped the mistake.

---

## Actionable Takeaways

1. Write requirements like you mean it -- clear acceptance criteria, edge cases, examples.
2. Have the AI interview you before any code is written.
3. If you cannot explain the problem in your own words, you are not ready for implementation.
4. If coding agents make mistakes, the requirements were not specified well enough.
5. Multiply zero understanding by a powerful AI and you still get zero.

---

---

## Spike Land in Practice

**Store's `StoreApp` interface as requirements-as-code.** The `src/app/store/data/store-apps.ts` file defines 23 apps with typed interfaces (`StoreApp`) that serve as executable requirements. Each app declares its `toolCount`, `mcpTools[]`, `features[]`, and `category` — making the requirement the literal source of truth. When an app claims 21 tools, the typed array enforces it.

**FEATURES.md drift as a warning sign.** In January 2026, FEATURES.md documented only "Pixel" and "Vibe Coding" as supporting tools — while the codebase had grown to 23 store apps with 163 MCP tools. This drift meant agents reading the docs got a fundamentally wrong picture of the platform. The requirement (docs) no longer matched the product (code), violating Principle 1 directly.

**The planning interview catches architecture mistakes.** When building the Chess Arena (21 MCP tools), question 2 ("What data already exists?") revealed that the chess engine, ELO calculations, and game state all needed to live in `src/lib/chess/` as pure business logic — not embedded in MCP tool handlers. This separation produced 6 clean library files that any tool layer can consume.

---

*Sources: Blog posts 01 (My PRs Were Pure AI Slop), 08 (How to Not Produce AI Slop), 11 (The Requirement Is the Product), 16 (How I Vibe-Coded a Production SaaS)*
