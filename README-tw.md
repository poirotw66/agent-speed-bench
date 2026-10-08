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

內建 coding CLI 統一使用指定的 YOLO 模式：Codex／Cursor 加上 `--yolo`，agy／Claude 加上 `--dangerously-skip-permissions`。Cursor 另指定 `--sandbox disabled`，避免繼承不同權限政策。每筆紀錄保留 adapter 的權限標記，舊 sandbox 結果不與 YOLO 結果合併。Generic 命令保留自行指定的 argv；demo 與 API adapter 沒有 coding CLI 權限模式。Workspace 隔離用於保護來源 checkout，不是主機安全沙箱；repo 內的測試仍可能被 agent 修改，不能當成防竄改的 hidden tests。

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

API 串流測速與更多真實 repo 案例仍屬後續項目；現有 CLI profiles 的 generation／model-active TPS 繼續保持未知。


## 真實 Go 案例與 Codex 設定隔離

`benchmarks/real-go.yaml` 使用 [lazygit PR #5495](https://github.com/jesseduffield/lazygit/pull/5495) 的 owner 大小寫錯誤，案例選擇參考 [HarnessBench](https://github.com/nyosegawa/harness-bench)。固定原始 commit 為 `8f258a3650cef809b911df24881712bc6b5d96bd`，修正版為 `38dd035e289dd71ad16fb0caa34525ad03460d21`；上游為 MIT 授權。這是一個真實 bug，不代表完整工程能力題庫。

```sh
# PATH 需要 Python 3.9+、Git、Go 1.26+。
python3 scripts/prepare-lazygit.py
./bin/agentspeedbench doctor benchmarks/real-go.yaml
./bin/agentspeedbench run benchmarks/real-go.yaml
```

準備步驟下載固定 repo 到忽略追蹤的 `runs/repos/lazygit`，確認原始版本在指定核心測試失敗、原始回歸通過、修正版兩層都通過。獨立編寫的外部測試檢查 owner 大小寫、分支名稱必須完全相同，以及不同 owner 不應配對；回歸另執行上游既有 PR-map 測試。評分每層都從原始 commit 重建可信的 module、package 與 vendor，只套用 agent 的 `pkg/commands/git_commands/github.go`，不採用 agent 修改的測試與依賴。使用 vendor 並關閉 module 查找，有時間限制並保留提交原始碼；這不是主機／網路沙箱，也不是全 repo 回歸。

`repo.fresh_history: true` 只在一次性 clone 移除上游 Git 歷史與 remote，建立單一起始 commit；run 仍記錄原始上游 SHA，manifest 保存此政策。不移除 repo 指令，也不限制其他主機檔案存取。

`isolate_config: true` 現在支援 Codex、Cursor 與 agy，建立私有暫存 HOME，只複製支援的登入資料。Codex 忽略個人設定與 rules，停用 memories、plugins、apps、瀏覽器／電腦操作及專案指令探索；CLI 一律使用上述 YOLO 政策，與設定隔離分開記錄；暫存 HOME 不再產生 sandbox 權限設定。各家的權限機制分別記錄，不能視為相同。Cursor 可讀取既有 macOS 鑰匙圈登入資料到暫存認證檔；agy 重用既有 OAuth token，沒有修改或修復系統鑰匙圈。正常清理會刪除暫存認證；程序突然終止可能留下私有暫存目錄。

案例可設定 `strip_instructions: true`，移除暫存工作區中已知的 agent 指令與設定，不跟隨符號連結。這是有版本的清單，並不代表主機、網路、管理政策、環境變數或服務端快取已完整隔離。兩個選項預設皆為 false，需使用支援相關旗標的 CLI。

## 受控比較與重新評分

`benchmarks/controlled-output.yaml` 包含七組設定、三種輸出長度，每組／案例一次暖身、十次正式測試，循序執行。`benchmarks/controlled-real-go.yaml` 每組執行三次 owner 大小寫案例。請分開執行，避免本機工作互相競爭。額度或認證限制會停止該設定的後續呼叫並列為未評分；不完整矩陣不能宣稱完成比較。

`go_cache: cold` 每次建立空的獨立 Go 快取；`go_cache: warm` 必須提供可信任的 `cache_warmup`，在固定原始版本上準備快取，再開始 agent 計時。兩者使用 vendored 相依套件，禁止模組查詢與自動下載工具鏈，另記錄準備時間。服務端快取仍不保證相同；舊設定維持原有快取行為。

新增真實案例為提交訊息空白保留（[PR #5528](https://github.com/jesseduffield/lazygit/pull/5528)）及分支差異批次查詢（[PR #5536](https://github.com/jesseduffield/lazygit/pull/5536)）。空白案例允許六個功能原始檔；外部評分聚焦分割與 co-author 行為，沒有完整互動 UI 驗證。固定 commit、允許提交的檔案與測試套件記於 `benchmarks/real-go/cases.json`。

```sh
python3 scripts/prepare-real-go.py
./bin/agentspeedbench run benchmarks/real-go-extended.yaml
# ATTEMPT_DIR 替換為一次 whitespace 測試的 artifact 目錄。
python3 scripts/verify-real-go.py whitespace core ATTEMPT_DIR/candidate --retained
python3 scripts/verify-real-go.py whitespace regression ATTEMPT_DIR/candidate --retained
```

`retain_patch: true` 需搭配 repo 與 retain_files；保存允許提交路徑的 `candidate.patch.txt`（包含新檔），以及 SHA-256／大小清單 `source_manifest.json`。重新評分驗證 hash，重建可信任原始版本與相依套件，不採用 candidate 測試。分享前需檢查提交原始碼內容。

報告依快取、指令、設定隔離與權限政策分組，顯示準備時間中位數、wall time 四分位數及範圍；這些不是信賴區間。舊紀錄若明確包含額度錯誤，報告會解讀為無法使用／未評分，不改寫原始資料。額度與基礎設施錯誤不納入速度樣本。agy 明確回報 headless 權限自動拒絕時，即使 exit code 是零也列為無法使用／未評分；參見[官方 headless 權限說明](https://www.antigravity.google/docs/cli/headless/)。Manifest 每筆紀錄後以原子替換更新；中斷的實驗需明確執行 `resume`。

## API 串流

`openai-responses` 是沒有 coding tools 的 HTTP SSE 回應 adapter。需指定 API 帳戶可用的 model、認證環境變數名稱與輸出 token 上限：

```yaml
name: api-response
adapter: openai-responses
model: YOUR_API_MODEL
api:
  endpoint: https://api.openai.com/v1/responses
  key_env: OPENAI_API_KEY
  max_output_tokens: 2048
```

金鑰只從環境變數讀取，不寫入設定或 artifacts。請求使用 stream=true、store=false，不自動重試；除離線測試用 loopback HTTP 外要求 HTTPS，拒絕轉址。原始回應會保留於本機，請使用適合保存的 prompt 與內容；HTTP 錯誤本文不保存。

第一至最後 SSE delta 的接收區間／Unicode 字元率不計入第一個 chunk 的字元，仍受緩衝與 relay 影響，不能當作解碼速度或 model-active TPS。原生 output token 可能包含推理，另標示 accounting；缺少 usage 或只有一個 delta 時保持未知。正式 API 實測仍待指定供應商、模型、認證環境變數與費用上限；本次尚未呼叫正式 API。[官方串流文件](https://developers.openai.com/api/docs/guides/streaming-responses)。

目前本機證據：修正 Cursor 認證重用後，七組隔離短輸出 smoke 皆成功；中斷的正式矩陣中 agy 完成 50 筆呼叫，沒有鑰匙圈／認證錯誤。受控快取 owner 案例通過兩層評分，兩個新增案例通過 base-fails/fixed-passes 驗證。這些驗證支持測試流程正常，尚不是完整速度排名，也不代表系統鑰匙圈已修復。

## 可靠性、失敗證據與續跑

可靠性表依實驗、agent、案例統計，不受 TTFA 或 token 計數分組影響。總紀錄數包含正式測量的成功、失敗、不可用、取消及跳過，排除暖身；完成率為完成數／總紀錄數，成功率為通過數／已評分數。速度表仍保留各自的時間基準與環境分組。明確連線錯誤列為服務錯誤／未評分；使用者取消也未評分，任務逾時仍算期限內未完成。歷史報告可重新解讀已有證據，不改寫原始紀錄；缺少診斷時不推測原因。

失敗、逾時及取消也會在清理 workspace 前，於獨立且有時間限制的清理 context 中保留指定原始碼與 patch。`patch_baseline` 使用 agent 執行前的 commit，包含 agent 自行 commit 的修改。未完成答案保存在 `assistant.partial.txt`；保留失敗記在 `artifact_errors`，不蓋掉原本錯誤。程序突然被強制終止仍可能無法完成保存。

```sh
agentspeedbench resume -db agentspeedbench.db runs/<experiment>
```

Schema 3 manifest 記錄 harness binary SHA-256、可取得的來源 commit／dirty 狀態、解析後設定雜湊、CLI binary 雜湊／版本及直接 verifier／快取準備執行檔雜湊。續跑要求版本一致並取得實驗鎖，將僅存在 artifact 的完成紀錄補入 SQLite，不覆寫既有證據。只補尚未記錄的工作；缺少的暖身先執行。已記錄的失敗、取消與跳過不重跑；重試或改設定需建立新實驗。舊版 manifest 不支援此續跑指令；升級前應保留原 binary。

`state: complete` 代表所有排程位置都有紀錄，不代表全部通過。`elapsed_seconds` 累積各次執行時間，不是跨中斷的日曆時間。雜湊不涵蓋完整主機環境、未宣告、由 verifier arguments 引用的輔助檔案、wrapper 依賴或服務端快取，不能宣稱環境完全相同。

Responses HTTP 錯誤只從有大小上限的回應中保留已知機器代碼，區分額度、認證、模型及限流；未知或格式錯誤使用 `http_error`。Relay 不保存 provider 錯誤訊息與本文，並區分 transport 錯誤與呼叫端取消。沒有自動重試；付費 API 實測仍待指定條件。

## 評分依賴與比較覆蓋率

`verify.inputs` 明確列出可信檔案或目錄，路徑相對於 YAML。Runner 記錄各檔案 SHA-256；修改、新增或刪除後會拒絕續跑。拒絕符號連結、特殊檔案、超過 8 MiB 的檔案及超過 10,000 個檔案的集合。內建工程 profiles 已宣告外部評分目錄，涵蓋案例設定與測試。未宣告依賴與 wrapper runtime 不會自動推測。

速度表同時呈現全部完成答案，以及僅驗證通過樣本的數量、wall／TTFA／有效 tokens/s／字元每秒中位數。錯誤答案仍列入完成樣本，但不會讓通過樣本的速度變快；未知評分也排除。時間基準、計數定義與環境仍各自分組。

可靠性表新增計畫／已記錄／缺少正式工作及覆蓋率。從 artifact 旁相符的 manifest 取得計畫；缺少或無效時保持未知。完成率仍以已記錄工作為分母，成功率仍以已評分工作為分母。未派出任何工作的 agent 也能顯示；完全沒有紀錄時可明確指定 manifest：

```sh
agentspeedbench report -db agentspeedbench.db -experiment <id> -manifest runs/<id>/manifest.json -out runs/coverage.html
agentspeedbench version
```

版本指令顯示來源 commit、dirty 狀態、commit 時間、Go 版本及平台；缺少 build metadata 時保持 `unknown`。Commit 時間不是編譯時間。Patch 保存改用私有 Git index，支援刪除、移出 index、重建與新增檔案，不修改 agent 的 index；必要的 retained source 缺少時仍另外記錄保存錯誤。

## 評分中斷與 artifact 錯誤

在 command verifier 執行期間取消，記錄為 `canceled`／未評分，停止後續評分層。Verifier 無法啟動時列為未評分的 `infrastructure_error`，代碼為 `verifier_start`；已啟動且回傳失敗 exit code 時仍算任務失敗。既有 verifier 期限政策保持已評分失敗。中斷前已完成的評分層證據保留。

指定 artifact 保存失敗時，只加入 `artifact_errors`，不改寫原本狀態、評分或失敗原因，包括通過與未評分結果。正式工作含保存錯誤時，CLI 的 `run`／`resume` 仍以失敗 exit code 結束，並明確說明評分已保留；報告另列保存錯誤。歷史紀錄不改寫，也不推測恢復已遺失的評分。

## 執行期間評分完整性與指標樣本數

正式 `run`／`resume` 會以原始 manifest 為基準，在 agent 執行前、評分前，以及每層 command 評分前後，核對宣告的評分輸入及直接 verifier／快取準備執行檔雜湊。依賴修改、新增、移除或無法讀取時，列為未評分的 `infrastructure_error`，代碼為 `scoring_inputs_changed`，停止後續評分層；後續工作也會在呼叫 agent 前被阻擋。原始紀錄與已執行的評分層證據保留，不會每輪重新接受新的基準。

這是邊界檢查，不是不可變的評分快照；不能保證發現兩次檢查之間改動後又還原的情況。未宣告依賴仍不涵蓋。雜湊檢查不計入 agent 程序 wall time。

各彙總速度指標新增自己的有效樣本數 `n`，涵蓋 wall time、TTFA、有效 tokens/s、字元每秒、答案完成、退出間隔與準備時間；僅通過樣本也各自計數。缺少數值不計入，明確回報的零值仍有效，`n=0` 保持未知。各指標只有 1–9 筆時，逐項標示小樣本，即使整組已有十筆完成或通過紀錄。時間基準、計數定義與環境分組保持原有規則。

## 乾淨安裝、版本保存與執行進度

```sh
# 需要乾淨且已 commit 的來源、Python 3.9+，以及 PATH 中的 Go。
make install
# 使用舊 manifest 中的 harness binary SHA-256 恢復版本。
python3 scripts/install-local.py --restore <full-sha256>
agentspeedbench resume -db agentspeedbench.db runs/<experiment>
# 舊實驗處理完後，重新安裝目前乾淨的 commit。
make install
```

安裝會核對來源 commit 與 `dirty=false`，編譯期間來源改變則拒絕安裝。替換 `~/.local/bin/agentspeedbench` 前，新舊 binary 都保存到 `~/.local/share/agentspeedbench/binaries/<sha256>/agentspeedbench`；保存內容雜湊不符時拒絕替換。恢復時使用原安裝路徑，讓執行檔路徑能與舊實驗一致；CLI、設定及評分依賴仍必須通過續跑檢查。可用 `--bin-dir`／`--archive-dir` 指定獨立安裝位置。保存的是執行檔，不是登入資料或完整環境。

CI 的 macOS／Linux 工作改為執行完整 `make check`，涵蓋壞版失敗／修正版通過的案例驗證，以及安裝保存／損壞拒絕測試，再執行離線 CLI demo。本機通過不等同 hosted CI 已通過。

每筆實際工作新增 `timing`，記錄工作流程準備、評分（含完整性檢查）、artifact 保存、清理與總時間；各 command 評分層也記錄程序 wall time。總時間從建立 artifact 目錄後到最終紀錄寫入前，涵蓋 agent 與輸出後處理，排除實驗 preflight、最終 JSON／SQLite 寫入、排隊及報告產生。原 agent wall time 與有效 tokens/s 定義不變。未進入的階段與歷史資料保持未知；報告另列工作流程中位數及各自有效樣本數。

CLI 會在階段切換時及每 30 秒顯示進度，包含目前階段、工作經過時間、最近收到 agent／verifier 輸出的 UTC 時間與距今多久；未換行的部分輸出也會更新觀察時間。沒有輸出時保持 `unknown`，不直接判定卡住，不觸發重試或改變評分。進度輸出與廠商 telemetry 事件分開。
