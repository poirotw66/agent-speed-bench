# AgentSpeedBench

使用 Go 測量 coding agent 的啟動延遲、有效 token 吞吐量、工具呼叫時間及任務正確性。

這個第一版已能執行，架構為 **Go CLI + YAML + JSONL + SQLite + HTML 報表**。參考 [HarnessBench](https://github.com/nyosegawa/harness-bench) 的相同任務比較方式，但目前未實作其 hidden tests 或案例格式相容性。完整設定與事件合約請見 [英文文件](README.md)。

## 快速開始

編譯需要 Go 1.26 以上，執行支援 macOS 與 Linux。

```sh
go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench
./bin/agentspeedbench run benchmarks/demo.yaml
```

離線示範會執行六次任務，驗證輸出及檔案內容，並產生 `runs/<experiment>/report.html`。示範 token 與事件是合成資料，不能當成 Codex 或其他服務的效能測量。編譯後的 binary 包含 SQLite，不需要另外安裝 SQLite。

要測試已安裝且已登入的 agent：

```sh
./bin/agentspeedbench doctor benchmarks/quick.yaml
./bin/agentspeedbench run -jobs 1 benchmarks/quick.yaml
./bin/agentspeedbench report -out runs/history.html
```

`doctor` 檢查設定、執行檔、版本及 repo commit；不會送出模型提示詞，也不驗證登入。`run` 會使用各 CLI 的帳號設定與額度。flags 必須放在 YAML 檔名之前。第一版開發驗證使用離線 fixture，尚未完成各廠商的實際登入執行測試。

## 已實作

- Codex、Claude Code、Cursor 的命令與事件 adapter。
- 自訂命令 adapter；Antigravity 範例需要填入已核實的 headless CLI。
- 重複與並行執行，每輪輪替 agent 順序。
- 每次使用獨立 workspace；有 repo 的案例只取指定 commit，不包含來源的未提交修改。
- 程序群組逾時／取消清理、stdout／stderr 原始紀錄及時間戳。
- 輸出斷言與命令驗證；未配置驗證的成功執行標示為未評分。
- SQLite 歷史資料、JSONL 原始事件、HTML 報表及報表重建。

各 agent 的預設權限保護會保留。Workspace 隔離用於保護來源 checkout，不是主機安全沙箱；repo 內的測試仍可能被 agent 修改，不能當成防竄改的 hidden tests。

## 吞吐量的界線

| 指標 | 意義 |
| --- | --- |
| TTFA | 啟動到第一個觀察到的 assistant 輸出或 tool start；不包含初始化 metadata。Codex 訊息可能已緩衝。 |
| Effective output tok/s | agent 回報的 output tokens ÷ 程序總時間，包含啟動、等待與工具時間。 |
| Generation / model-active tok/s | CLI 沒有可靠的生成／模型活動區間，目前顯示 `unknown`。 |
| Tool latency | 有配對 ID 的工具開始到完成時間，包含 harness overhead。 |
| Wall p50 / p95 | 包含失敗與逾時的程序時間，使用 nearest-rank percentile。 |
| Pass / graded | 通過驗證的任務 ÷ 結果已知的任務；未評分保持未知。 |
| Passed / wall hour | 依程序時間換算的序列等效通過量，不是並行實驗的實際每小時完成量。 |

缺少的數值保留 JSON `null`、SQL `NULL` 及報表 `unknown`；真正回報的零值才顯示零。Cursor 官方輸出格式沒有可靠的 token usage，因此不估算 tokens。各廠商的 tokenizer 與 usage 計費定義也不一定相同。

報表依「實驗、agent、案例」分組，不會把不同實驗設定混成一個平均值。要做前後比較，請固定模型、repo commit、權限、CLI 版本及並行數，並核對 `manifest.json`。目前不能僅憑這些數據判定是模型推論、網路、服務排隊或 orchestration 造成變慢。

## 設定與驗證

`benchmarks/quick.yaml` 可直接用於簡單回應測試。`repo-debug.example.yaml` 與 `antigravity.example.yaml` 是需要填寫的範例，不能直接當成已驗證的 benchmark。

設定中的 repo 路徑、含 `/` 的執行檔路徑，相對於 YAML 所在目錄；一般執行檔名稱從 `PATH` 尋找。Verifier 的 arguments 相對於臨時 workspace。`output_contains` 是 substring smoke check，並不證明語意或程式正確性；需要較強的評分時，配置可信的外部 verifier。

```sh
make check
```

檢查包含格式、`go vet`、race-enabled tests 及編譯。測試涵蓋 telemetry 解析、未知與零值、逾時及子程序清理、驗證失敗、workspace 隔離、來源未提交修改保留、HTML escaping 與 SQLite 儲存。CI 已配置 macOS／Linux；本機通過不代表 hosted CI 或廠商服務已驗證。

原始紀錄及設定快照可能包含任務私有內容，預設只寫入本機且不會上傳。Workspace 完成後會刪除，目前保留的是 telemetry 與驗證紀錄，未保留產出的 patch。Docker 隔離、可防竄改的 hidden tests、崩潰復原、真正生成區間量測及更完整的趨勢 dashboard 是後續工作。
