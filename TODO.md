# TODO

Open items found while repairing the backend and deploying both services to Railway on 2026-07-30.
Ordered by how much each one threatens the study rather than by effort.

These are for whoever owns the study to triage and decide on. Several are research calls rather than
code changes, and are flagged as such instead of being guessed at. The engineering findings were all
checked against the running backend rather than read off the source.

Deployment details, URLs and credentials are deliberately kept out of this file. Ask Simo for those.

## Already fixed, for context

- [x] Repaired the Go backend, which did not compile (five errors in `submitHandler`).
- [x] Aligned `SessionState` with the five states the frontend actually drives. A session reaching
      `STATE_INTERACTION` previously fell through and came back typed as a survey, so the chat never
      loaded from the server.
- [x] `/api/stage` and `/api/submit` now return `allScripts` with all three chat stages, which is what
      the frontend reads.
- [x] Generated and committed `src/go/go.sum`, which was missing, so builds are reproducible.
- [x] Participants now choose a chat name at onboarding. Bots previously addressed people by their
      24-character Prolific ID.
- [x] Moved MUI, Emotion and TypeScript into `dependencies`, since the build cannot compile without
      them and a production install would skip them.
- [x] Added `railway.json` so the frontend is served as a static bundle rather than by the CRA dev
      server.
- [x] The Prolific ID is filled in automatically from the `PROLIFIC_PID` url parameter and shown
      read-only, so participants cannot mistype it. `STUDY_ID` and `SESSION_ID` are captured too, so
      submissions can be reconciled against Prolific's own export.
- [x] Added a researcher test mode at `?test=true`. It pre-fills every field, shows a warning banner,
      adds a "Skip chat" button, and flags the stored session with `test_mode: true`.

## Test mode

Open `/admin` and use the "Walk the study" tab for shortlinks into the flow, one per condition, plus
a live backend health check. That last part matters because the participant flow falls back to local
state on any backend failure and still reaches a completion code, so a run that looks fine in the
browser proves nothing about whether anything was saved.

The tab replaced the old `/test` page, which served the condition list to anyone who found the URL.
The condition labels describe the manipulation, so they now sit behind the admin secret.

The underlying urls, if you prefer to type them:

```
/admin                                   researcher page, creates no session, asks for the secret
/?test=true                              test mode, condition assigned at random
/?test=true&condition=2-1                test mode, forced condition
/?test=true&start=post                   open one screen directly, skipping everything before it
/?PROLIFIC_PID=<id>&STUDY_ID=<id>&SESSION_ID=<id>
```

`start` takes `onboarding`, `pre`, `chat`, `post` or `complete`, and is resolved by the backend so
the screen is the real one with the live completion code and the real scripts. Like `condition`, it
is honoured only alongside `test=true` and an unknown value returns 400. The `/admin` page links to
all of them under "Walk the study".

`condition` is honoured only alongside `test=true`, and an unknown key returns 400 rather than
falling back to a random cell, so a typo cannot leave you believing you walked a condition you never
saw. Test sessions bypass the allocator entirely, so they neither consume a participant slot nor get
turned away once recruitment is full. The `test_mode` flag is now written when the session row is
created rather than at the onboarding submit, so a walkthrough abandoned partway through is still
excluded from the tally.

Every preset uses the lowest value on its scale, so test rows are recognisable in the data even if
the flag is ever missed. Filter on `pre_survey_data.test_mode` when analysing.

- [ ] **`?test=true` is not access-controlled.** Any participant who discovers it can skip the chat
      and reach a valid completion code in seconds. Those sessions are flagged `test_mode: true`, so
      they can be spotted and rejected on Prolific, but that is a manual process control rather than
      a technical one. Decide whether that is acceptable before recruiting, since a build-time secret
      would sit in the JavaScript bundle and not actually be secret.

## Blocking before recruitment

- [x] **Cleared the development and test rows from the production database**, done by Simo on
      2026-08-03 through the new page. The database now holds only rows created while testing that
      day. Sessions can be cleared either by scope from "Export and delete" or by ticking individual
      rows in the session table, which is the everyday way to do it while still developing.
- [x] **Built the `/admin` route.** One authenticated page owns inspection, per-row and filtered
      delete, the data export, recruitment progress, and mirror health, and it absorbed `/test` so
      the condition list is no longer served to anyone who finds the URL. Spec in `docs/specs/`
      (local only, ask Simo).

      The security property it holds to. The frontend is a static bundle, so `/admin` is publicly
      reachable whatever the React code does. Every `/api/admin/*` endpoint therefore authenticates
      on its own request against `EXPORT_SECRET`, in constant time, denying everything when the
      variable is unset. Loading the page without the secret fires no API call at all, so no
      participant data reaches the browser. Verified with `curl` per endpoint and in a browser.
- [x] **Enabled the comfort measure, and extended it.** `ENABLE_ASSESSMENTS` is gone. The check-in now
      runs after every stage rather than after the first two, so the Late stage has a reading, and a
      condition-blind baseline (`Self_Comfort_Pre`) was added at the end of the pre-survey, before the
      participant has met the bots. All five points use the same 1 to 7 scale as the post-survey
      `Self_Comfort` item, so the trajectory can be read as one series. The flag was removed rather
      than set to `true`, because toggling it mid-recruitment would leave two datasets in one table.

      Note for analysis: the baseline asks what people *expect* to feel and the rest ask what they
      *do* feel, so the first interval is partly a shift from anticipation to experience.
- [ ] **Set `PROLIFIC_COMPLETION_CODE`.** It is still the placeholder `DEMO_CODE`, so participants
      cannot be credited.
- [x] **Fixed: the condition quota counted every session row rather than completions.** A row is
      written by `/api/session/init` the moment the page loads, so abandoned page views and
      researcher test runs permanently consumed participant slots. The live database showed 10 rows
      against 2 completions, which would have returned "Condition allocation is full" long before
      180 people finished and locked out real participants mid-recruitment.

      The allocator now separates the two questions it has to answer. Recruitment stops on
      **completed** sessions per condition, so abandonment never reduces the achievable sample.
      Assignment balances on **started** sessions, so people who are mid-study still push new
      arrivals toward the emptier cells. Rows flagged `test_mode`, and rows still sitting at
      onboarding, are excluded from both counts. Covered by tests in `main_test.go`.
- [ ] **Decide whether the repository should be private.** `src/data.json` contains every bot turn.
      A participant who searches a distinctive phrase mid-study can read the whole script in advance.
      Making the repo private costs nothing and removes the problem. This is the owner's call, and it
      also governs how much study detail belongs in this file.

## Scoring

Server-side reverse-scoring was stripped in `b3fe1dd`, so **every response is now stored raw**. That
resolves the earlier problem where BFNE was reversed, AIAS silently was not, and the stored value gave
no way to tell which convention applied to a given row.

The consequence is that all reversal now belongs in the analysis script, and none of it is done for
you:

- [ ] BFNE items 2, 4, 7 and 10 need reversing.
- [ ] AIAS is the Risks factor of the 13-item scale, renumbered `AIAS_1..4` in the survey. The design
      states all four are negatively worded and must be reversed.
- [ ] DDI items 2, 4, 5, 8, 9 and 12 are reverse-worded. The design lists the items but never states
      the reverse set, so record the decision somewhere before analysis.

Rows collected before `b3fe1dd` had BFNE reversed at write time and AIAS untouched. There are only
test rows in that group so far, but do not mix them with real data.

## Mismatches between the design document and the code

- [ ] Third bot is named `Taylor` in the design's Stage 1 table and `Charlie` everywhere else. The code
      uses `Charlie` throughout. Fix whichever is wrong.
- [x] The second open-ended item ("Did the responses from the other chat members influence what you
      chose to share? If so, how?") is now implemented as an optional field, `reflection_influence`.

      The first item was also reworded. It previously read "What specifically about the chat
      environment made you feel more (or less) comfortable sharing your experiences?", which
      presupposed that the chat had changed their comfort. Since the field is required, anyone who
      felt no effect had to invent one. It now reads "Did anything about the chat affect how
      comfortable you felt sharing your experiences? If so, what? If it made no difference, please
      say so", which makes "no effect" a valid and informative answer. That matters most for the
      guarded participants the `SOC-DEFLECT` code is meant to capture.

      Note for analysis: responses collected before this change may be inflated toward reporting an
      effect. Only test rows exist so far.
- [ ] The self-assessed Comfort and Depth items are struck through in the design, but the code
      implements Comfort and omits Depth. Either the strikethrough is stale or the code is.
- [ ] Pre-interaction mood items and a human-AI trust measure appear as decisions in the design and are
      not implemented.

## The paper

The write-up lives in its own repository and is not tracked here. Its open items, including where it
and this implementation disagree, are tracked there rather than in this file. Ask Simo for the path.

## Methodology

- [x] **Fixed: the host decided how long each stage ran, which is a confound.** It emitted a
      `[STAGE_COMPLETE]` marker whenever it judged the participant had shared enough, capped at ten
      turns. The number of invitations to disclose was therefore a free variable, and one free to
      correlate with condition, since the peer disclosures sit in the history the host reads. Every
      stage now runs exactly two participant turns for everyone: the answer, then one optional chance
      to add more, invited by a fixed constant the model does not write and declinable with a
      "Nothing to add" button. Turn count is no longer a measure, and word count is comparable across
      participants for the first time. Pinned by `src/go/mirror_test.go`.
- [x] **Fixed: the host attributed peer disclosures to the participant.** Peers and the participant
      share the API's `user` role and consecutive same-role turns are folded into one message, but
      only the peers carried a speaker label, so the participant's words arrived as an unlabelled
      tail on a block headed by a peer's name. Given a short participant turn the host reliably
      acknowledged the peer's experience as though the participant had lived it. Reproduced on the
      first try in testing and fixed by labelling every speaker. It could only ever happen in `2-1`
      and `3-1`, so it did not cancel out across the design. **Any transcript collected before
      2026-08-06 may contain it**, though only test rows exist so far.
- [x] **Done: peers made more salient without changing the disclosure dose.** Each peer cell now runs
      a two-beat exchange (peer speaks, Vieno answers that peer by name, a peer answers her) instead
      of a single round of quotations. Disclosure word counts are unchanged and still matched between
      the peer cells; the same content is simply split across two shorter peer turns. Vieno's
      hand-off also varies by stage rather than repeating one formula, while the sentence that hands
      the participant the floor stays byte-identical across all three cells.
- [ ] **The baseline is now much thinner than the peer cells, on purpose.** Stage 2 carries roughly 32
      words of bot text in `1-1` against roughly 110 in each peer cell, up from 32 against 75. Padding
      the baseline was considered and rejected on 2026-08-06, because it would make the host behave
      differently by condition. The consequence is that a condition effect is an effect of the peer
      cells as a package (peers present, peers disclosing, more conversation), and the design cannot
      separate those three. **This needs a sentence in the limitations section of the paper.**
- [ ] **There is no manipulation check.** Nothing asks what the participant made of the other chat
      members: whether they noticed the peers disclosed, whether they took them for people or for
      software, or how many others they recall. A reviewer of a paper about multi-agent personas will
      ask, and the data cannot currently answer. Raised and deliberately deferred on 2026-08-06; one
      required open-ended item in the post-survey would close it.
- [x] **Done: age and gender are collected.** In the pre-survey, placed after the attitude items
      rather than at the top, since asking gender immediately before items about being judged for
      needing help is a priming risk in a study that measures exactly that. Stored as `age` (integer)
      and `gender`, with `gender_self_describe` when chosen.
- [ ] **The seriousness check is a model judgment, and its error rate on real data is unknown.**
      Measured over three runs of a 20-item probe set that was deliberately stacked with hard cases:
      22/27 non-answers caught, 6/33 genuine answers wrongly flagged, and not deterministic between
      runs. Those two rates are **not** an estimate of how often real participants will see the
      remark. Eight of the eleven genuine items were never flagged in any run, and all six false
      positives came from three telegraphic ways of saying "nothing" (`n/a` in every run, plus
      "cant think of anythin right now sorry" and "Nothing comes to mind honestly"). Ordinary prose
      answers were never flagged.

      It costs a participant one hedged sentence and never a turn, so it cannot affect the design.
      **Count `not-serious` marks in the first exports** to find the real rate, and check whether any
      landed on a genuine answer before writing anything about it in the paper.
- [x] **Superseded: neither survey collects demographics.** Fine if the analysis joins on Prolific's own export
      by `prolific_id`, which is captured. Worth confirming before recruitment closes rather than
      after.
- [ ] **Condition assignment is unbalanced.** Each session draws a condition uniformly at random and
      independently, with nothing rebalancing afterwards. Across a target of 180 the cells will drift
      away from 60 each, and abandoned sessions make it worse because they still consume an assignment.
      Assigning whichever condition currently has the fewest completed sessions is a small backend
      change.
- [x] **Fixed: bot count was confounded with how much disclosure gets modelled.** The three cells now
      share one non-disclosing host, and the disclosure content in the two peer cells is identical,
      split across one speaker or two. The single-bot cell contains no peer disclosure at all and
      serves as the baseline, so the design now tests whether peer modelling works before asking
      whether the number of peers matters. Every stage also runs in the same order in every cell,
      question then peers then hand-off, so the within-cell stage comparison is interpretable.
      Invariants are enforced by `src/go/scripts_test.go`, which fails the build if the participant
      question drifts between cells, if the disclosure volume stops matching within 10%, if the
      baseline gains a disclosure, if the host discloses, or if a peer speaks before the question.

      Message volume and time on task still rise with peer count, because a two-person conversation
      has fewer turns than a group. Padding the host to equalise was rejected as it risks turning the
      facilitator into a discloser. That residual belongs in the limitations.
- [x] **Fixed: bots did not read what the participant writes.** The host now acknowledges each
      participant message in one generated sentence, constrained to reflecting back only what was
      written: no advice, no evaluation, no new topic, no question. It runs after stages 1 and 2 and
      is on in every condition, so it does not interact with the manipulation. Any failure falls back
      to a fixed line, and each host turn in the transcript is marked `generated` or `fallback` (or
      carries no mark when it came from the script) so the three are separable when coding.
      There is deliberately no switch to turn it off, since the only thing such a switch could do is
      change the manipulation partway through recruitment.
- [x] **Fixed: bot typing speed was about 300 words per minute**, roughly four times a realistic rate.
      Now `words * 350ms + 1s`, capped at 10s, with the three values as named constants at the top of
      `chat-interface.tsx`. That is about 170 wpm, fast for a person and still believable. The cap
      stops a long message from stalling the session. The scripts have since been rewritten and are
      much shorter, so the per-session waits quoted here no longer apply; the time-on-task difference
      between conditions is now the residual noted under the disclosure-dose item above.
- [x] **Fixed: participants could see their assigned condition.** The header rendered
      "Condition: 3-1" on every screen under the title "Multibot Research Prototype", so a participant
      could infer that the number of other members was the manipulation. The condition is now shown
      under `?test=true` only, and the title reads "Peer Support Study".

## Ethics and privacy

- [ ] **There is no debriefing.** The completion screen thanks participants and shows the code, without
      stating that the other chat members were scripted. For a study collecting burnout and
      competence-anxiety disclosures from Prolific participants, confirm what the ethics approval
      commits to here.
- [ ] **The chosen chat name is stored as `display_name`.** Participants are told a nickname is fine,
      but some will enter a real first name, which makes that field personal data under GDPR. The
      current consent text refers only to "anonymized data". Check that the consent and the approval
      cover it, and decide who is responsible for the stored data.
- [ ] **Exclude the deployment test sessions from analysis.** Three sessions were created while
      verifying the deployment. They are identifiable because their Prolific IDs are not
      24-character hex strings. Alternatively wipe the database before recruiting.

## Robustness

- [ ] **The frontend hides backend failures.** Every request falls back to locally computed state, so
      if the backend is unreachable or misconfigured the study still walks the participant all the way
      to a completion code while saving nothing. Failing visibly would be safer during real data
      collection, at the cost of showing an error to whoever hits it.
- [ ] `REACT_APP_BACKEND_URL` is compiled into the bundle, so changing it needs a rebuild rather than a
      restart. Easy to forget when moving environments.
- [x] The two `react-hooks/exhaustive-deps` warnings were fixed in `a274cae`, so `npm run build` now
      passes with `CI=true`. The `CI=false` variable on the frontend service is no longer required and
      can be removed, though leaving it costs nothing and guards against the warnings returning.
- [ ] The frontend has no automated tests. The Go side has them (`main_test.go`, `scripts_test.go`,
      `mirror_test.go`, `admin_test.go`), run with `go test ./...` from `src/go`. For the frontend the
      practical smoke test is still walking the state machine with curl, since it masks server errors.
