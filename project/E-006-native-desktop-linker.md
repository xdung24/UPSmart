# E-006 - Default native GUI link rejected by Windows

Updated: 2026-11-12 | Status: Resolved for documented build path / P1

Related: [F-018](F-018-native-desktop.md), [F-012](F-012-delivery-verification.md), [F-015](F-015-quality.md).

## Impact and reproduction

On this Windows amd64 machine, Go 1.26.5 with installed MinGW GCC 8.1.0 / linker 2.30 successfully compiled Fyne v2.8.2, but the default external-link GUI executable could not be launched.

```powershell
go build -ldflags "-H=windowsgui" -o .\bin\upsmart-desktop.exe .\cmd\upsmart-desktop
.\bin\upsmart-desktop.exe --smoke-test
```

Expected: show the desktop briefly, then exit 0 without backend requests. Actual: Windows loader error 193, “not a valid application for this OS platform.”

Evidence: the rejected executable was PE32+ / machine 0x8664, not a 32/64-bit target mismatch. Its external-link headers reported file alignment 0x200 and SizeOfHeaders 0x520. Whether that particular header value is the definitive loader cause is unconfirmed.

## Resolution and verification

The same sources and C compiler build correctly using Go's supported internal Windows linker:

```powershell
go build -ldflags "-H=windowsgui -linkmode=internal" -o .\bin\upsmart-desktop.exe .\cmd\upsmart-desktop
```

The resulting executable launched a real native window titled `UPSmart Monitor`, then exited 0 in smoke mode. The controlled linker-path change establishes compatibility of the documented build path; a specific GCC/binutils defect or fix in newer compilers is not claimed.

`build-desktop.ps1` and desktop build instructions persist these flags. No PE-header edits, compiler replacement, global toolchain settings, service operation or actual power test were used. This does not resolve or retest the separate race-runtime observation [E-003](E-003-race-runtime-startup.md).
