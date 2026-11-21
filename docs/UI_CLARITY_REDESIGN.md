# Embedded manager clarity redesign — superseded design note

This document records the earlier transition from a ten-destination interface to the historical Setup / Tools / Review / Operate model. That model has now been replaced by the lifecycle workspace documented in [`UI_LIFECYCLE_WORKSPACE.md`](UI_LIFECYCLE_WORKSPACE.md).

The durable lessons from this pass remain active:

- one client authorization path;
- readable typography and 44px interaction targets;
- plain operational language;
- explicit light and dark theme tokens;
- stable search focus;
- atomic live announcements;
- bounded activity history;
- responsive layouts without horizontal overflow.

The current primary destinations are **Home**, **Environments**, **Changes**, and **Activity**. Selection, exact review, and approval now remain together in Changes. Installation progress, partial-failure recovery, transaction history, maintenance, and diagnostics now live in Activity.
