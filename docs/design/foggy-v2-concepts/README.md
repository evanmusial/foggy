# Foggy v2 design concepts

Generated October 1, 2026 as the visual approval gate for the Foggy v2 rebuild.

## Approval status

**Pending.** These concepts are committed for review and do not authorize application implementation. The implementation contract should be finalized only after explicit approval or a requested revision.

## Concept set

| Surface | Asset | Intended viewport | Purpose |
| --- | --- | --- | --- |
| Desktop Review | [`desktop-review.png`](desktop-review.png) | Approximately 1440 × 1024 | Longitudinal review, functional change, episode recovery, coverage, and report preparation |
| Mobile Today | [`mobile-today.png`](mobile-today.png) | Approximately 390 × 844 | One-tap unchanged check-in, changed symptom/event entry, active concerns, and weekly function |
| Mobile landscape Review | [`mobile-landscape-review.png`](mobile-landscape-review.png) | Approximately 844 × 390 | Wide change and episode timeline with a persistent selected-record detail panel |

## Proposed semantic contract

- Mobile logging and desktop review are sibling surfaces, not a compressed version of one layout.
- The Today screen leads with one-tap “Nothing meaningfully changed”; no concern value is preselected or copied forward.
- The Review screen leads with a plain-language insight, followed immediately by supporting longitudinal evidence.
- Change strips, episode lanes, functional activities, coverage, labels, annotations, and caveats remain live, code-rendered, data-bound layers.
- Missing observations stay visible as gaps. A missed entry is never interpreted as zero or no symptoms.
- Essential values remain visible without hover; touch, keyboard, previous/next, screen-reader, table, reduced-motion, and print paths are required.
- Mobile landscape keeps the wide timeline visible while filters and selected-record details remain adjacent.
- Offline and stale states preserve the last usable view and clearly identify pending or last-synced data.
- Red is reserved for emergency or onset semantics; blue is the primary focus color, teal indicates improvement, amber indicates worsening or attention, and neutral gray carries context.
- The interface must not imply diagnosis, causation, relapse classification, progression scoring, or real-time monitoring.

## Flexible implementation details

Exact spacing, final typography tokens, chart geometry, breakpoints, and minor icon choices may change as long as the approved hierarchy, color roles, mobile continuation, safety language, data meaning, and report/export continuation are preserved.

## Generation metadata

- Tool: Codex built-in image generation
- Tool model/version: not exposed by the built-in tool
- Use case: `ui-mockup`
- Inputs: text-only; no reference images
- Background: opaque
- Concepts are design references only. No rasterized labels, values, charts, or controls should ship in the product.

### Desktop Review prompt

High-fidelity desktop web application concept for a calm, patient-owned MS Review workspace. Use a warm off-white background, white evidence surfaces, deep navy text, cobalt focus, teal improvement, amber worsening/attention, and red only for emergency/onset. Include a left navigation rail; a 30-day Review header; the insight “Fatigue felt similar, but showering took more rest.”; direct-labeled ordinal change strips for Fatigue, Left leg stiffness, and Hand coordination; an episode lane; a supporting “What changed” column; functional reference activities; coverage; “Prepare visit report”; an example-data caveat; and a not-monitored-in-real-time note. Preserve an evidence-first reading path and avoid generic dashboard tiles, gradients, decorative imagery, hover-only discovery, diagnosis language, and causal claims.

### Mobile Today prompt

High-fidelity mobile portrait concept for extremely fast, one-handed logging. Show Foggy, the friendly date, protected/offline state, lock control, “Not monitored in real time,” “How are things today?”, a dominant “Nothing meaningfully changed” button, supporting “Something changed” and “Log an event” actions, three active concerns with last-recorded status but no current preselection, a one-minute weekly function check, offline-sync messaging, and Today/Review/Reports/More navigation. Use large touch targets and avoid sliders, mood faces, streaks, rings, scores, decorative imagery, medical clichés, and diagnosis language.

### Mobile landscape Review prompt

High-fidelity mobile-landscape Review concept that preserves a wide longitudinal timeline. Include a compact Review header, 30-day and concern controls, offline/stale state, Change/Episodes/Function/Coverage tabs, a direct-labeled Fatigue change strip with visible gaps, an episode lane with onset/follow-up/improving/not-back-to-baseline states, an adjacent selected-date detail panel, previous/next controls, and the example-data caveat. Keep the main visualization visible at all times and avoid squeezed desktop navigation, settings-first layout, hover-only or drag-only interaction, tiny labels, decorative imagery, and causal or diagnostic claims.
