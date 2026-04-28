# fastembed-go local patch

This local replacement keeps `thr` independent from Homebrew by adding
`InitOptions.ONNXRuntimeLibraryPath` and removing macOS Homebrew probing from
model initialization. Upstream can replace this fork once it supports explicit
ONNX Runtime library paths without auto-detecting package-manager locations.
