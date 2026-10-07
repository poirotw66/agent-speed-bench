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

`doctor` 檢查設定、執行檔、版本及 repo commit；不會送出模型提示詞，也不驗證登入。`run` 會使用各 CLI 的帳號設定與額度。flags 必須放在 YAML 檔名之前。已在本機執行 Codex 登入實測；Cursor Auto 與 agy Gemini 3.8 Flash 也已完成本機實測；Claude 尚未完成實際登入驗證。

## 已實作

- Codex、Claude Code、Cursor、agy 的命令與事件 adapter。
- 自訂命令 adapter；Antigravity 範例直接使用原生 agy CLI。
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
| Tool receipt interval | 配對事件的接收間隔，可能受 CLI 緩衝影響；實際工具執行耗時保持未知。 |
| Wall p50 / p95 | 包含失敗與逾時的程序時間，使用 nearest-rank percentile。 |
| Pass / graded | 通過驗證的任務 ÷ 結果已知的任務；未評分保持未知。 |
| Passed / wall hour | 依程序時間換算的序列等效通過量，不是並行實驗的實際每小時完成量。 |

缺少的數值保留 JSON `null`、SQL `NULL` 及報表 `unknown`；真正回報的零值才顯示零。Cursor 會讀取原生終端結果的 camelCase usage；沒有回報時才保持未知。各廠商的 tokenizer 與 usage 計費定義也不一定相同。

報表依「實驗、agent、案例、TTFA 時間基準、output token 計數定義」分組，不會把不同實驗設定混成一個平均值。要做前後比較，請固定模型、repo commit、權限、CLI 版本及並行數，並核對 `manifest.json`。目前不能僅憑這些數據判定是模型推論、網路、服務排隊或 orchestration 造成變慢。

## 設定與驗證

`benchmarks/quick.yaml` 可直接用於簡單回應測試。`repo-debug.example.yaml` 需要填寫 repo；`antigravity.example.yaml` 使用原生 agy，仍需可用模型與登入。`native-comparison.yaml` 提供七組模型／設定的序列測試。

設定中的 repo 路徑、含 `/` 的執行檔路徑，相對於 YAML 所在目錄；一般執行檔名稱從 `PATH` 尋找。Verifier 的 arguments 相對於臨時 workspace。`output_contains` 是 substring smoke check，並不證明語意或程式正確性；需要較強的評分時，配置可信的外部 verifier。

```sh
make check
```

檢查包含格式、`go vet`、race-enabled tests 及編譯。測試涵蓋 telemetry 解析、未知與零值、逾時及子程序清理、驗證失敗、workspace 隔離、來源未提交修改保留、HTML escaping 與 SQLite 儲存。CI 已配置 macOS／Linux；本機通過不代表 hosted CI 或廠商服務已驗證。

原始紀錄及設定快照可能包含任務私有內容，預設只寫入本機且不會上傳。Workspace 完成後會刪除，目前保留的是 telemetry 與驗證紀錄，未保留產出的 patch。Docker 隔離、可防竄改的 hidden tests、崩潰復原、真正生成區間量測及更完整的趨勢 dashboard 是後續工作。

## 實測後修正

- 工具的 `tool_receipt_interval_*` 是 runner 接收到配對事件的間隔。它不代表工具執行耗時；沒有權威執行時間時，`tool_latency_*` 保持未知。HTML 會顯示微秒／毫秒，舊紀錄的工具間隔也會重新標示，但不改寫歷史資料。
- TTFA 記錄 `ttfa_timing_basis`，並分別保留第一個文字 delta、完整訊息與工具動作的接收時間。報告不混合不同時間基準，Codex 完整訊息仍可能受 CLI 緩衝影響。
- 巢狀錯誤保留原因、分類、HTTP 狀態、scope 與 retryability。不支援的模型或認證錯誤會停止該 agent 後續排程並留下 `skipped` 紀錄；其他 agent 繼續。已啟動的並行工作可能完成。`agent_unavailable`／`service_error`／`infrastructure_error`／`telemetry_error`／`skipped` 不計入任務正確率，暫時性服務錯誤不會停止重複測試。
- Codex 可設定 `reasoning_effort: high`，透過 CLI 覆寫 `model_reasoning_effort`。每次執行分開記錄 requested、configured、observed model／effort。只擷取使用者 TOML 中允許的 model、effort、service tier 與 fast_mode 欄位，不保存秘密；這不是完整設定層解析。CLI 未明確回報時，observed 保持未知。[設定文件](https://learn.chatgpt.com/docs/developer-settings)。
- `verify.output_equals` 驗證逐位元組一致；`verify.integer_sequence` 驗證完整數字序列與結束標記。`benchmarks/quick.yaml` 已改成完整驗證，缺漏、重複、順序錯誤、額外文字或只輸出標記都不會通過。所有已設定的驗證條件必須同時通過。

```yaml
agents:
  - name: codex
    adapter: codex
    model: YOUR_EXPLICIT_MODEL_ID
    reasoning_effort: high
cases:
  - name: enumerate
    prompt: Print integers 1 through 100, one per line, then BENCH_DONE.
    verify:
      integer_sequence:
        start: 1
        end: 100
        end_marker: BENCH_DONE
```

序列驗證允許 LF／CRLF 與一次可選的末尾換行。設定快照只讀取 `$CODEX_HOME/config.toml` 或 `~/.codex/config.toml`，不解析 project、managed 或 profile 設定。舊紀錄缺乏結構化錯誤時，原本的失敗評分無法自動還原。

## Cursor、agy 與 Fast 修正

- Cursor 讀取 input／output／cache-read／cache-write tokens，保留缺值與零值的差別。`trust_workspace` 預設關閉；明確設為 `true` 才加入 `--trust`。stderr 的 workspace trust 錯誤只有在程序非零退出時才視為不可用，後續排程跳過且不評分；一般警告不影響成功結果。
- agy 直接使用原生 `stream-json`，解析 metadata、文字 delta、工具配對與最終結果；只有 terminal `SUCCESS` 才可完成。最終 response 用於驗證，最終 usage 覆蓋總量，避免重複計數。
- Thinking tokens 與 cache-write tokens 分開保存。agy 的 output tokens 包含 thinking；報表明確顯示計數定義。CLI 回報的 duration 與程序 wall time 分開，不能當成純生成區間或可見文字速度。
- Codex 的 `service_tier: fast` 會加入 tier override 與 `--enable fast_mode`；requested、configured、observed 分開記錄。成功請求不證明實際 Fast tier，也不保證 1.5 倍。CLI 未回報時 observed 保持未知。[Fast 設定文件](https://learn.chatgpt.com/docs/agent-configuration/speed?site_variant=chatgpt)。

歷史 run 不改寫；新欄位需重新測試才能取得。七組完整設定見 `benchmarks/native-comparison.yaml`，其中 Cursor 明確信任測試 workspace；使用 repo 或 seed files 前請核對此設定。

## 新的量測設定

實作計畫見 [measurement-remediation-plan.md](docs/measurement-remediation-plan.md)。

- `output-lengths.yaml`：固定 100／500／2,000 行序列，七組設定，每組案例一次暖身、十次正式測量，共 21 次暖身與 210 次正式模型呼叫。
- `go-engineering.yaml`：三個明確標示的合成 Go bug，共 63 次正式呼叫。外部核心／回歸測試都必須通過；這不是上游真實 repo 題庫或 HarnessBench 相容實作。
- `warmup_repeats` 預設 0。所有暖身完成後才開始正式排程；暖身保留紀錄，但不納入統計。暖身不保證服務端快取一致。不可用 agent 仍會停止後續工作。
- 報表新增最後完整答案、成功 terminal 與 terminal 後程序退出的接收時間，另顯示 Unicode code point 字元／秒及其中位數。這些仍是 CLI 觀測值，不是純生成速度；少於十次完成樣本會標示限制。
- 補讀 Codex 原生 reasoning／cache-write usage。舊資料不改寫或推測補值。

Go 評分需要 PATH 中的 Go 1.26+ 與 Python 3。先執行 `make check-fixtures`，確認所有壞版在核心測試失敗、修正版通過核心與回歸測試。每層只複製 `candidate.go` 到獨立暫存 module，使用可信的外部測試，不採用 agent 修改的測試或 go.mod。此流程不是主機安全沙箱。`retain_files` 只保留明確指定、位於 workspace 內的檔案；保留檔案加上 `.txt` 副檔名，避免被開發工具誤編譯；各層結果存入紀錄與報表。

API 串流測速、真實 repo 案例挑選，以及 benchmark 專用設定隔離仍屬後續項目；現有 CLI profiles 的 generation／model-active TPS 繼續保持未知。
