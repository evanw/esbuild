# Principle 6: Trust Is Earned In PRs

**Core idea:** Not in promises, not in demos. Trust is earned one good PR at a time.

---

## The Trust Gap

A developer's PR sat untouched for three days. The code was good. The tests passed. The feature worked exactly as requested.

Then they heard the conversation: "That one from Zoltan? I will just rewrite it myself. Faster than reviewing all that AI stuff."

Everything from that developer took 3-4x longer to review. Not because the code was bad. Because the team did not trust AI-generated code. They looked at PRs differently. They searched for problems that might not exist. They questioned decisions that made perfect sense.

**The trust gap is real. And AI slop makes it worse.**

---

## The 5-Step Trust Rebuilding Framework

### Step 1: Stop Hiding

Stop trying to make AI-assisted code look like you wrote it all yourself. People find out anyway, and it makes the trust problem worse.

Be open about your tools. Not defensive. Not apologetic. Just honest.

### Step 2: Show Your Work

Not just the final code. Show the thinking behind it. The problems you solved. The decisions you made.

When people see that you understand what you submitted, they trust it more.

### Step 3: Ask Differently

Instead of "please review my PR," try "I would love your thoughts on this approach."

This changes the conversation from judgment to collaboration.

### Step 4: Help Others Learn

Some colleagues are curious about AI tools but afraid to try them. When you share what you have learned, you become partners instead of competitors.

### Step 5: Give It Time

Trust does not rebuild overnight. Every good PR, every helpful conversation, every moment of genuine collaboration adds a little bit back.

---

## The Sprint-End Review Batching Problem

Thursday morning. Eight hours until sprint review. Three PRs submitted eight days ago sit untouched. No comments. No questions. No feedback.

Now, with 36 hours left, dozens of comments. Everything is wrong. Everything must change.

**If every sprint ends in chaos, that is not bad luck. That is a broken process.** Name it. Discuss it. Fix it.

---

## PR Best Practices for AI-Assisted Development

1. **PR description explains the thinking** -- not just "what changed" but "why this approach."
2. **Small, focused PRs** -- easier to review, easier to trust, easier to merge.
3. **Tests prove the work** -- reviewers see tests pass and gain confidence.
4. **Answer "why" for every decision** -- be ready to explain any line in the diff.
5. **Link to requirements** -- every PR traces back to a documented ticket.
6. **No unexplained AI artifacts** -- if you cannot explain it, do not submit it.

---

## Actionable Takeaways

1. Acknowledge the trust gap exists. Do not pretend it does not.
2. Be transparent about AI usage. Hiding it makes things worse.
3. Show understanding, not just code. Explain the "why."
4. Submit small, well-tested PRs with clear descriptions.
5. Help teammates learn AI tools. Partners, not competitors.
6. Be patient. Trust rebuilds one good PR at a time.

---

---

## Spike Land in Practice

**CI branch protection enforces PR discipline.** The CLAUDE.md ADHD-Safe Protocol requires: 1 approval + all CI green before merge, `enforce_admins=true` so even the owner cannot bypass. Claude Code Review auto-approves good PRs and tags @Jules for fixes on bad ones. This eliminates "YOLO merges" — trust is literally enforced by infrastructure, not willpower.

**`file_guard` prevents AI slop.** The `file_guard` MCP tool pre-checks file changes against `vitest --changed` before committing. This is a trust mechanism: before any code reaches a PR, it's already been validated against the test suite. Reviewers see test-verified changes, not unchecked AI output.

**98 typed MCP tools with tests = reviewable PRs.** When a PR modifies a chess tool, the reviewer can check: (1) the typed schema matches the interface, (2) the test passes with meaningful assertions, (3) the business logic in `src/lib/chess/` is correct. Each piece is independently verifiable. This is "show your work" at scale — not just the final code, but the contracts and proofs behind it.

---

*Sources: Blog posts 02 (More Productive, Ruining Career), 03 (The Last Two Days of Every Sprint), 05 (The Trust Gap: Why Teams Reject AI Code)*
