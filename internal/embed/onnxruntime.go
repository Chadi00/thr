package embed

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	ONNXRuntimeVersion = "1.25.1"
	onnxRuntimeEnvVar  = "THR_ONNXRUNTIME_LIB"
)

func resolveONNXRuntimeLibrary() (string, error) {
	if override := os.Getenv(onnxRuntimeEnvVar); override != "" {
		return verifyONNXRuntimeLibrary(override, "explicit "+onnxRuntimeEnvVar)
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate thr executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	lib := filepath.Clean(filepath.Join(
		filepath.Dir(exe),
		"..",
		"lib",
		"thr",
		"onnxruntime",
		ONNXRuntimeVersion,
		runtimeTarget(),
		onnxRuntimeLibraryName(),
	))
	return verifyONNXRuntimeLibrary(lib, "packaged ONNX Runtime")
}

func verifyONNXRuntimeLibrary(path string, source string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s library not found at %s: %w. Reinstall thr or set %s to the ONNX Runtime shared library path", source, path, err, onnxRuntimeEnvVar)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s library path is a directory: %s", source, path)
	}
	return path, nil
}

func runtimeTarget() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

func onnxRuntimeLibraryName() string {
	return onnxRuntimeLibraryNameForGOOS(runtime.GOOS)
}

func onnxRuntimeLibraryNameForGOOS(goos string) string {
	switch goos {
	case "darwin":
		return "libonnxruntime.dylib"
	case "linux":
		return "libonnxruntime.so"
	case "windows":
		return "onnxruntime.dll"
	default:
		return "onnxruntime.so"
	}
}
