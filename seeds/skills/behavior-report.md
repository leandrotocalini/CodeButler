# behavior-report

Analyze agent behavior across threads to find patterns for improving CodeButler.

## Trigger
behavior report, analyze behavior, how are agents doing, agent performance, what should we improve

## Agent
lead

## Prompt
Read all thread report files in `.codebutler/reports/` and produce an aggregate behavior analysis.

1. **Load reports** — read all `.json` files in `.codebutler/reports/`. If no reports exist, say so and stop
2. **Compute aggregate metrics** per agent across all threads:
   - Average and max `turns_used / max_turns` ratio — are budgets right?
   - Total `loops_detected` and which agents loop most — is stuck detection working?
   - Average `reasoning_messages` per agent — are they following reasoning-in-thread?
   - `plan_deviations` frequency — are PM plans accurate?
   - Average `review_rounds` — is Coder output quality improving?
   - `issues_found` by type across all threads — what does Reviewer catch most?
   - Cost distribution by agent and by workflow type
3. **Detect recurring patterns** — group `patterns` entries by `type` across reports. If the same pattern type appears in 3+ threads, it's systemic
4. **Identify trends** — compare recent reports (last 5) to older ones. Are metrics improving or degrading?
5. **Post a structured report** in the thread:
   - **Summary** — one paragraph: overall agent health, biggest issue, biggest improvement
   - **Budget efficiency** — turns used vs max, per agent. Flag agents that consistently hit >80% or use <20%
   - **Reasoning compliance** — average reasoning messages per agent. Flag agents that consistently post 0
   - **Recurring patterns** — systemic issues that repeat across threads, with count and examples
   - **Cost breakdown** — average cost per workflow type, cost trends over time
   - **Recommendations** — concrete, prioritized changes to CodeButler (seed updates, code changes, config tweaks). Each recommendation should cite the reports that support it
6. **Be honest, not generous.** If agents are underperforming, say so with data. This report drives real code changes
