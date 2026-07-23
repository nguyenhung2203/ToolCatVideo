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

	// PDH (Performance Data Helper) để đọc bộ đếm "GPU Engine" của Windows — cách
	// duy nhất lấy % GPU theo tiến trình mà không cần driver hãng. Query giữ mở
	// bền giữa các lần lấy mẫu vì "Utilization Percentage" cần 2 mẫu mới ra số.
	modpdh                          = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery                = modpdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounter        = modpdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData         = modpdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterArray = modpdh.NewProc("PdhGetFormattedCounterArrayW")

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

	// Query PDH cho GPU: khởi tạo 1 lần, giữ mở suốt vòng đời tiến trình.
	gpuQueryOnce    sync.Once
	gpuQueryHandle  uintptr
	gpuCounter      uintptr
	gpuQueryReady   bool
)

// Hằng PDH.
const (
	pdhFmtDouble  = 0x00000200
	pdhMoreData   = 0x800007D2
	pdhInvalidVal = 0x800007D8 // PDH_CALC_NEGATIVE_DENOMINATOR... bỏ qua instance lỗi
)

// PDH_FMT_COUNTERVALUE_DOUBLE: giá trị double + trạng thái. Có 4 byte đệm sau
// CStatus để double thẳng hàng 8 byte (union trong C căn theo double/LONGLONG).
type pdhFmtCounterValueDouble struct {
	CStatus     uint32
	_           uint32
	DoubleValue float64
}

// PDH_FMT_COUNTERVALUE_ITEM_W: một phần tử trong mảng trả về của
// PdhGetFormattedCounterArray — tên instance (con trỏ WCHAR) + giá trị. Trên
// 64-bit con trỏ 8 byte nên FmtValue (bắt đầu bằng double) đã thẳng hàng, không
// cần đệm thêm.
type pdhFmtCounterValueItemDouble struct {
	SzName   *uint16
	FmtValue pdhFmtCounterValueDouble
}

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

		// Tập PID của Tool (chính + con/cháu + binary công cụ) — dùng cả cho đo GPU.
		appPids := make(map[uint32]bool)
		// Tổng thời gian CPU (kernel+user, đơn vị 100ns) của từng tiến trình Tool
		// tại lần lấy mẫu này. So với lần trước để ra % CPU tiêu tốn thật.
		curProcCPU := make(map[uint32]uint64)

		for _, item := range procList {
			// Chỉ đo nếu tiến trình là bản thân Tool, là con/cháu do Tool khởi chạy, hoặc là binary công cụ ffmpeg/yt-dlp
			isAppProc := isAppDescendant(item.pid, mainPid, parentMap) || toolBinaries[item.exeName]

			if isAppProc {
				appPids[item.pid] = true
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

					// Thời gian CPU của tiến trình: kernel + user (bỏ creation/exit).
					var ctime, etime, ktime, utime windows.Filetime
					rT, _, _ := procGetProcessTimes.Call(
						uintptr(hProc),
						uintptr(unsafe.Pointer(&ctime)),
						uintptr(unsafe.Pointer(&etime)),
						uintptr(unsafe.Pointer(&ktime)),
						uintptr(unsafe.Pointer(&utime)),
					)
					if rT != 0 {
						cpu := filetimeToUint64(ktime) + filetimeToUint64(utime)
						curProcCPU[item.pid] = cpu
					}
					windows.CloseHandle(hProc)
				}
			}
		}

		res.AppRamMB = float64(totalWorkingSet) / (1024.0 * 1024.0)

		// CPU riêng của Tool: so tổng thời gian CPU đã tiêu giữa 2 lần lấy mẫu với
		// thời gian thực đã trôi qua, rồi chia số nhân logic để ra % của CẢ máy
		// (giống Task Manager: 100% = full 1 máy, không phải full 1 nhân).
		now := time.Now()
		if lastProcCPU != nil && !lastAppSampleTime.IsZero() {
			// Chỉ cộng delta của PID còn tồn tại ở cả 2 lần (PID mới coi như bắt đầu
			// từ 0; PID đã chết thì bỏ, không tính âm).
			var deltaCPU uint64
			for pid, cur := range curProcCPU {
				if prev, ok := lastProcCPU[pid]; ok && cur >= prev {
					deltaCPU += cur - prev
				}
			}
			elapsed := now.Sub(lastAppSampleTime).Seconds()
			nCPU := runtime.NumCPU()
			if elapsed > 0 && nCPU > 0 {
				// deltaCPU tính theo đơn vị 100ns → đổi ra giây: *1e-7.
				busySec := float64(deltaCPU) * 1e-7
				pct := busySec / (elapsed * float64(nCPU)) * 100.0
				if pct < 0 {
					pct = 0
				}
				if pct > 100 {
					pct = 100
				}
				res.AppCpuPercent = pct
			}
		}
		lastProcCPU = curProcCPU
		lastAppSampleTime = now

		// GPU riêng của Tool: cộng utilization các engine GPU của những PID thuộc Tool.
		res.GpuPercent = sampleAppGpuPercent(appPids)

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

// initGpuQuery mở PDH query cho bộ đếm "GPU Engine(*)\Utilization Percentage"
// (dùng tên tiếng Anh nên không phụ thuộc ngôn ngữ Windows) và lấy mẫu đầu tiên
// để "mồi" bộ đếm. Chỉ chạy 1 lần. Nếu máy không có PDH/GPU counter thì
// gpuQueryReady = false và mọi lần đo GPU sau trả 0 (không crash).
func initGpuQuery() {
	path, err := windows.UTF16PtrFromString(`\GPU Engine(*)\Utilization Percentage`)
	if err != nil {
		return
	}
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&gpuQueryHandle))); r != 0 {
		return
	}
	if r, _, _ := procPdhAddEnglishCounter.Call(
		gpuQueryHandle, uintptr(unsafe.Pointer(path)), 0,
		uintptr(unsafe.Pointer(&gpuCounter)),
	); r != 0 {
		return
	}
	// Mẫu đầu tiên: "Utilization Percentage" là bộ đếm dạng tỉ lệ, cần 2 mẫu mới
	// tính được → lần collect này chỉ để mồi, chưa lấy giá trị.
	procPdhCollectQueryData.Call(gpuQueryHandle)
	gpuQueryReady = true
}

// sampleAppGpuPercent trả về tổng % sử dụng GPU của riêng các tiến trình Tool
// (theo appPids). Đọc mảng instance của "GPU Engine", mỗi instance có tên dạng
// "pid_1234_luid_..._engtype_3D"; ta cộng utilization của instance có pid thuộc
// Tool. Cùng 1 pid có thể có nhiều engine (3D, Copy, VideoDecode...) nên cộng
// hết rồi kẹp trần 100%. Trả 0 nếu PDH không sẵn sàng hoặc chưa đủ mẫu.
func sampleAppGpuPercent(appPids map[uint32]bool) float64 {
	gpuQueryOnce.Do(initGpuQuery)
	if !gpuQueryReady {
		return 0
	}

	// Lấy mẫu mới. Cần lần collect này để có delta so với mẫu trước.
	if r, _, _ := procPdhCollectQueryData.Call(gpuQueryHandle); r != 0 {
		return 0
	}

	// Bước 1: hỏi kích thước buffer cần thiết (gọi với buffer rỗng → PDH_MORE_DATA).
	var bufSize, itemCount uint32
	r, _, _ := procPdhGetFormattedCounterArray.Call(
		gpuCounter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&bufSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		0,
	)
	if r != pdhMoreData || bufSize == 0 || itemCount == 0 {
		return 0
	}

	// Bước 2: cấp buffer đúng cỡ rồi lấy mảng instance thật.
	buf := make([]byte, bufSize)
	r, _, _ = procPdhGetFormattedCounterArray.Call(
		gpuCounter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&bufSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if r != 0 {
		return 0
	}

	items := unsafe.Slice((*pdhFmtCounterValueItemDouble)(unsafe.Pointer(&buf[0])), int(itemCount))
	var total float64
	for i := range items {
		if items[i].SzName == nil {
			continue
		}
		name := windows.UTF16PtrToString(items[i].SzName)
		pid := pidFromGpuInstance(name)
		if pid == 0 || !appPids[pid] {
			continue
		}
		if items[i].FmtValue.CStatus != 0 {
			continue // instance lỗi/không đủ mẫu → bỏ
		}
		total += items[i].FmtValue.DoubleValue
	}
	if total < 0 {
		total = 0
	}
	if total > 100 {
		total = 100
	}
	return total
}

// pidFromGpuInstance rút PID từ tên instance GPU Engine, dạng
// "pid_1234_luid_0x...". Trả 0 nếu không parse được.
func pidFromGpuInstance(name string) uint32 {
	const prefix = "pid_"
	if !strings.HasPrefix(name, prefix) {
		return 0
	}
	rest := name[len(prefix):]
	end := strings.IndexByte(rest, '_')
	if end < 0 {
		end = len(rest)
	}
	var pid uint32
	if _, err := fmt.Sscanf(rest[:end], "%d", &pid); err != nil {
		return 0
	}
	return pid
}
