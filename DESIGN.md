---
name: AgentStack Manager
description: Conservation Ledger interface for preservation-first local stack control.
colors:
  action-teal: "#0f766e"
  action-teal-strong: "#0a5f59"
  action-teal-soft: "#d9eeeb"
  canvas-cold: "#edf0ef"
  surface-paper: "#f8faf9"
  surface-raised: "#ffffff"
  surface-muted: "#e5eae8"
  sidebar-graphite: "#18201f"
  text-graphite: "#17201f"
  text-muted: "#596562"
  line: "#cbd3d0"
  line-strong: "#aebbb7"
  success: "#1f7a55"
  warning: "#946515"
  danger: "#a43c45"
typography:
  display:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "clamp(28px, 3vw, 38px)"
    fontWeight: 690
    lineHeight: 1.08
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "24px"
    fontWeight: 700
    lineHeight: 1.15
    letterSpacing: "-0.025em"
  body:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "Aptos, Segoe UI Variable Text, Segoe UI, ui-sans-serif, system-ui, sans-serif"
    fontSize: "12px"
    fontWeight: 680
    lineHeight: 1.4
  mono:
    fontFamily: "Cascadia Code, Cascadia Mono, SFMono-Regular, Consolas, monospace"
    fontSize: "11px"
    fontWeight: 400
    lineHeight: 1.7
rounded:
  control: "8px"
  panel: "12px"
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

**Creative North Star: "Conservation Ledger"**

AgentStack Manager is an operational provenance workbench. The interface separates observed machine state, plan inputs, reviewed changes, and authorized mutation so users can tell what is known, proposed, and permitted at a glance. Cold paper surfaces and graphite structure keep attention on records rather than decoration. Muted teal appears only where the user can act, where the current section is selected, or where verified state needs emphasis.

The system is compact and familiar because users arrive to complete a technical task. It avoids marketing-page composition, decorative motion, and repeated card shells. Registry rows, fine dividers, status seals, and one persistent workspace rail provide the product character.

**Key Characteristics:**
- Preservation-first hierarchy with observed, planned, and authorized states kept visually distinct.
- Restrained cold-neutral palette with one scarce action accent.
- Workhorse sans typography for every task surface, with mono reserved for data and provenance.
- Flat registry structure by default, with elevation used only for the main work surface and transient feedback.
- Responsive behavior that changes topology instead of shrinking desktop type.

## Colors

The palette is a cold neutral field with one muted teal action voice and semantic colors reserved for actual state.

### Primary
- **Controlled Teal** (`action-teal`): Primary actions, active navigation, verified state, and focus emphasis.
- **Deep Controlled Teal** (`action-teal-strong`): Hover state for primary actions.
- **Washed Teal** (`action-teal-soft`): Selected rows, focus support, and low-emphasis verified surfaces.

### Neutral
- **Cold Canvas** (`canvas-cold`): Application background.
- **Paper Surface** (`surface-paper`): Default controls and quiet grouped regions.
- **Raised Paper** (`surface-raised`): Main configuration panel, operation ribbon, and secondary buttons.
- **Muted Surface** (`surface-muted`): Hover, skeleton, and secondary structural layers.
- **Graphite Rail** (`sidebar-graphite`): Persistent workspace navigation.
- **Graphite Text** (`text-graphite`): Primary copy and controls.
- **Muted Text** (`text-muted`): Explanations, metadata, and non-primary labels.
- **Registry Line** (`line`) and **Strong Registry Line** (`line-strong`): Group boundaries and record structure.

### Semantic
- **Verified Green** (`success`): Completed or healthy state only.
- **Review Amber** (`warning`): Recovery guidance or attention without destructive impact.
- **Authorization Red** (`danger`): Explicitly destructive or irreversible confirmation only.

**The One Action Voice Rule.** Teal is reserved for primary action, current selection, focus, and verified state. It is never scattered as decoration.

**The Semantic Honesty Rule.** Green, amber, and red communicate real system state. They do not create visual variety.

## Typography

**Display Font:** Aptos with Segoe UI Variable and system sans fallbacks  
**Body Font:** Aptos with Segoe UI Variable and system sans fallbacks  
**Label/Mono Font:** Cascadia Code with Cascadia Mono, SFMono-Regular, and Consolas fallbacks

**Character:** One compact workhorse sans family keeps the interface trustworthy and familiar. The mono stack is limited to values, versions, action records, and provenance markers.

### Hierarchy
- **Display** (690, `clamp(28px, 3vw, 38px)`, 1.08): Workspace title only.
- **Headline** (700, 24px, 1.15): Section titles.
- **Title** (650-700, 13-17px): Panel, row, and action titles.
- **Body** (400, 13-14px, 1.5-1.55): Guidance and descriptions, normally kept below 75 characters per line.
- **Label** (650-700, 10-12px): Form labels, statuses, badges, and navigation.
- **Mono** (400, 9-11px): Numeric values, version strings, record seals, and command output.

**The Work Surface Rule.** No separate display typeface enters the product UI. Hierarchy comes from weight, scale, spacing, and structure.

**The Mono Evidence Rule.** Monospace is evidence language, not a technical costume.

## Layout

Desktop uses a 248px persistent rail and a fluid workspace capped at 1560px. The first viewport places a compact task masthead above a live operation ribbon, then a metric ledger and a two-part work area. The main configuration surface receives elevation; the protection register remains flat and divided.

Spacing follows tight internal groups and larger separation between task regions. Panels use 22px internal padding, controls maintain 44px minimum height, and section spacing begins at 30px. The 1120px breakpoint collapses split work areas to one column. At 800px the rail becomes a sticky compact header with horizontally scrollable section tabs. At 520px action clusters and button rows become one-column stacks. All grid children explicitly permit shrinking so navigation content cannot expand the page beyond the viewport.

**The Topology Before Scale Rule.** Mobile changes structure first. It does not solve crowding by shrinking labels or touch targets.

## Elevation & Depth

The system is flat by default. Registry rows, metrics, assurance lists, and command sheets use lines and tonal layers. One diffuse ambient shadow lifts the primary configuration panel, and a stronger compact shadow belongs only to transient toast feedback. Focus uses a three-pixel teal support ring rather than a shadow halo.

### Shadow Vocabulary
- **Panel Ambient** (`0 22px 48px rgba(26, 42, 39, 0.10)`): Main configuration panel only.
- **Transient Float** (`0 14px 32px rgba(17, 31, 29, 0.16)`): Toast and temporary overlay feedback.

**The Flat Registry Rule.** Records remain flat and divided. Elevation marks a work surface or a transient layer, never ordinary content.

## Shapes

Panels use gently curved 12px corners. Controls, fields, navigation items, and transient feedback use 8px corners. Full pills are limited to compact record seals and action-state badges. Borders are one pixel and structural. Major containers do not combine strong borders with strong shadows.

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
- **Corner Style:** 12px for the primary panel and operation ribbon.
- **Background:** Raised Paper for active work, Paper Surface for quiet grouped regions.
- **Shadow Strategy:** Panel Ambient only on the primary configuration surface.
- **Border:** One-pixel Registry Line.
- **Internal Padding:** 22px desktop, 18px on narrow mobile.

### Inputs / Fields
- **Style:** Paper Surface, Registry Line border, 8px corners, 44px minimum height.
- **Focus:** Controlled Teal border plus Washed Teal support ring.
- **Error / Disabled:** Semantic text and state, with disabled controls visibly muted while remaining readable.

### Navigation
- The desktop rail uses muted labels at rest, raised graphite on hover, and a teal inset marker for the current section.
- Mobile retains text labels and moves the same items into a horizontally scrollable strip. It never substitutes icon-only navigation.

### Operation Ribbon
- The ribbon is the single live status surface for running, completed, and failed operations.
- It uses a compact state icon, action title, recovery or completion detail, and a provenance seal.
- Motion is limited to state feedback and respects reduced-motion settings.

### Metric Ledger
- Four machine-state metrics share one horizontal record with internal dividers rather than separate cards.
- Values use mono typography; labels and explanatory text use the UI sans.
- Loading uses shape-matched skeletons with no generic spinner.

### Registry Row
- Component selection is a full-width row with checkbox, name, explanation, and optional metadata seals.
- Hover uses a small horizontal transform rather than padding or size animation.
- Selected rows receive a Washed Teal surface without adding elevation.

### Record Seals
- Seals are compact provenance labels with mono text, fine borders, and pill geometry.
- They identify state or ownership. They are never decorative tags.

## Do's and Don'ts

### Do:
- **Do** keep observed state, plan input, reviewed output, and authorization visually distinct.
- **Do** reserve teal for action, selection, focus, and verified state.
- **Do** use registry lines and negative space before adding another container.
- **Do** keep every primary control at least 44px high and preserve visible focus.
- **Do** use the mono stack for evidence such as values, versions, seals, and command output.
- **Do** collapse layout topology at 1120px, 800px, and 520px before reducing density.

### Don't:
- **Don't** introduce a marketing hero, decorative dashboard cards, or promotional proof into the manager.
- **Don't** add a second accent color, purple glow, decorative glass, or color without state meaning.
- **Don't** animate layout-driving properties such as width, height, margin, or padding.
- **Don't** hide navigation behind unlabeled icons or reduce touch targets to make mobile fit.
- **Don't** use pills for ordinary buttons, panels, or form controls.
- **Don't** use repeated eyebrows, section numbers, or technical microcopy as decoration.
