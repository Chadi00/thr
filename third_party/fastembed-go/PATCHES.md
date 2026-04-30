# fastembed-go local patch

This local replacement keeps `thr` independent from Homebrew by adding
`InitOptions.ONNXRuntimeLibraryPath` and removing macOS Homebrew probing from
model initialization. It also removes upstream model downloads so `thr` only
uses the pinned, hash-verified model cache prepared from bundled assets.
Upstream can replace this fork once it supports explicit ONNX Runtime library
paths without auto-detecting package-manager locations and without an unpinned
network fallback.
