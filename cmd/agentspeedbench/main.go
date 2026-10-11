package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/report"
	"github.com/poirotw66/agent-speed-bench/internal/runner"
	"github.com/poirotw66/agent-speed-bench/internal/storage"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func main() { os.Exit(mainExit(os.Args[1:])) }
func mainExit(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "__responses-agent":
		return responsesAgent(ctx, args[1:])
	case "help", "--help", "-h":
		usage()
		return nil
	case "version", "--version":
		fmt.Println(versionString())
		return nil
	case "__demo-agent":
		return demoAgent()
	case "run", "resume":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		root, jobs := "runs", 0
		if args[0] == "run" {
			fs.StringVar(&root, "out", "runs", "Artifact root")
			fs.IntVar(&jobs, "jobs", 0, "Override parallel jobs (default: configuration)")
		}
		dbPath := fs.String("db", "agentspeedbench.db", "SQLite database")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: agentspeedbench %s [-db agentspeedbench.db] configuration-or-experiment-directory", args[0])
		}
		var cfg benchmark.Config
		if args[0] == "run" {
			var err error
			cfg, err = benchmark.Load(fs.Arg(0))
			if err != nil {
				return err
			}
			if jobs != 0 {
				cfg.Jobs = jobs
			}
		}
		s, err := storage.Open(*dbPath)
		if err != nil {
			return err
		}
		defer s.Close()
		var result runner.Result
		var runErr error
		if args[0] == "resume" {
			result, runErr = runner.Resume(ctx, fs.Arg(0), s, os.Stdout)
		} else {
			result, runErr = runner.Execute(ctx, cfg, root, s, os.Stdout)
		}
		if result.Directory != "" {
			if err := writeReport(filepath.Join(result.Directory, "report.html"), result.Runs, manifestPlans(result.Manifest)...); err != nil {
				return err
			}
			report.Text(os.Stdout, result.Runs, manifestPlans(result.Manifest)...)
			fmt.Println("Artifacts:", result.Directory)
		}
		if runErr != nil {
			return runErr
		}
		for _, r := range result.Runs {
			if r.Cleanup != nil {
				return errors.New("workspace cleanup failed; grading results are preserved; inspect report.html")
			}
			if r.Warmup {
				continue
			}
			if len(r.ArtifactErrors) > 0 {
				return errors.New("requested artifacts could not be retained; grading results are preserved; inspect report.html")
			}
			if r.Status != "completed" || (r.Success != nil && !*r.Success) {
				return errors.New("one or more benchmark attempts did not complete or pass; inspect report.html")
			}
		}
		return nil
	case "doctor":
		if len(args) != 2 {
			return errors.New("usage: agentspeedbench doctor benchmark.yaml")
		}
		cfg, err := benchmark.Load(args[1])
		if err != nil {
			return err
		}
		infos, err := runner.Preflight(ctx, cfg)
		if err != nil {
			return err
		}
		if err := runner.ResolveRepos(ctx, &cfg); err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(infos)
	case "report":
		fs := flag.NewFlagSet("report", flag.ContinueOnError)
		dbPath := fs.String("db", "agentspeedbench.db", "SQLite database")
		experiment := fs.String("experiment", "", "Filter experiment ID; empty includes history")
		out := fs.String("out", "report.html", "HTML report path")
		manifestPath := fs.String("manifest", "", "Explicit experiment manifest for coverage, including zero records")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("report does not accept positional arguments")
		}
		if _, err := os.Stat(*dbPath); err != nil {
			return err
		}
		s, err := storage.Open(*dbPath)
		if err != nil {
			return err
		}
		defer s.Close()
		runs, err := s.Runs(*experiment)
		if err != nil {
			return err
		}
		plans := reportPlans(runs)
		if *manifestPath != "" {
			plans, err = explicitReportPlans(*manifestPath, *experiment)
			if err != nil {
				return err
			}
			if *experiment == "" {
				for _, r := range runs {
					if len(plans) > 0 && r.ExperimentID != plans[0].Experiment {
						return errors.New("explicit manifest requires a matching experiment filter")
					}
				}
			}
		}
		if len(runs) == 0 && len(plans) == 0 {
			return errors.New("no runs match the requested experiment")
		}
		if err := writeReport(*out, runs, plans...); err != nil {
			return err
		}
		report.Text(os.Stdout, runs, plans...)
		fmt.Println("Report:", *out)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func usage() {
	fmt.Print(`AgentSpeedBench — benchmark coding-agent latency, throughput, and correctness.

Usage:
  agentspeedbench run [-out runs] [-db agentspeedbench.db] [-jobs 1] benchmark.yaml
  agentspeedbench resume [-db agentspeedbench.db] experiment-directory
  agentspeedbench doctor benchmark.yaml
  agentspeedbench report [-db agentspeedbench.db] [-experiment ID] [-out report.html]
  agentspeedbench version

Start offline: agentspeedbench run benchmarks/demo.yaml
Real runs invoke installed CLIs and can consume the configured account's quota.
`)
}
func writeReport(path string, runs []telemetry.Run, plans ...report.Plan) error {
	if len(plans) == 0 {
		plans = reportPlans(runs)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	writeErr := report.HTML(f, runs, plans...)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
func demoAgent() error {
	if _, err := io.ReadAll(io.LimitReader(os.Stdin, 1024*1024)); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	emit := func(e telemetry.Event) error { return enc.Encode(e) }
	if err := emit(telemetry.Event{Type: "agent_ready"}); err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	if err := emit(telemetry.Event{Type: "assistant_output", Text: "Working on the fixture.\n"}); err != nil {
		return err
	}
	if err := emit(telemetry.Event{Type: "tool_started", ToolID: "demo-write", ToolName: "write_file"}); err != nil {
		return err
	}
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile("solution.txt", []byte("42\n"), 0600); err != nil {
		return err
	}
	if err := emit(telemetry.Event{Type: "tool_finished", ToolID: "demo-write", ToolName: "write_file"}); err != nil {
		return err
	}
	time.Sleep(30 * time.Millisecond)
	if err := emit(telemetry.Event{Type: "assistant_output", Text: "Done: 42\n"}); err != nil {
		return err
	}
	input, output := int64(24), int64(12)
	if err := emit(telemetry.Event{Type: "usage_reported", Usage: &telemetry.Usage{InputTokens: &input, OutputTokens: &output}}); err != nil {
		return err
	}
	return emit(telemetry.Event{Type: "agent_completed"})
}
