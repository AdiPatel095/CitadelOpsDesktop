# Onboarding evaluation protocol (CIT-22)

A local, moderated protocol for a future trial. It states agreed pass criteria, not predictions: **no gain is claimed until a trial has been run and reported.** There is no telemetry, no new production instrumentation, and no recruitment or outreach beyond people already available to the studio. The criteria below were agreed with Maya (Product/Simpler automation setup.md, "CIT-22 product answers", answer 2) and are not adjusted after a trial has started.

## Conditions

- **A, baseline:** the experience before CIT-15. Desktop `develop` at `97d17481`, frontend `develop` at `62ac12c6`.
- **B, candidate:** the CIT-22 candidate (the exact revision under test is written on the observer sheet).
- Between subjects: each participant runs exactly one condition, so nobody carries learning from one to the other.

## Participants

- At least 5 per condition (10 in total). Each condition has at least 2 people who are new to CitadelOps and at least 2 existing users with custom setups.
- With fewer than 5 per condition the results are observations and can never be reported as a pass.

## Tasks (exact wording, 10 minutes each, then mark incomplete)

| Id | Task | Fixture scenario (condition B) |
|---|---|---|
| T1 | "Set up Auto Nomad from nothing and get it ready to start." | `new-user` |
| T2 | "Make Auto Khan re-apply your main-castle defense." | `khan-defense-fresh` |
| T3 | "Use the same Auto Station troop reserve on Ashford Keep and Marrow Court as on Stonehaven." | `copy-compatible` |
| T4 | "Return to the Auto Towers setup you were interrupted in and finish it." | `onboarding-interrupted-return` |
| T5 | "Stop a running automation and say what happens to an attack already on its way." | `phases-running` |

Condition A is run on the matching pre-CIT-15 build with equivalent starting data; the fixture scenarios above are for condition B (see `OnboardingPreview.md`).

## Measures per task (observer sheet)

- Unassisted completion: yes or no.
- Time from task start to the Save or Start confirmation.
- Help requests: a question the moderator must answer for the participant to proceed.
- Preset-view detours: visits to Attack or Defense Presets that the task did not require.
- Errors: a blocked Save, the wrong castle, an unintended spending or consumable permission change, or a Start attempted with a blocked check.
- Comprehension answers (below).

## Comprehension questions (asked after the task, exact wording)

- **C1, cost:** "Before you press Start, what will this automation spend or use up, if anything?" Pass: names every spend or consumable shown in the plan and summary lines for that task, and invents none.
- **C2, Start:** "What happens when you press Save? And when you press Start?" Pass: Save keeps settings and starts nothing; Start turns the automation on and it acts at its next check.
- **C3, Stop:** "You press Stop while an attack is on its way. What happens to it?" Pass: the automation turns off; an attack the game already accepted is not recalled.
- **C4, after T1:** "You renamed the preset the app created. Whose preset is it now?" Pass: a normal preset of mine; the automation still uses it.

## Pass thresholds for condition B

- Unassisted completion by at least 4 of 5 participants on every task.
- Median help requests at most 1 per task.
- Zero preset-view detours on T1 to T3.
- **Zero** unintended spending or consumable permission changes across all tasks (a hard requirement).
- C1, C2 and C3 each passed by at least 4 of 5 participants.
- C4 is reported; it has no threshold.

Condition A is reported descriptively beside B. Any difference is stated as "in this sample" only. Below threshold, the findings go to a follow-up issue (through Ellis); no threshold is changed afterwards.

## FREE badge observation

At the commander panel, ask: "Does FREE on a commander that is switched off read as 'will be used'?" Record yes or no for the CEO and for every participant who reaches the panel. If any answer is yes: follow-up issue to show the activity badge only for allowed commanders and a muted "Off for this automation" otherwise. If all are no: leave it and keep the record.

## Observer sheet

| Field | Value |
|---|---|
| Participant id (no names) | |
| Condition (A or B) and revision | |
| New to CitadelOps / existing user with custom setup | |
| Task (T1-T5) | |
| Unassisted completion (yes/no) | |
| Time to Save or Start confirmation | |
| Help requests (count, and what was asked) | |
| Preset-view detours | |
| Errors (which) | |
| C1 / C2 / C3 / C4 answer and pass (yes/no) | |
| FREE answer (yes/no/not reached) | |
| Notes | |

## Results template (fill after a trial; do not pre-fill)

| Task | Completed unassisted (B) | Median help requests (B) | Detours (B) | Unintended spend/consumable changes (B) | C1 pass | C2 pass | C3 pass | A, for reference |
|---|---|---|---|---|---|---|---|---|
| T1 | | | | | | | | |
| T2 | | | | | | | | |
| T3 | | | | | | | | |
| T4 | | | | | | | | |
| T5 | | | | | | | | |

C4 pass count: __ of __. Sample size per condition: __ / __. Verdict: below the sample minimum (observations only) / meets / does not meet the thresholds. Any statement is "in this sample".
