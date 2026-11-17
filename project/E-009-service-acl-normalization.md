# E-009 - Service installer rejects equivalent Windows-normalized ACL

Updated: 2026-11-16 | Priority: P1 | Status: Historical fix verified; installer subsequently replaced by explicit user choice

Subsequent decision on 2026-11-16: the user selected rdpproxy-style service handling and LocalSystem. The exact-ACL installer/functions described below were replaced, not deployed. Current behavior and pending elevated verification are in [F-006](F-006-windows-service.md#current-service-model-rdpproxy-alignment---2026-11-16). The evidence below remains the verified history of the original defect, not the current install contract.

## Feature, impact and environment

Linked feature: [F-006 Windows service](F-006-windows-service.md); regression reference: SVC-03, SVC-06.

Lỗi chặn cài Windows service ngay khi kiểm tra thư mục đích, dù chủ sở hữu và quyền truy cập đúng chính sách. Không cần xóa thư mục hoặc cấp thêm quyền để xử lý lỗi so sánh này.

Environment: Windows amd64, reported 2026-11-16 at 15:57:57. The diagnostic/test session is not elevated. The existing `C:\Program Files\UPSmart` directory was empty during read-only inspection.

## Reproduction and expected/actual behavior

1. In the user's elevated terminal, run `upsdaemon.exe service install --config config.yaml` from `D:\project\UPSmart\bin`.
2. The protected destination directory exists with the policy owner/group and allow rules.
3. Actual: installation stops with `deployment ACL/owner does not match protected policy: C:\Program Files\UPSmart; inspect it rather than changing permissions implicitly`.
4. Expected: accept equivalent protected ownership/access and continue configuration deployment; still reject genuinely different policies and never repair existing permissions implicitly.

The original installation attempt was performed by the user. This investigation did not rerun installation or start the service.

## Evidence and confirmed cause

Read-only inspection confirmed Administrators as both owner and group, protected inheritance, SYSTEM/Administrators full control and LocalService/Users read/execute with synchronize. No write permission was granted to LocalService/Users.

Expected SDDL:

```text
O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;LS)(A;OICI;GRGX;;;BU)
```

Actual SDDL:

```text
O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;0x1200a9;;;LS)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;BU)
```

Confirmed cause: `verifySecurity` compared descriptor strings exactly. Windows maps generic file read/execute rights (`GRGX`) to file-specific mask `0x1200a9`; the observed allow-rule order also differs. These textual differences do not change this allow-only policy. Temporary filesystem tests reproduced Windows rights normalization.

## Fix and boundaries

Entry points: `verifySecurity`, `sameFileSecurityPolicy`, `fileSecurityPolicy` and `fileAccessMask` in `internal/service/install_windows.go`.

- Normalize generic rights using Windows file access mappings.
- Compare owner/group SIDs and DACL presence/protection exactly.
- Compare the complete multiset of allow rules, including SID, full ACE flags, access mask and duplicate count; allow order is irrelevant only because unsupported/non-allow ACE types are explicitly rejected.
- Keep rejecting unexpected grants, insufficient access, changed inheritance/ownership, null DACLs and reparse points. No ACL reset, ownership change or production-file deletion was introduced.

No user permission repair decision is required: the existing directory already passes the corrected protected-policy check. No broader service-account privilege or deployment-policy change was selected.

## Actual regression and retest results - 2026-11-16

- `go test .\internal\service .\internal\config .\cmd\upsdaemon`: passed.
- Corresponding `go vet`: passed.
- Five consecutive targeted runs passed for the captured equivalent descriptor, changed owner/group, unprotected inheritance, writable Users, missing/extra/duplicate grants, changed/inherited ACE flags, deny entry, null/empty DACL, and existing unprotected-executable overwrite rejection.
- Temporary-directory tests passed for protected directory creation/reuse, configuration file creation/replacement and content readback with Windows-mapped rights. Test-owned objects retain current-owner access and are cleaned by the test runner.
- A temporary read-only test invoked production `verifySecurity` against the real existing Program Files destination: passed five times without changing that directory or its contents/permissions. The temporary diagnostic test source was removed.
- Windows amd64 daemon build passed with the existing CGO-disabled/internal-linker flags, producing `bin\upsdaemon.exe`.

Assessment: the reported false rejection is verified fixed at both descriptor and filesystem-check boundaries. Full elevated configuration/binary deployment, SCM registration and LocalService startup/USB access remain unverified. Next step: retry the installation from an elevated terminal with the rebuilt daemon; this record does not claim the service is installed or running. Independent UPS startup observation [E-008](E-008-q1-startup-control-response.md) remains open.
