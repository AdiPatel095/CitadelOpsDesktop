# Stable 2.3.6 handover bridge

The original bridge started at CDS-paused stable source `daacfc7` and
backported the reviewed worker archive/fencing/adoption commits with `-x`
provenance, plus the gated executor transport support. It retains version 2.3.6.
The current bridge also integrates CDS batching/unpause `025c27a` and the queued
Auto Bird support-expiry/castle controls from `427709a`. The App and Automation
trees match that stable hotfix source; beta-only policies are not imported.
This prevents rollout from restoring the obsolete blanket CDS pause.

The 2.4.0-beta.2 data model and persistence readers/writers are backported so a
return from beta cannot drop new fields during stable snapshot writes. Report
and telemetry classifications recognize retained beta histories. Stable SEI
refresh preserves the booster observations it does not parse. No booster or
fortress purchase/attack policy is registered by this compatibility work.

State migration parity includes Advisor Time Skip usage, booster occurrence and
purchase status, feast uncertainty metadata, stationing preset identity,
Advisor movement fields, fortress map history, and resource report categories.
The full-rewrite persistence test exercises repeated legacy-to-component saves,
including multiple support movement IDs, expected return time, castle pause
expiry and rescan requests. Those fields survive a return from the matching beta.
Canonical settings remain raw versioned sections, including beta sections;
configuration sync preserves the exact canonical revision/digest. Stable retains
its existing installation-local defaults and research setting boundary; those
are not substituted for the canonical account settings.

Do not infer compatibility with future beta versions. Pin this exact build pair
only after Cloud Build provenance, archive transport, profile fixture and live
readiness verification. This document is not a deployment receipt.
