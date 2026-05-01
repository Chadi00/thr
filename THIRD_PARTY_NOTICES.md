# Third-Party Notices

This file summarizes notable third-party software and model assets distributed
with or vendored by `thr`. It is informational and does not replace the
applicable license texts.

## Bundled Embedding Model

`thr` bundles the files needed to prepare the local semantic-search model cache
for `Qdrant/bge-base-en-v1.5-onnx-Q`, a quantized ONNX export derived from
`BAAI/bge-base-en-v1.5`.

- Source: <https://huggingface.co/Qdrant/bge-base-en-v1.5-onnx-Q>
- Base model: <https://huggingface.co/BAAI/bge-base-en-v1.5>
- Bundled revision: `738cad1c108e2f23649db9e44b2eab988626493b`
- License: Apache-2.0 for the Qdrant ONNX model repository; the BAAI base model
  is distributed under MIT license terms.

## fastembed-go

`thr` vendors a locally patched copy of `github.com/bdombro/fastembed-go` under
`third_party/fastembed-go`.

- Source: <https://github.com/bdombro/fastembed-go>
- Local patch notes: `third_party/fastembed-go/PATCHES.md`
- License: MIT; see `third_party/fastembed-go/LICENSE`.

## ONNX Runtime

Release archives include the ONNX Runtime shared library for the target
platform so semantic search works without a separate package-manager install.
The packaged runtime files are pinned in `native/onnxruntime.lock`.

- Source: <https://github.com/microsoft/onnxruntime>
- Version: `1.25.1`
- License files from ONNX Runtime, including `LICENSE`,
  `ThirdPartyNotices.txt`, and `Privacy.md`, are included alongside the
  packaged runtime library inside release archives.
