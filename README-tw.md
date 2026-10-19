# AgentSpeedBench

[![Go checks](https://github.com/poirotw66/agent-speed-bench/actions/workflows/ci.yml/badge.svg)](https://github.com/poirotw66/agent-speed-bench/actions/workflows/ci.yml)

[English](README.md) · [繁體中文](README-tw.md)

以 Go CLI 測量 coding agent 的延遲、回報的 token 吞吐量、工具間隔，以及端到端正確性。

[結果](docs/results.md) · [使用與方法](docs/guide.md) · [量測計畫](docs/measurement-remediation-plan.md)

靈感來自 [HarnessBench](https://github.com/nyosegawa/harness-bench)；為獨立實作（YAML、本地 repo 快照、JSONL、SQLite、獨立 HTML 報表），不需要網頁服務。

![AgentSpeedBench 離線示範 HTML 報表](docs/images/report-demo.png)

## 重點結果

序列 100 / 500 整數輸出重測（2026-10-11）。Harness commit `b2d2c674cd3754f1ae00a1e86145580cd8bae744`，`jobs: 1`，每格 **n=3**（小樣本；僅供描述）。Codex／Cursor 使用隔離設定；agy 使用原生登入 HOME。有效 tok/s = 回報的輸出 token／完整行程牆鐘時間——不是純生成速度。完整表格、平行執行與限制說明：[docs/results.md](docs/results.md)。

| 請求的設定 | 100 行牆鐘 p50（秒） | 100 行有效 tok/s p50 | 500 行牆鐘 p50（秒） | 500 行有效 tok/s p50 |
| --- | ---: | ---: | ---: | ---: |
| GPT-6.1 Sol / medium | 9.76 | 21.20 | 19.39 | 51.92 |
| GPT-6 Luna / high | 7.69 | 29.64 | 12.41 | 83.51 |
| GPT-6 Luna / high / 請求的 fast 層級 | 7.26 | 30.71 | 13.81 | 76.18 |
| Cursor Auto | 15.52 | 16.36 | 20.19 | 52.26 |
| AGY Gemini-3.8-Flash / low | 7.08 | 41.82 | 16.29 | 116.36 |
| AGY Gemini-3.8-Flash / medium | 6.21 | 47.66 | 13.73 | 138.13 |
| AGY Gemini-3.8-Flash / high | 8.48 | 51.30 | 15.19 | 145.93 |

全部 **42** 筆量測樣本與 **14** 次 warmup 皆通過。AGY 輸出計帳含 thinking；Codex／Cursor 計帳方式未知。各 adapter 的 TTFA 計時基準不同，不可互換。

## 快速開始

需要 Go 1.26+；支援 macOS 與 Linux。

```sh
go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench
./bin/agentspeedbench run benchmarks/demo.yaml
```

離線示範會執行六次合成嘗試，並寫入 `runs/<experiment>/report.html`（上方截圖）。時間與 token 是 **harness 用的 fixture**，不是任何 AI 服務的實測。Binary 內建 SQLite。

```sh
./bin/agentspeedbench doctor benchmarks/quick.yaml
./bin/agentspeedbench run -jobs 1 benchmarks/quick.yaml
make check
make demo
```

真實的 `run` 會使用已安裝的 agent CLI，並可能消耗額度。請先分別完成各 CLI 登入。Flags 必須放在 YAML 路徑之前。細節：[docs/guide.md](docs/guide.md)。

## 架構

```text
cmd/agentspeedbench    CLI 與離線示範
internal/benchmark     嚴格設定與任務合約
internal/adapters      各廠商命令／事件差異
internal/runner        排程、工作區、行程、評分
internal/telemetry     正規化事件與指標
internal/storage       SQLite 執行紀錄與 JSON 產物
internal/report        統計與獨立 HTML
internal/responses     僅回應的 API 串流 adapter
```

Adapters：`codex`、`claude`、`cursor`、`agy`、`generic`、`demo`、`openai-responses`。指標、YAML schema、驗證器與 resume／復原：[docs/guide.md](docs/guide.md)。

## 文件

| 文件 | 內容 |
| --- | --- |
| [docs/results.md](docs/results.md) | 原生量測表格、commit 釘選、樣本數、鑰匙圈說明 |
| [docs/guide.md](docs/guide.md) | CLI 參考、adapter、指標、YAML、產物、真實 Go 案例 |
| [docs/measurement-remediation-plan.md](docs/measurement-remediation-plan.md) | 範圍邊界與驗證紀錄 |

工程 fixture 使用獨立撰寫的測試。真實 Go 案例採用 [lazygit PR #5495](https://github.com/jesseduffield/lazygit/pull/5495)；上游 lazygit 為 MIT 授權。

## 授權

MIT © 2026 poirotw66。見 [LICENSE](LICENSE)。
