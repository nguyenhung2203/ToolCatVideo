package sysmonitor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type SystemStats struct {
	AppRamMB      float64 `json:"appRamMB"`      // RAM của tool & các tiến trình con (MB)
	SysRamPercent float64 `json:"sysRamPercent"` // RAM tổng hệ thống (%)
	AppCpuPercent float64 `json:"appCpuPercent"` // CPU RIÊNG của tool & tiến trình con (%)
	SysCpuPercent float64 `json:"sysCpuPercent"` // CPU tổng hệ thống (%)
	GpuPercent    float64 `json:"gpuPercent"`    // GPU RIÊNG của tool & tiến trình con (%)
	ActiveTasks   string  `json:"activeTasks"`   // Danh sách tiến trình tác vụ đang chạy
}

var (
	modkernel32              = windows.NewLazySystemDLL("kernel32.dll")
	modpsapi                 = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo = modpsapi.NewProc("GetProcessMemoryInfo")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemTimes       = modkernel32.NewProc("GetSystemTimes")
	procGetProcessTimes      = modkernel32.NewProc("GetProcessTimes")

	lastKernelTime uint64
	lastUserTime   uint64
	lastIdleTime   uint64
	lastTime       time.Time
	statsLock      sync.Mutex

	// Đo CPU riêng của tool theo delta thời gian CPU từng tiến trình. Giữ tổng
	// thời gian CPU (kernel+user, đơn vị 100ns) theo PID giữa 2 lần lấy mẫu để
	// tính phần CPU thực sự tiêu tốn trong khoảng đó, chia cho số nhân logic.
	lastProcCPU       map[uint32]uint64
	lastAppSampleTime time.Time
)

type PROCESS_MEMORY_COUNTERS struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

type MEMORYSTATUSEX struct {
	Length                  uint32
	MemoryLoad              uint32
	TotalPhys               uint64
	AvailPhys               uint64
	TotalPageFile           uint64
	AvailPageFile           uint64
	TotalVirtual            uint64
	AvailVirtual            uint64
	AvailExtendedVirtual    uint64
}

// Danh sách các công cụ nền đặc thù của ứng dụng
func getToolBinaries() map[string]bool {
	mainExe := strings.ToLower(filepath.Base(os.Args[0]))
	return map[string]bool{
		mainExe:                  true,
		"smartvideosplitter.exe": true,
		"traffictool.exe":        true,
		"video-splitter.exe":     true,
		"ffmpeg.exe":             true,
		"ffprobe.exe":            true,
		"worker.exe":             true,
		"yt-dlp.exe":             true,
	}
}

// Kiểm tra pid có phải là con/cháu của mainPid hay không
func isAppDescendant(pid uint32, mainPid uint32, parentMap map[uint32]uint32) bool {
	curr := pid
	for i := 0; i < 10; i++ {
		if curr == mainPid {
			return true
		}
		parent, ok := parentMap[curr]
		if !ok || parent == 0 || parent == curr {
			break
		}
		curr = parent
	}
	return false
}

func GetStats() SystemStats {
	statsLock.Lock()
	defer statsLock.Unlock()

	res := SystemStats{}

	// 1. RAM tổng hệ thống (%)
	var memStatus MEMORYSTATUSEX
	memStatus.Length = uint32(unsafe.Sizeof(memStatus))
	r1, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if r1 != 0 {
		res.SysRamPercent = float64(memStatus.MemoryLoad)
	}

	// 2. CPU tổng hệ thống (%) tính theo thời gian thực
	var idle, kernel, user windows.Filetime
	rCpu, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if rCpu != 0 {
		idle64 := filetimeToUint64(idle)
		kernel64 := filetimeToUint64(kernel)
		user64 := filetimeToUint64(user)

		if lastKernelTime > 0 {
			kernelDiff := kernel64 - lastKernelTime
			userDiff := user64 - lastUserTime
			idleDiff := idle64 - lastIdleTime
			totalSys := kernelDiff + userDiff
			if totalSys > 0 {
				usedSys := totalSys - idleDiff
				res.SysCpuPercent = (float64(usedSys) / float64(totalSys)) * 100.0
				if res.SysCpuPercent < 0 {
					res.SysCpuPercent = 0
				}
				if res.SysCpuPercent > 100 {
					res.SysCpuPercent = 100
				}
			}
		}
		lastKernelTime = kernel64
		lastUserTime = user64
		lastIdleTime = idle64
	}

	// 3. Đo RAM và Tiến trình của Tool + Tiến trình con (không tính Chrome ngoài của người dùng)
	toolBinaries := getToolBinaries()
	mainPid := uint32(os.Getpid())

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err == nil {
		defer windows.CloseHandle(snap)
		var pe windows.ProcessEntry32
		pe.Size = uint32(unsafe.Sizeof(pe))

		type procItem struct {
			pid       uint32
			parentPid uint32
			exeName   string
		}
		var procList []procItem
		parentMap := make(map[uint32]uint32)

		if windows.Process32First(snap, &pe) == nil {
			for {
				exeName := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
				procList = append(procList, procItem{
					pid:       pe.ProcessID,
					parentPid: pe.ParentProcessID,
					exeName:   exeName,
				})
				parentMap[pe.ProcessID] = pe.ParentProcessID

				if windows.Process32Next(snap, &pe) != nil {
					break
				}
			}
		}

		taskCounts := make(map[string]int)
		var totalWorkingSet uintptr

		for _, item := range procList {
			// Chỉ đo nếu tiến trình là bản thân Tool, là con/cháu do Tool khởi chạy, hoặc là binary công cụ ffmpeg/yt-dlp
			isAppProc := isAppDescendant(item.pid, mainPid, parentMap) || toolBinaries[item.exeName]

			if isAppProc {
				hProc, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, item.pid)
				if err == nil {
					var memCounters PROCESS_MEMORY_COUNTERS
					memCounters.CB = uint32(unsafe.Sizeof(memCounters))
					rMem, _, _ := procGetProcessMemoryInfo.Call(
						uintptr(hProc),
						uintptr(unsafe.Pointer(&memCounters)),
						uintptr(memCounters.CB),
					)
					if rMem != 0 {
						totalWorkingSet += memCounters.WorkingSetSize
						taskCounts[item.exeName]++
					}
					windows.CloseHandle(hProc)
				}
			}
		}

		res.AppRamMB = float64(totalWorkingSet) / (1024.0 * 1024.0)

		var activeParts []string
		for name, count := range taskCounts {
			cleanName := name
			if strings.HasSuffix(cleanName, ".exe") {
				cleanName = strings.TrimSuffix(cleanName, ".exe")
			}
			activeParts = append(activeParts, fmt.Sprintf("%s (%d)", cleanName, count))
		}
		res.ActiveTasks = strings.Join(activeParts, ", ")
	}

	return res
}

func filetimeToUint64(ft windows.Filetime) uint64 {
	return (uint64(ft.HighDateTime) << 32) | uint64(ft.LowDateTime)
}
