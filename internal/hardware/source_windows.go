//go:build windows

package hardware

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed collect_windows.ps1
var providerScript string

var systemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")
var processorGroups = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetActiveProcessorGroupCount")

func platformSources() sources {
	return sources{cpu: readCPUTimes, query: queryWindows}
}

func readCPUTimes(ctx context.Context) (cpuTimes, error) {
	if err := ctx.Err(); err != nil {
		return cpuTimes{}, err
	}
	groups, _, _ := processorGroups.Call()
	if groups > 1 {
		return cpuTimes{}, errors.New("GetSystemTimes covers only one processor group on this machine; whole-machine CPU utilization unavailable")
	}
	var idle, kernel, user windows.Filetime
	ok, _, err := systemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if ok == 0 {
		return cpuTimes{}, fmt.Errorf("GetSystemTimes: %w", err)
	}
	ticks := func(t windows.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
	return cpuTimes{idle: ticks(idle), kernel: ticks(kernel), user: ticks(user)}, nil
}

// limitedBuffer never retains more than limit bytes, including for stderr.
// Returning an error rather than discarding excess output rejects truncation.
type limitedBuffer struct {
	data     []byte
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - len(b.data)
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.overflow = true
		return remaining, errors.New("hardware provider output limit exceeded")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func queryWindows(ctx context.Context) ([]byte, error) {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	// Absolute system path and fixed arguments: no PATH search, user script,
	// interpolated command, profile, elevation, or execution-policy change.
	command := exec.CommandContext(ctx, filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe"),
		"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", providerScript)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED}
	command.WaitDelay = 200 * time.Millisecond
	stdout := &limitedBuffer{limit: maxProviderOutput}
	stderr := &limitedBuffer{limit: 16 * 1024}
	command.Stdout, command.Stderr = stdout, stderr

	// Closing the job kills any compiler/helper descendants as well as PS.
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("provider job: %w", err)
	}
	var closeOnce sync.Once
	closeJob := func() { closeOnce.Do(func() { _ = windows.CloseHandle(job) }) }
	defer closeJob()
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return nil, fmt.Errorf("provider job limits: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Windows provider: %w", err)
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		_ = windows.CloseHandle(process)
	}
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("contain Windows provider: %w", err)
	}
	// Start suspended so PowerShell cannot create a compiler/helper before it
	// belongs to the kill-on-close job.
	if err := resumeProvider(ctx, uint32(command.Process.Pid)); err != nil {
		closeJob()
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("resume Windows provider: %w", err)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			closeJob()
		case <-done:
		}
	}()
	err = command.Wait()
	close(done)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if stdout.overflow || stderr.overflow {
		return nil, errors.New("Windows provider exceeded bounded output")
	}
	if err != nil {
		// Avoid echoing raw PowerShell stderr (which can include script text).
		return nil, fmt.Errorf("Windows provider failed: %w", err)
	}
	return stdout.data, nil
}

func resumeProvider(ctx context.Context, pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	err = windows.Thread32First(snapshot, &entry)
	for count := 0; err == nil && count < 16384; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.OwnerProcessID == pid {
			thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if err != nil {
				return err
			}
			_, err = windows.ResumeThread(thread)
			_ = windows.CloseHandle(thread)
			return err
		}
		err = windows.Thread32Next(snapshot, &entry)
	}
	return errors.New("suspended provider thread unavailable within bounded thread enumeration")
}
