# Selective extraction measurements

Measured on 2026-09-17 using Go 1.25.4, Windows amd64, Intel Core Ultra 7 155H.
The input contains 2,000 text-show operations (20 characters each) and 2,000
rectangle painting operations on one page. See `BenchmarkExtractDense` in
[extract_bench_test.go](../extract_bench_test.go).

```powershell
go test . -run '^$' -bench '^BenchmarkExtractDense$' -benchmem -benchtime=200ms -count=6
```

| Mode | Median allocated bytes/op | Median allocations/op | JSON bytes |
| --- | ---: | ---: | ---: |
| Legacy detailed analysis + basic projection | 47070972.5 | 166618.5 | 1902028 |
| ExtractReader, default Unicode text | 2532830 | 66494 | 214198 |
| ExtractReader, all content + positions/styles/glyphs | 23109828 | 154520 | 7764212 |

On this fixture, requesting only Unicode text reduces total allocated bytes by
94.6% and JSON bytes by 88.7% relative to the legacy basic result.
These modes intentionally produce different fields. All-content extraction
still allocates less than the legacy path, but its JSON is larger because the
benchmark explicitly requests per-glyph output that legacy basic JSON omits.

`B/op` measures total allocation during a call, not peak live heap, retained
heap, or process RSS. Input snapshots and decoded caches still exist during
extraction. These are synthetic workload measurements, not guarantees for all
PDFs. Timing is omitted because other verification work shared the machine;
this report makes no throughput claim.
