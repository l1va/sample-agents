package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	bitgn "bitgn.com/samples/pac1-go/gen/bitgn/harness"
	"bitgn.com/samples/pac1-go/gen/bitgn/harness/harnessconnect"
	"connectrpc.com/connect"
)

func main() {
	loadDotEnv(".env")

	bitgnURL := envOr("BITGN_HOST", "https://api.bitgn.com")
	bitgnAPIKey := os.Getenv("BITGN_API_KEY")
	benchID := envOr("BENCH_ID", "bitgn/pac1-dev")
	modelID := envOr("MODEL_ID", "gpt-4.1-2025-04-14")

	taskFilter := map[string]bool{}
	for _, t := range os.Args[1:] {
		taskFilter[t] = true
	}

	ctx := context.Background()
	client := harnessconnect.NewHarnessServiceClient(http.DefaultClient, bitgnURL)

	type scoreEntry struct {
		taskID string
		score  float32
	}
	var scores []scoreEntry

	status, err := client.Status(ctx, connect.NewRequest(&bitgn.StatusRequest{}))
	if err != nil {
		printConnectErr("status", err)
		os.Exit(1)
	}
	fmt.Printf("Connecting to BitGN: status=%q version=%q\n",
		status.Msg.Status, status.Msg.Version)

	bench, err := client.GetBenchmark(ctx, connect.NewRequest(&bitgn.GetBenchmarkRequest{
		BenchmarkId: benchID,
	}))
	if err != nil {
		printConnectErr("get_benchmark", err)
		os.Exit(1)
	}
	fmt.Printf("%s benchmark: %s with %d tasks.\n%s%s%s\n",
		bitgn.EvalPolicy_name[int32(bench.Msg.Policy)],
		bench.Msg.BenchmarkId,
		len(bench.Msg.Tasks),
		cliGreen, bench.Msg.Description, cliClr)

	run, err := client.StartRun(ctx, connect.NewRequest(&bitgn.StartRunRequest{
		Name:        "SGR NextStep Sample (Go)",
		BenchmarkId: benchID,
		ApiKey:      bitgnAPIKey,
	}))
	if err != nil {
		printConnectErr("start_run", err)
		os.Exit(1)
	}

	// Always submit the run, even on error / Ctrl-C. Mirrors the try/finally
	// block in pac1-py/main.py so a half-finished run still lands on the
	// leaderboard.
	defer func() {
		if _, err := client.SubmitRun(ctx, connect.NewRequest(&bitgn.SubmitRunRequest{
			RunId: run.Msg.RunId, Force: true,
		})); err != nil {
			printConnectErr("submit_run", err)
		}

		if len(scores) > 0 {
			var total float32
			for _, s := range scores {
				style := cliGreen
				if s.score < 1 {
					style = cliRed
				}
				fmt.Printf("%s: %s%.2f%s\n", s.taskID, style, s.score, cliClr)
				total += s.score
			}
			avg := float64(total) / float64(len(scores)) * 100.0
			fmt.Printf("FINAL: %.2f%%\n", avg)
		}
	}()

	for _, trialID := range run.Msg.TrialIds {
		trial, err := client.StartTrial(ctx, connect.NewRequest(&bitgn.StartTrialRequest{
			TrialId: trialID,
		}))
		if err != nil {
			printConnectErr("start_trial", err)
			continue
		}

		if len(taskFilter) > 0 && !taskFilter[trial.Msg.TaskId] {
			continue
		}

		header := strings.Repeat("=", 30)
		fmt.Printf("%s Starting task: %s %s\n", header, trial.Msg.TaskId, header)
		fmt.Printf("%s%s%s\n%s\n", cliBlue, trial.Msg.Instruction, cliClr, strings.Repeat("-", 80))

		if err := runAgent(ctx, modelID, trial.Msg.HarnessUrl, trial.Msg.Instruction); err != nil {
			fmt.Printf("%sagent error: %v%s\n", cliRed, err, cliClr)
		}

		result, err := client.EndTrial(ctx, connect.NewRequest(&bitgn.EndTrialRequest{
			TrialId: trial.Msg.TrialId,
		}))
		if err != nil {
			printConnectErr("end_trial", err)
			continue
		}
		if result.Msg.Score != nil && *result.Msg.Score >= 0 {
			score := *result.Msg.Score
			scores = append(scores, scoreEntry{trial.Msg.TaskId, score})
			style := cliGreen
			if score < 1 {
				style = cliRed
			}
			fmt.Printf("\n%sScore: %.2f\n", style, score)
			for _, line := range result.Msg.ScoreDetail {
				fmt.Printf("  %s\n", line)
			}
			fmt.Printf("%s\n", cliClr)
		}
	}
}

func printConnectErr(op string, err error) {
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		fmt.Printf("%s%s: %s: %s%s\n", cliRed, op, cerr.Code(), cerr.Message(), cliClr)
		return
	}
	fmt.Printf("%s%s: %v%s\n", cliRed, op, err, cliClr)
}
