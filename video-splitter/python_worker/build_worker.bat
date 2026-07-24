@echo off
REM ============================================================================
REM  build_worker.bat — Đóng gói Python worker thành worker.exe bằng PyInstaller.
REM
REM  Kết quả: video-splitter\bin\worker.exe (đặt cạnh ffmpeg.exe/ffprobe.exe).
REM  Đây đúng là nơi Go GetWorkerExe() tìm, nên sau khi build xong app tự dùng
REM  worker.exe và người dùng cuối KHÔNG cần cài Python.
REM
REM  Cách dùng:
REM     1. (khuyên dùng) tạo venv:  python -m venv .venv && .venv\Scripts\activate
REM     2. cài phụ thuộc:           pip install -r requirements.txt
REM     3. chạy script này:         build_worker.bat
REM ============================================================================
setlocal

REM Thư mục chứa script này (python_worker\)
set "SCRIPT_DIR=%~dp0"
REM Thư mục gốc project (video-splitter\) = cha của python_worker\
for %%I in ("%SCRIPT_DIR%..") do set "PROJECT_ROOT=%%~fI"

set "DIST_DIR=%PROJECT_ROOT%\bin"
set "BUILD_DIR=%SCRIPT_DIR%build"
set "WORK_TMP=%SCRIPT_DIR%_pyi_build"

echo === Kiem tra PyInstaller ===
python -m PyInstaller --version >nul 2>&1
if errorlevel 1 (
    echo PyInstaller chua duoc cai. Dang cai...
    pip install pyinstaller || goto :error
)

echo === Kiem tra faster-whisper (phu de tu dong) ===
python -c "import faster_whisper" >nul 2>&1
if errorlevel 1 (
    echo faster-whisper chua duoc cai. Dang cai lan dau, co the mat vai phut...
    pip install faster-whisper==1.0.3 || goto :error
)

echo === Bat dau dong goi worker.exe ===
REM --onefile: 1 file duy nhat, de phan phoi (chap nhan khoi dong cham hon chut).
REM --collect-all: gom TAT CA data file + hidden import cho cac thu vien kho dong goi.
REM --console: giu console de doc stdout/stderr (Go worker giao tiep qua stdout).
python -m PyInstaller ^
    --onefile ^
    --console ^
    --name worker ^
    --distpath "%DIST_DIR%" ^
    --workpath "%WORK_TMP%" ^
    --specpath "%WORK_TMP%" ^
    --collect-all scenedetect ^
    --collect-all cv2 ^
    --collect-all librosa ^
    --collect-all numpy ^
    --collect-all scipy ^
    --collect-all soundfile ^
    --collect-all lazy_loader ^
    --collect-all soxr ^
    --collect-all faster_whisper ^
    --collect-all ctranslate2 ^
    --collect-all tokenizers ^
    --collect-all onnxruntime ^
    --hidden-import sklearn.utils._typedefs ^
    "%SCRIPT_DIR%main.py" || goto :error

echo.
echo === HOAN TAT ===
echo worker.exe da tao tai: %DIST_DIR%\worker.exe
echo Ban co the xoa thu muc tam: %WORK_TMP%
goto :end

:error
echo.
echo *** BUILD THAT BAI *** Xem log ben tren.
exit /b 1

:end
endlocal
