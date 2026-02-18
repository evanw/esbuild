# Principle 7: Own What You Ship

**Core idea:** If you cannot explain it at 3am, do not ship it.

---

## The Responsibility Framework

Four questions to ask before shipping any code:

1. **Can you explain every line of your PR to a teammate?**
   - If not, you do not understand the code well enough.

2. **Can you answer "why" for every decision?**
   - Not "Claude suggested it." Why is this the right approach?

3. **Can you debug this at 3am when it breaks in production?**
   - If it breaks, you answer the phone. Not the AI.

4. **Can you own the consequences?**
   - When the system fails, when the bug causes harm, when the decision was wrong, someone must own that. AI cannot be responsible. Only we can.

**If the answer to any of these is no, you are not ready to ship.**

---

## What Remains Human

### Creativity Stays Human

AI writes code that looks like code it has seen before. True creativity -- the kind that sees a problem nobody else sees -- stays with us.

### Judgment Stays Human

AI can give you ten solutions. It cannot tell you which one is right for your situation.

### Empathy Stays Human

AI cannot watch a user struggle with your interface and feel what they feel.

### Responsibility Stays Human

When something goes wrong in production at 3am, AI does not answer the phone. You do.

---

## Sustainable Work

For developers with ADHD or similar challenges, AI offers unique advantages:

- When you lose focus and come back hours later, the AI remembers the context
- It picks up where you left off
- It does not judge. It does not ask "where were we?"

But sustainability requires structure:
- Consistent daily routines
- Regular breaks
- Boundaries between work and rest

AI is a force multiplier for structured work. Without structure, it multiplies chaos.

---

## Actionable Takeaways

1. Apply the 4-question responsibility framework before every PR.
2. If you cannot explain the code, do not ship it. Go back and learn.
3. Remember what stays human: creativity, judgment, empathy, responsibility.
4. Use AI as a force multiplier, not a replacement for understanding.
5. Build sustainable work habits. Structure enables AI-assisted productivity.
6. You code because you choose to. AI does it differently. You do it anyway.

---

---

## Spike Land in Practice

**ADHD-Safe Protocol implements sustainable work.** The 5 hard rules in CLAUDE.md (never touch git directly, one branch/one agent/one conversation, pipeline is the boss, no YOLO merges, context switching protocol) are specifically designed for sustainable AI-assisted development. When focus drifts, "save state" commits WIP. When returning, "where was I?" checks open PRs. Structure enables productivity without demanding perfect attention.

**98 typed MCP tools = ownable capabilities.** Each MCP tool has a typed Zod schema for inputs, a typed response structure, and a test file. This means the developer can apply the 4-question framework to any tool: Can I explain every line? (yes, the schema documents it). Can I debug at 3am? (yes, the test reproduces any issue). Can I own the consequences? (yes, the typed contract limits blast radius). Typed tools are inherently more ownable than untyped sprawl.

**The chess engine exemplifies "own what you ship."** The ELO calculation in `src/lib/chess/elo.ts` is pure math — no AI magic, no hidden complexity. A developer can explain the K-factor formula, debug a rating discrepancy, and own the consequences of a miscalculation. The AI wrote the code, but the mathematics are auditable by any engineer. This is what ownership looks like in the AI era.

---

*Sources: Blog posts 10 (What Do Developers Become), 12 (Earning Less Than Five Years Ago), 13 (Brighton, Dogs, and ADHD), 14 (Why I Still Code When AI Does It Better), 15 (Letter to a Junior Developer in 2026)*
