# E-007 - Native tray readiness warning

Updated: 2026-11-12 | Status: Open observation / P2

Related feature: [F-018](F-018-native-desktop.md).

## Impact and evidence

During the final offline Windows amd64 GUI smoke launch at 21:44, Fyne/systray logged:

```text
systray error: unable to set icon: tray not ready yet
```

The native window still appeared as `UPSmart Monitor` and the process exited 0 after its three-second smoke interval. Earlier recorded smoke launches did not show this warning. Tray visibility/click behavior was not checked, so functional impact and reproducibility remain unknown. This is not evidence that USB mode selection failed.

Environment: Windows amd64, Go 1.26.5, Fyne v2.8.2, documented internal-linker GUI build. No backend requests, USB operations or power actions occurred.

## Reproduction and expected behavior

1. Build the GUI using the command in [F-018](F-018-native-desktop.md).
2. Launch `bin\upsmart-desktop.exe --smoke-test`.
3. Observe stderr and native window startup. The warning may not recur.

Expected: native window/tray initialization without readiness errors. Actual: one observed tray warning with successful window startup/exit.

## Cause, disposition and next check

Cause is unconfirmed; a tray-initialization timing issue is possible but not established. The tray setup predates dual-mode support. No speculative fix or error suppression was made in this connection-mode change.

Next: reproduce with native visual/tray interaction, distinguish smoke/startup ordering from normal-mode behavior, then fix and retest if confirmed. No claim that this observation is resolved.
