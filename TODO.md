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

Use `?test=true` to walk the whole flow without answering anything. Combine it with the Prolific
parameters if you want to check those too:

```
/?test=true
/?PROLIFIC_PID=<id>&STUDY_ID=<id>&SESSION_ID=<id>
```

Every preset uses the lowest value on its scale, so test rows are recognisable in the data even if
the flag is ever missed. Filter on `pre_survey_data.test_mode` when analysing.

- [ ] **`?test=true` is not access-controlled.** Any participant who discovers it can skip the chat
      and reach a valid completion code in seconds. Those sessions are flagged `test_mode: true`, so
      they can be spotted and rejected on Prolific, but that is a manual process control rather than
      a technical one. Decide whether that is acceptable before recruiting, since a build-time secret
      would sit in the JavaScript bundle and not actually be secret.

## Blocking before recruitment

- [ ] **Re-enable the between-stage comfort assessments.** `ENABLE_ASSESSMENTS` is `false` in
      `src/gui/chat-interface.tsx`, so the item never appears and both comfort scores are stored as
      `0`. Confirmed against the deployed backend. This is the only quantitative measure of change
      across stages, so the within-subjects half of the design currently has no data behind it. The UI
      is already built, so this is a one-line change.
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

## Methodology

- [ ] **Condition assignment is unbalanced.** Each session draws a condition uniformly at random and
      independently, with nothing rebalancing afterwards. Across a target of 180 the cells will drift
      away from 60 each, and abandoned sessions make it worse because they still consume an assignment.
      Assigning whichever condition currently has the fewest completed sessions is a small backend
      change.
- [ ] **Bot count is confounded with how much disclosure gets modelled.** Stage 2 carries one vulnerable
      disclosure in the single-bot condition, two in the two-bot condition and three in the three-bot
      condition. Stage 1 runs 1, 5 and 7 messages respectively. An effect therefore cannot be
      attributed to several peers as opposed to simply more peer disclosure to read. Either equalise
      the disclosure content and vary only who speaks it, or treat the manipulation as a composite and
      say so in the limitations. Worth deciding deliberately rather than in response to review.
- [ ] **Bots do not read what the participant writes.** The scripts are fixed, so a participant who
      discloses nothing still gets replies referring to what they shared. This lands hardest on exactly
      the guarded, deflecting participants the codebook is built to detect, and it is the most likely
      way someone concludes the other members are not real. Consider a short neutral bridging line, or
      accept it and watch for it in the qualitative responses.
- [ ] **Bot typing speed is about 300 words per minute**, roughly four times a realistic rate. The delay
      is `words * 200ms + 1s` in `chat-interface.tsx`. Worth slowing if believability matters.

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
- [ ] There are no automated tests on either side. The practical smoke test is walking the state machine
      with curl, since the frontend masks server errors.
