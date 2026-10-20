# Vendored go-echarts

- Upstream: https://github.com/go-echarts/go-echarts
- Tag: v2.7.3
- Commit: a154e551b39da7eaae534c9fdcff009c99d97f56 (Sun Sep 13 14:20:18 2026 +0800)
- License: Apache-2.0 (see LICENSE)
- Modifications: tests, docs, go.sum and upstream .github workflows removed; go.mod reduced to the module line
  (upstream's only requirement, stretchr/testify, is test-only). No source changes.
- Consumed via `replace github.com/go-echarts/go-echarts/v2 => ./third_party/go-echarts`
  so builds are reproducible offline. To track upstream instead, delete the replace line.
