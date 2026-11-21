package ups

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	hid               = syscall.NewLazyDLL("hid.dll")
	cfgmgr            = syscall.NewLazyDLL("cfgmgr32.dll")
	kernel            = syscall.NewLazyDLL("kernel32.dll")
	getHIDGUID        = hid.NewProc("HidD_GetHidGuid")
	getAttributes     = hid.NewProc("HidD_GetAttributes")
	getProduct        = hid.NewProc("HidD_GetProductString")
	getManufacturer   = hid.NewProc("HidD_GetManufacturerString")
	getIndexed        = hid.NewProc("HidD_GetIndexedString")
	interfaceListSize = cfgmgr.NewProc("CM_Get_Device_Interface_List_SizeW")
	interfaceList     = cfgmgr.NewProc("CM_Get_Device_Interface_ListW")
	createMutex       = kernel.NewProc("CreateMutexW")
	queryProcessName  = kernel.NewProc("QueryFullProcessImageNameW")
	getPowerStatus    = kernel.NewProc("GetSystemPowerStatus")
)

type attributes struct {
	Size    uint32
	Vendor  uint16
	Product uint16
	Version uint16
}

type candidate struct {
	device Device
	path   string
}

type systemPowerStatus struct {
	ACLineStatus       byte
	BatteryFlag        byte
	BatteryLifePercent byte
	SystemStatusFlag   byte
	BatteryLifeTime    uint32
	BatteryFullTime    uint32
}

func AcquireOwner() (func(), error) {
	name, err := syscall.UTF16PtrFromString(`Global\UPSmart-Monitor`)
	if err != nil {
		return nil, err
	}
	handle, _, callErr := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, fmt.Errorf("create single-instance mutex: %w", callErr)
	}
	if errors.Is(callErr, syscall.ERROR_ALREADY_EXISTS) {
		syscall.CloseHandle(syscall.Handle(handle))
		return nil, errors.New("another UPSmart replacement instance is already running")
	}
	return func() { syscall.CloseHandle(syscall.Handle(handle)) }, nil
}

func CheckLegacy() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("identify monitoring executable: %w", err)
	}
	handle, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("check legacy UPSmart processes: %w", err)
	}
	defer syscall.CloseHandle(handle)
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	err = syscall.Process32First(handle, &entry)
	for err == nil {
		name := strings.ToLower(syscall.UTF16ToString(entry.ExeFile[:]))
		if name == "upsmart.exe" || name == "upsmart-ii.exe" {
			path, err := processImage(entry.ProcessID)
			if err != nil {
				return fmt.Errorf("cannot identify possible legacy UPSmart process %d: %w", entry.ProcessID, err)
			}
			if !strings.EqualFold(path, self) {
				same, err := sameImage(path, self)
				if err != nil {
					return fmt.Errorf("identify possible legacy UPSmart image: %w", err)
				}
				if !same {
					return errors.New("legacy UPSmart or a different monitor build is running; close it deliberately before enabling monitoring")
				}
			}
		}
		err = syscall.Process32Next(handle, &entry)
	}
	if !errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate processes: %w", err)
	}
	return nil
}

func sameImage(left, right string) (bool, error) {
	first, err := imageHash(left)
	if err != nil {
		return false, err
	}
	second, err := imageHash(right)
	if err != nil {
		return false, err
	}
	return first == second, nil
}

func imageHash(path string) ([32]byte, error) {
	var result [32]byte
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, 64<<20+1))
	if err != nil {
		return result, err
	}
	if n > 64<<20 {
		return result, errors.New("process image exceeds identity-check limit")
	}
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func processImage(id uint32) (string, error) {
	handle, err := syscall.OpenProcess(0x1000, false, id)
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(handle)
	var buffer [32768]uint16
	size := uint32(len(buffer))
	ok, _, callErr := queryProcessName.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return "", callErr
	}
	return syscall.UTF16ToString(buffer[:size]), nil
}

func openDevice(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	return syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
}

func openSessionDevice(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	access := uint32(syscall.GENERIC_READ | syscall.GENERIC_WRITE)
	share := uint32(0)
	flags := uint32(syscall.FILE_FLAG_OVERLAPPED)
	handle, err := syscall.CreateFile(p, access, share, nil, syscall.OPEN_EXISTING, flags, 0)
	if err == nil {
		return handle, nil
	}
	return syscall.CreateFile(p, access, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil,
		syscall.OPEN_EXISTING, flags, 0)
}

func deviceString(proc *syscall.LazyProc, handle syscall.Handle) (string, error) {
	var buf [256]uint16
	ok, _, err := proc.Call(uintptr(handle), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Sizeof(buf)))
	if ok == 0 {
		return "", fmt.Errorf("%s: %w", proc.Name, err)
	}
	return syscall.UTF16ToString(buf[:]), nil
}

func enumerate() ([]candidate, error) {
	var guid syscall.GUID
	getHIDGUID.Call(uintptr(unsafe.Pointer(&guid)))
	var paths []uint16
	for attempt := 0; attempt < 3; attempt++ {
		var count uint32
		code, _, _ := interfaceListSize.Call(uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&guid)), 0, 0)
		if code != 0 {
			return nil, fmt.Errorf("HID interface list size: CONFIGRET 0x%x", code)
		}
		if count < 2 {
			return nil, nil
		}
		if count > 1<<20 {
			return nil, errors.New("HID interface list exceeds safe size")
		}
		paths = make([]uint16, count)
		code, _, _ = interfaceList.Call(uintptr(unsafe.Pointer(&guid)), 0, uintptr(unsafe.Pointer(&paths[0])), uintptr(count), 0)
		if code == 0 {
			break
		}
		if code != 0x1a || attempt == 2 {
			return nil, fmt.Errorf("HID interface list: CONFIGRET 0x%x", code)
		}
	}
	var found []candidate
	for offset := 0; offset < len(paths) && paths[offset] != 0; {
		end := offset
		for end < len(paths) && paths[end] != 0 {
			end++
		}
		path := syscall.UTF16ToString(paths[offset:end])
		offset = end + 1
		if !strings.Contains(strings.ToLower(path), "vid_0001&pid_0000") {
			continue
		}
		item, err := inspect(path)
		if err != nil {
			return nil, err
		}
		found = append(found, item)
	}
	return found, nil
}

func inspect(path string) (candidate, error) {
	handle, err := openDevice(path)
	if err != nil {
		return candidate{}, fmt.Errorf("open candidate HID device: %w", err)
	}
	defer syscall.CloseHandle(handle)
	attr := attributes{Size: uint32(unsafe.Sizeof(attributes{}))}
	ok, _, callErr := getAttributes.Call(uintptr(handle), uintptr(unsafe.Pointer(&attr)))
	if ok == 0 {
		return candidate{}, fmt.Errorf("read HID attributes: %w", callErr)
	}
	if attr.Vendor != 1 || attr.Product != 0 {
		return candidate{}, errors.New("candidate HID attributes do not match expected VID/PID")
	}
	product, err := deviceString(getProduct, handle)
	if err != nil {
		return candidate{}, err
	}
	manufacturer, err := deviceString(getManufacturer, handle)
	var warnings []string
	if err != nil {
		if !errors.Is(err, syscall.Errno(87)) {
			return candidate{}, err
		}
		warnings = append(warnings, "Manufacturer descriptor unavailable: "+err.Error())
	}
	return candidate{Device{
		VendorID: "0001", ProductID: "0000", Product: product,
		Manufacturer: manufacturer, DescriptorWarnings: warnings,
		Capabilities: []string{},
	}, path}, nil
}

func indexedString(handle syscall.Handle, index uintptr, length int) (string, error) {
	response := make([]uint16, length)
	bufferBytes := uint32(len(response) * 2)
	ok, _, callErr := getIndexed.Call(uintptr(handle), index,
		uintptr(unsafe.Pointer(&response[0])), uintptr(bufferBytes))
	if ok == 0 {
		return "", fmt.Errorf("read indexed HID string %d: %w", index, callErr)
	}
	return syscall.UTF16ToString(response), nil
}

type nativeSession struct {
	handle      syscall.Handle
	device      Device
	phase       int
	initialized bool
}

func openNativeSession() (*nativeSession, error) {
	if err := CheckLegacy(); err != nil {
		return nil, err
	}
	items, err := enumerate()
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, fmt.Errorf("expected exactly one UPS candidate; found %d", len(items))
	}
	if items[0].device.Product != "MEC0003" {
		return nil, errors.New("unrecognized UPS product; refusing status query")
	}
	handle, err := openSessionDevice(items[0].path)
	if err != nil {
		return nil, fmt.Errorf("open selected UPS: %w", err)
	}
	return &nativeSession{handle: handle, device: items[0].device, initialized: true}, nil
}

func (s *nativeSession) Close() error {
	if !s.initialized {
		return nil
	}
	s.initialized = false
	return syscall.CloseHandle(s.handle)
}

func (s *nativeSession) Status() (Result, error) {
	result := Result{Devices: []Device{s.device}}
	indexes := [...]uintptr{3, 3, 3, 0x0d, 0xf3, 0x0c}
	phase := s.phase
	s.phase = (s.phase + 1) % len(indexes)
	statusText, err := indexedString(s.handle, indexes[phase], 50)
	if err != nil {
		return result, err
	}
	result.Devices[0].Capabilities = []string{"read_status_q1"}
	if phase >= 3 {
		result.RefreshOnly = true
		return result, nil
	}
	result.Status = normalizeHIDQ1(statusText)
	if _, err := ParseQ1(result.Status); err != nil {
		return result, fmt.Errorf("unsupported UPS status response: %w", err)
	}
	var power systemPowerStatus
	ok, _, _ := getPowerStatus.Call(uintptr(unsafe.Pointer(&power)))
	if ok != 0 {
		switch power.ACLineStatus {
		case 0:
			result.WindowsACLineState = "offline"
		case 1:
			result.WindowsACLineState = "online"
		default:
			result.WindowsACLineState = "unknown"
		}
	}
	return result, nil
}

func normalizeHIDQ1(status string) string {
	if strings.HasPrefix(status, "(") || len(status) < 2 {
		return status
	}
	candidate := "(" + status[1:]
	if _, err := ParseQ1(candidate); err == nil {
		return candidate
	}
	return status
}

func NativeOperation(status bool) (Result, error) {
	if status {
		session, err := openNativeSession()
		if err != nil {
			return Result{}, err
		}
		defer session.Close()
		return session.Status()
	}
	items, err := enumerate()
	if err != nil {
		return Result{}, err
	}
	result := Result{Devices: make([]Device, 0, len(items))}
	for _, item := range items {
		result.Devices = append(result.Devices, item.device)
	}
	return result, nil
}
