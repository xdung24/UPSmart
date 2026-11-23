# E-012 - Log content disappears after repeated refresh

Updated: 2026-11-23 | Linked features: [F-018](F-018-native-desktop.md) | Status: Fixed and regression-tested / P1

## Impact and environment

On the Fyne Windows desktop, the Log tab initially displayed events but became
visually blank or showed only clipped fragments after running for a while. The
backend snapshot still contained event history; the defect was in repeated GUI
rendering.

## Reproduction

1. Open the desktop Log tab.
2. Allow the 2-second snapshot refresh to update event history repeatedly.
3. Leave the window running with a long event list.

Expected: existing event text remains readable and scrollable.

Actual: the tab could become blank or leave fragments along its left edge.

## Cause

The Log tab used one large `widget.RichText` segment and replaced its segment
objects during every changed snapshot. Repeated Fyne refreshes could leave the
rich-text canvas objects misplaced or not visibly rendered. The event store and
IPC page were not cleared.

## Fix approach

- Render the log as a wrapped `widget.Label` inside `container.NewVScroll`.
- Update the label text instead of replacing RichText segment objects.
- Refresh the label and its scroll container together.
- Display `No events recorded yet.` for a genuinely empty event page.

## Verification

2026-11-23: `go test -count=1 .\internal\desktop` and
`go vet .\internal\desktop` passed. A regression test applied a 100-line event
log ten times, verified all text remained present, then confirmed empty history
shows the explicit placeholder. Native screenshot retest remains pending.
