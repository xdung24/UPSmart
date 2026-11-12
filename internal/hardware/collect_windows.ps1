$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$result = @{
    GPUs = @(); FallbackGPUs = @(); Engines = @(); GPUMemory = @()
    Volumes = @(); Physical = @(); Activity = @(); Performance = @()
    Errors = @{}
}
function Record-Error([string]$key, $failure) {
    $message = [string]$failure.Exception.Message
    if ($message.Length -gt 500) { $message = $message.Substring(0, 500) }
    $result.Errors[$key] = $message
}
function Query-Cim([string]$class, [string[]]$properties, [int]$limit = 128, [string]$namespace = 'root/cimv2') {
    # One extra record detects truncation rather than silently undercounting.
    $rows = @(Get-CimInstance -Namespace $namespace -ClassName $class -Property $properties -OperationTimeoutSec 2 |
        Select-Object -First ($limit + 1) -Property $properties)
    if ($rows.Count -gt $limit) { throw "$class exceeded bounded record limit" }
    return $rows
}
try {
    # DXGI gives a correctly sized dedicated-memory capacity and adapter LUID.
    # Win32_VideoController.AdapterRAM is uint32 and is deliberately not used.
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Threading;
public static class UPSmartHardwareDXGI {
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    public struct Description {
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)] public string Name;
        public uint Vendor, Device, Subsystem, Revision;
        public UIntPtr DedicatedVideo, DedicatedSystem, SharedSystem;
        public uint LuidLow;
        public int LuidHigh;
        public uint Flags;
    }
    public sealed class Adapter {
        public string ID;
        public string Name;
        public ulong DedicatedTotal;
    }
    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    [DllImport("dxgi.dll", ExactSpelling = true)]
    private static extern int CreateDXGIFactory1(ref Guid iid, out IntPtr factory);
    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int Enumerate(IntPtr self, uint index, out IntPtr adapter);
    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int Describe(IntPtr self, out Description description);
    [DllImport("pdh.dll", CharSet = CharSet.Unicode)]
    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    private static extern uint PdhOpenQuery(string source, IntPtr userData, out IntPtr query);
    [DllImport("pdh.dll", CharSet = CharSet.Unicode)]
    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    private static extern uint PdhAddEnglishCounter(IntPtr query, string path, IntPtr userData, out IntPtr counter);
    [DllImport("pdh.dll")]
    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    private static extern uint PdhCollectQueryData(IntPtr query);
    [DllImport("pdh.dll", CharSet = CharSet.Unicode)]
    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    private static extern uint PdhGetFormattedCounterArray(IntPtr counter, uint format, ref uint bytes, out uint count, IntPtr buffer);
    [DllImport("pdh.dll")]
    [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
    private static extern uint PdhCloseQuery(IntPtr query);
    [StructLayout(LayoutKind.Sequential)]
    private struct CounterValue { public uint Status; public double Value; }
    [StructLayout(LayoutKind.Sequential)]
    private struct CounterItem { public IntPtr Name; public CounterValue Value; }
    public sealed class Counter {
        public string Kind;
        public string Name;
        public double Value;
    }
    public sealed class PerformanceResult {
        public List<Counter> Values = new List<Counter>();
        public Dictionary<string, string> Errors = new Dictionary<string, string>();
    }
    private static string Status(uint status) {
        string description = status == 0xc0000bb8 ? " (PDH_CSTATUS_NO_OBJECT: performance object not found)" : "";
        return String.Format("PDH status 0x{0:x8}{1}", status, description);
    }
    public static PerformanceResult Performance() {
        var result = new PerformanceResult();
        string[] paths = {
            @"\PhysicalDisk(*)\% Idle Time", @"\PhysicalDisk(*)\Disk Read Bytes/sec",
            @"\PhysicalDisk(*)\Disk Write Bytes/sec", @"\GPU Engine(*)\Utilization Percentage",
            @"\GPU Adapter Memory(*)\Dedicated Usage", @"\GPU Adapter Memory(*)\Shared Usage"
        };
        string[] kinds = { "disk_idle", "disk_read", "disk_write", "gpu_engine", "gpu_dedicated", "gpu_shared" };
        string[] groups = { "disk_activity.disk_idle", "disk_activity.disk_read", "disk_activity.disk_write", "gpu_engines", "gpu_memory", "gpu_memory" };
        IntPtr query;
        uint status = PdhOpenQuery(null, IntPtr.Zero, out query);
        if (status != 0) throw new InvalidOperationException(Status(status));
        try {
            IntPtr[] counters = new IntPtr[paths.Length];
            for (int i = 0; i < paths.Length; i++) {
                status = PdhAddEnglishCounter(query, paths[i], IntPtr.Zero, out counters[i]);
                if (status != 0) {
                    result.Errors[groups[i]] = kinds[i] + ": " + Status(status);
                    counters[i] = IntPtr.Zero;
                }
            }
            status = PdhCollectQueryData(query);
            if (status != 0) throw new InvalidOperationException("PDH baseline: " + Status(status));
            Thread.Sleep(250);
            status = PdhCollectQueryData(query);
            if (status != 0) throw new InvalidOperationException("PDH sample: " + Status(status));
            for (int i = 0; i < counters.Length; i++) {
                if (counters[i] == IntPtr.Zero) continue;
                uint bytes = 0, count;
                // DOUBLE | NOCAP100: reject inconsistent percentages in Go,
                // rather than silently presenting a capped successful reading.
                const uint format = 0x200 | 0x8000;
                status = PdhGetFormattedCounterArray(counters[i], format, ref bytes, out count, IntPtr.Zero);
                if (status != 0x800007d2 || bytes == 0 || bytes > 524288) {
                    result.Errors[groups[i]] = kinds[i] + ": unavailable or oversized counter array, " + Status(status);
                    continue;
                }
                IntPtr buffer = Marshal.AllocHGlobal((int)bytes);
                try {
                    status = PdhGetFormattedCounterArray(counters[i], format, ref bytes, out count, buffer);
                    int size = Marshal.SizeOf(typeof(CounterItem));
                    uint limit = i == 3 ? 2048U : 128U;
                    if (status != 0 || count > limit || (ulong)count * (ulong)size > bytes) {
                        result.Errors[groups[i]] = kinds[i] + ": invalid or oversized counter array, " + Status(status);
                        continue;
                    }
                    for (int j = 0; j < count; j++) {
                        var item = (CounterItem)Marshal.PtrToStructure(IntPtr.Add(buffer, j * size), typeof(CounterItem));
                        if (item.Value.Status != 0 && item.Value.Status != 1) {
                            result.Errors[groups[i]] = kinds[i] + ": invalid instance, " + Status(item.Value.Status);
                            continue;
                        }
                        result.Values.Add(new Counter { Kind = kinds[i], Name = Marshal.PtrToStringUni(item.Name), Value = item.Value.Value });
                    }
                } finally { Marshal.FreeHGlobal(buffer); }
            }
            return result;
        } finally { PdhCloseQuery(query); }
    }
    public static Adapter[] Read() {
        Guid iid = new Guid("770aae78-f26f-4dba-a829-253c83d1b387");
        IntPtr factory;
        Marshal.ThrowExceptionForHR(CreateDXGIFactory1(ref iid, out factory));
        try {
            IntPtr table = Marshal.ReadIntPtr(factory);
            var enumerate = (Enumerate)Marshal.GetDelegateForFunctionPointer(
                Marshal.ReadIntPtr(table, 12 * IntPtr.Size), typeof(Enumerate));
            var adapters = new List<Adapter>();
            for (uint i = 0; i <= 64; i++) {
                IntPtr adapter;
                int hr = enumerate(factory, i, out adapter);
                if (hr == unchecked((int)0x887A0002)) break;
                Marshal.ThrowExceptionForHR(hr);
                try {
                    if (i == 64) throw new InvalidOperationException("DXGI adapter limit exceeded");
                    var describe = (Describe)Marshal.GetDelegateForFunctionPointer(
                        Marshal.ReadIntPtr(Marshal.ReadIntPtr(adapter), 10 * IntPtr.Size), typeof(Describe));
                    Description d;
                    Marshal.ThrowExceptionForHR(describe(adapter, out d));
                    if ((d.Flags & 2) != 0) continue; // Exclude software rasterizers.
                    adapters.Add(new Adapter {
                        ID = String.Format("0x{0:x8}_0x{1:x8}", unchecked((uint)d.LuidHigh), d.LuidLow),
                        Name = d.Name, DedicatedTotal = d.DedicatedVideo.ToUInt64()
                    });
                } finally { Marshal.Release(adapter); }
            }
            return adapters.ToArray();
        } finally { Marshal.Release(factory); }
    }
}
'@ | Out-Null
    $result.GPUs = @([UPSmartHardwareDXGI]::Read())
} catch { Record-Error 'gpu_inventory' $_ }
if ($result.GPUs.Count -eq 0) {
    try { $result.FallbackGPUs = @(Query-Cim 'Win32_VideoController' @('Name') 64) }
    catch { Record-Error 'gpu_fallback_inventory' $_ }
}
try { $result.Volumes = @(Query-Cim 'Win32_LogicalDisk' @('DeviceID', 'Size', 'FreeSpace', 'DriveType') |
    Where-Object { $_.DriveType -eq 3 }) }
catch { Record-Error 'volumes' $_ }
try { $result.Physical = @(Query-Cim 'Win32_DiskDrive' @('Index', 'Model', 'Size')) }
catch { Record-Error 'physical' $_ }
try {
    $performance = [UPSmartHardwareDXGI]::Performance()
    $result.Performance = @($performance.Values.ToArray())
    foreach ($entry in $performance.Errors.GetEnumerator()) { $result.Errors[$entry.Key] = $entry.Value }
} catch {
    Record-Error 'disk_activity' $_
    Record-Error 'gpu_engines' $_
    Record-Error 'gpu_memory' $_
}
$result | ConvertTo-Json -Depth 6 -Compress
