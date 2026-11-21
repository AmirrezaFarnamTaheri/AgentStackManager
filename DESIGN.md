---
name: AgentStack Manager
description: Lifecycle Ledger interface for preservation-first local stack control.
colors:
  action-teal: "#0f766e"
  action-teal-strong: "#0a5f59"
  action-teal-soft: "#d9eeeb"
  canvas-cold: "#edf0ef"
  surface-paper: "#f8faf9"
  surface-raised: "#ffffff"
  surface-muted: "#e5eae8"
  sidebar-graphite: "#18201f"
  sidebar-muted: "#9ba8a5"
  sidebar-mark: "#22302e"
  sidebar-mark-accent: "#78b8ae"
  text-graphite: "#17201f"
  text-muted: "#596562"
  line: "#cbd3d0"
  line-strong: "#aebbb7"
  success: "#1f7a55"
  warning: "#946515"
  danger: "#a43c45"
  danger-strong: "#873039"
  action-ink-dark: "#10201c"
darkColors:
  canvas-cold: "#111715"
  surface-paper: "#18201e"
  surface-raised: "#1d2623"
  surface-muted: "#25302d"
  sidebar-graphite: "#0c1110"
  sidebar-muted: "#8d9b97"
  sidebar-mark: "#182723"
  sidebar-mark-accent: "#78c8bd"
  text-graphite: "#edf4f1"
  text-muted: "#a6b3af"
  line: "#34413d"
  line-strong: "#4a5a55"
  action-teal: "#63b9ad"
  action-teal-strong: "#78c8bd"
  action-teal-soft: "#1e3b37"
  success: "#6dc49b"
  warning: "#d7ae5e"
  danger: "#a43c45"
  danger-strong: "#873039"
  danger-text: "#f08f98"
typography:
  display:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(25px, 3.2vw, 40px)"
    fontWeight: 690
    lineHeight: 1.08
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "24px"
    fontWeight: 700
    lineHeight: 1.15
    letterSpacing: "-0.025em"
  page:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "26px"
    fontWeight: 700
    lineHeight: 1.2
  title:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "17px"
    fontWeight: 700
    lineHeight: 1.3
  metric:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "20px"
    fontWeight: 800
    lineHeight: 1.2
  progress:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "18px"
    fontWeight: 800
    lineHeight: 1.2
  body:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "12px"
    fontWeight: 680
    lineHeight: 1.4
  mono:
    fontFamily: "Cascadia Code, Cascadia Mono, SFMono-Regular, Consolas, monospace"
    fontSize: "12px"
    fontWeight: 400
    lineHeight: 1.6
rounded:
  control: "8px"
  compact: "9px"
  medium: "10px"
  brand: "11px"
  panel: "14px"
  pill: "999px"
spacing:
  compact: "8px"
  control: "12px"
  panel: "22px"
  section: "30px"
components:
  button-primary:
    backgroundColor: "{colors.action-teal}"
    textColor: "{colors.surface-raised}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    padding: "10px 14px"
    height: "44px"
  button-secondary:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.text-graphite}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    padding: "10px 14px"
    height: "44px"
  field:
    backgroundColor: "{colors.surface-paper}"
    textColor: "{colors.text-graphite}"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    padding: "10px 12px"
    height: "44px"
  panel:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.text-graphite}"
    rounded: "{rounded.panel}"
    padding: "22px"
---

# Design System: AgentStack Manager

## Overview

**Creative North Star: "Lifecycle Ledger"**

AgentStack Manager is an operational lifecycle workspace. The interface separates detected environments, selected tools, pending changes, active installation, and recorded outcomes so users can tell what exists, what they approved, what is happening, and what needs recovery at a glance. Cold paper surfaces and graphite structure keep attention on work rather than decoration. Muted teal appears only where the user can act, where the current area is selected, or where verified state needs emphasis.

The system is compact and familiar because users arrive to complete technical work. It avoids marketing-page composition, decorative motion, repeated card shells, and subsystem-first navigation. Environment rows, pending changes, progress items, transactions, fine dividers, and one persistent workspace rail provide the product character.

**Key Characteristics:**
- Lifecycle hierarchy keeps detected state, selected intent, pending changes, active installation, and recorded outcomes distinct.
- Exactly five primary destinations: Home, Environments, Sharing & Sync, Changes, and Activity.
- One continuous selection-to-approval flow and one server-authoritative apply action.
- Flat environment, tool, progress, and transaction rows instead of repeated dashboard cards.
- Responsive behavior changes topology before typography or touch targets shrink.

## Colors

The palette is a cold neutral field with one muted teal action voice and semantic colors reserved for actual state.

### Primary
- **Controlled Teal** (`action-teal`): Primary actions, active navigation, verified state, and focus emphasis.
- **Deep Controlled Teal** (`action-teal-strong`): Hover state for primary actions.
- **Washed Teal** (`action-teal-soft`): Selected rows, focus support, and low-emphasis verified surfaces.

### Neutral
- **Cold Canvas** (`canvas-cold`): Application background.
- **Paper Surface** (`surface-paper`): Default controls and quiet grouped regions.
- **Raised Paper** (`surface-raised`): Active work surface, installation tracker, and secondary buttons.
- **Muted Surface** (`surface-muted`): Hover, skeleton, and secondary structural layers.
- **Graphite Rail** (`sidebar-graphite`): Persistent workspace navigation.
- **Graphite Text** (`text-graphite`): Primary copy and controls.
- **Muted Text** (`text-muted`): Explanations, metadata, and non-primary labels.
- **Registry Line** (`line`) and **Strong Registry Line** (`line-strong`): Group boundaries and record structure.

### Semantic
- **Verified Green** (`success`): Completed or healthy state only.
- **Attention Amber** (`warning`): Recovery guidance or attention without destructive impact.
- **Authorization Red** (`danger`): Explicitly destructive or irreversible confirmation only.

**The One Action Voice Rule.** Teal is reserved for primary action, current selection, focus, and verified state. It is never scattered as decoration.

**The Semantic Honesty Rule.** Green, amber, and red communicate real system state. They do not create visual variety.

## Typography

**Display Font:** Aptos with Segoe UI Variable and system sans fallbacks  
**Body Font:** Aptos with Segoe UI Variable and system sans fallbacks  
**Label/Mono Font:** Cascadia Code with Cascadia Mono, SFMono-Regular, and Consolas fallbacks

**Character:** One compact workhorse sans family keeps the interface trustworthy and familiar. The mono stack is limited to values, versions, action records, and provenance markers.

### Hierarchy
- **Hero** (800, `clamp(25px, 3.2vw, 40px)`, 1.12): Home readiness statement only.
- **Page** (700, 26px, 1.2): Current workspace title.
- **Headline** (700, 24px, 1.25): Major section titles.
- **Metric** (800, 20px, 1.2): Summary values only.
- **Progress** (800, 18px, 1.2): Installation stage percentage and title.
- **Title** (650-700, 17px, 1.3): Panel, row, and action titles.
- **Body** (400, 16px, 1.5): Guidance and descriptions, normally kept below 75 characters per line.
- **Label / Evidence** (650-750, 12-13px): Form labels, statuses, badges, metadata, and command output.

**The Work Surface Rule.** No separate display typeface enters the product UI. Hierarchy comes from weight, scale, spacing, and structure.

**The Mono Evidence Rule.** Monospace is evidence language, not a technical costume.

## Layout

Desktop uses a 236px persistent rail and a fluid workspace capped at 1440px. Five primary destinations—Home, Environments, Sharing & Sync, Changes, and Activity—form the complete everyday path. Internal engines are represented through the environment inventory, reviewed changes, maintenance actions, or disclosed technical details instead of parallel navigation. The first viewport places a compact masthead above one concise readiness summary; the installation tracker appears only when work exists.

Spacing follows tight internal groups and larger separation between task regions. Panels use 18–22px internal padding, controls maintain 44px minimum height, and section spacing begins at 28px. At 1050px split work areas collapse to one column. At 760px the rail becomes a compact four-item navigation strip, and action, approval, tracker, and environment-detail regions stack. All grid children explicitly permit shrinking so navigation content cannot expand the page beyond the viewport.

**The Topology Before Scale Rule.** Mobile changes structure first. It does not solve crowding by shrinking labels or touch targets.

## Elevation & Depth

The system is flat by default. Registry rows, metrics, assurance lists, and command sheets use lines and tonal layers. One diffuse ambient shadow lifts the primary configuration panel, and a stronger compact shadow belongs only to transient toast feedback. Focus uses a three-pixel teal support ring rather than a shadow halo.

### Shadow Vocabulary
- **Panel Ambient** (`0 22px 48px rgba(26, 42, 39, 0.10)`): Main configuration panel only.
- **Transient Float** (`0 14px 32px rgba(17, 31, 29, 0.16)`): Toast and temporary overlay feedback.

**The Flat Registry Rule.** Records remain flat and divided. Elevation marks a work surface or a transient layer, never ordinary content.

## Shapes

Panels use gently curved 12–14px corners. Controls, fields, navigation items, and transient feedback use 8px corners. Full pills are limited to compact state badges. Borders are one pixel and structural. Major containers do not combine strong borders with strong shadows.

**The Two Radius Rule.** Use 8px for controls and 12px for surfaces. Pills are status artifacts, not a general shape language.

## Components

### Buttons
- **Shape:** Compact control corners (8px), 44px minimum height, and 10px by 14px padding.
- **Primary:** Controlled Teal background with raised-paper text.
- **Secondary:** Raised-paper background, graphite text, and registry-line border.
- **Quiet:** Transparent at rest, muted text, and a muted-surface hover.
- **Danger:** Authorization Red only for the final reviewed apply action.
- **Hover / Focus:** One-pixel upward transform on hover, one-pixel press on active, and a visible three-pixel focus outline. Disabled controls retain shape and lose emphasis.

### Cards / Containers
- **Corner Style:** 12–14px for the active work surface and installation tracker.
- **Background:** Raised Paper for active work, Paper Surface for quiet grouped regions.
- **Shadow Strategy:** Panel Ambient only on the primary configuration surface.
- **Border:** One-pixel Registry Line.
- **Internal Padding:** 22px desktop, 18px on narrow mobile.

### Inputs / Fields
- **Style:** Paper Surface, Registry Line border, 8px corners, 44px minimum height.
- **Focus:** Controlled Teal border plus Washed Teal support ring.
- **Error / Disabled:** Semantic text and state, with disabled controls visibly muted while remaining readable.

### Navigation
- The desktop rail exposes exactly five primary destinations: Home, Environments, Sharing & Sync, Changes, and Activity.
- Internal engines do not become additional primary navigation. Their evidence appears in the environment inventory, changes, activity, maintenance, or technical details.
- The current destination uses a teal marker and `aria-current="page"`; inactive destinations remain quiet.
- Mobile retains all five text labels in a compact navigation strip and never substitutes icon-only navigation.

### Installation Tracker and Notice
- The tracker is the durable status surface for active and partial installation operations.
- It displays the server-reported stage, percentage, completed/total count, active item, and bounded per-item states.
- A separate notice may summarize the outcome and one recovery action, but it never duplicates the tracker or exposes private paths.
- Motion is limited to state feedback and respects reduced-motion settings.

### Home Summary and Environment Ledger
- Home shows readiness, environment count, installed tools, attention items, recent activity, and one recommended next action.
- Environments provides a read-only inventory of AI apps, IDEs, command-line tools, MCP servers, workspaces, resources, and managed connection state.
- Connection state is visible without implying that generic tool changes mutate sharing or synchronization.
- Values use mono typography only when they are evidence; labels and guidance use the UI sans.

### Registry Row
- Component selection is a full-width row with checkbox, name, explanation, and optional metadata seals.
- Hover uses a small horizontal transform rather than padding or size animation.
- Selected rows receive a Washed Teal surface without adding elevation.

### State Badges
- Badges are compact text labels with fine borders and pill geometry.
- They identify installed, available, running, successful, failed, skipped, or attention state. They are never decorative tags and never rely on color alone.

### Changes Workspace
- Changes presents one continuous path: choose a profile and tools, inspect the inline estimate, create the exact pending-change list, confirm, and apply.
- Provider, credential, and runtime-update controls remain secondary to the common profile path.
- Uses precise, plain-language copy. It never promises absolute safety or implies that a consequential change bypasses review.
- Apply is the only client mutation action. A reviewed plan is marked consumed before execution and cannot be retried after success or partial failure.

### Pending Changes and Tool Filters
- Category filter buttons (`All`, `Essential`, `Recommended`, `Optional Local`, `Credential`) filter the tool registry without page reload.
- The exact consequential actions are rendered directly below the selection workspace before approval. Already-ready items are summarized rather than repeated as a second list.
- Search is debounced and preserves focus while filtering.

## Do's and Don'ts

### Do:
- **Do** keep detected state, selected intent, pending changes, active installation, and recorded outcome visually distinct.
- **Do** reserve teal for action, selection, focus, and verified state.
- **Do** use registry lines and negative space before adding another container.
- **Do** keep every primary control at least 44px high and preserve visible focus.
- **Do** use the mono stack for evidence such as values, versions, seals, and command output.
- **Do** collapse layout topology at 1050px and 760px before reducing density (with 460px and 1180px supporting rules in styles.css).
- **Do** state preservation and authorization guarantees precisely; describe the reviewed mechanism instead of promising absolute safety.

### Don't:
- **Don't** introduce a marketing hero, decorative dashboard cards, or promotional proof into the manager.
- **Don't** add a second accent color, purple glow, decorative glass, or color without state meaning.
- **Don't** animate layout-driving properties such as width, height, margin, or padding.
- **Don't** hide navigation behind unlabeled icons or reduce touch targets to make mobile fit.
- **Don't** use pills for ordinary buttons, panels, or form controls.
- **Don't** use repeated eyebrows, section numbers, or technical microcopy as decoration.

## Anti-Slop Audit Log & Quality Gate

| Slop Tell | Status | Remediation Applied |
| :--- | :---: | :--- |
| Generic Fonts (Inter/Roboto/System) | **PASSED** | Structural font stack: Aptos / Segoe UI Variable (UI sans) + Cascadia Code / SFMono (Evidence mono). |
| Purple/Indigo Gradients | **PASSED** | Palette strictly locked to Cold Canvas (`#edf0ef` / `#111715`) + Controlled Teal (`#0f766e` / `#63b9ad`). |
| Structural Icons | **PASSED** | Structural navigation icons use one local SVG language; state and action labels use text rather than decorative glyphs. |
| Hover Layout Shift | **PASSED** | Smooth 180ms CSS transitions on color/opacity/border. Non-layout driving transforms only. |
| Sub-44px Touch Targets | **PASSED** | All primary buttons, selects, and input controls meet minimum `44px` height requirement. |
| Missing Focus Outline | **PASSED** | `3px` visible focus outline on all interactive controls (`:focus-visible`). |
| Color-Only Indicators | **PASSED** | Every state communicates through visible text and structure in addition to semantic color. |
