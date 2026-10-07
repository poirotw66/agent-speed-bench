package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/poirotw66/agent-speed-bench/internal/responses"
)

func responsesAgent(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("__responses-agent", flag.ContinueOnError)
	model := flags.String("model", "", "API model")
	endpoint := flags.String("endpoint", "https://api.openai.com/v1/responses", "Responses endpoint")
	keyEnv := flags.String("key-env", "", "API key environment variable name")
	maxTokens := flags.Int("max-output-tokens", 0, "Output token limit")
	effort := flags.String("effort", "", "Reasoning effort")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected Responses arguments")
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 8*1024*1024+1))
	if err != nil {
		return err
	}
	if len(input) > 8*1024*1024 {
		return fmt.Errorf("API prompt exceeds limit")
	}
	request := responses.Request{Model: *model, Input: string(input), MaxOutputTokens: *maxTokens}
	if *effort != "" {
		request.Reasoning = &responses.Reasoning{Effort: *effort}
	}
	return responses.Stream(ctx, *endpoint, os.Getenv(*keyEnv), request, os.Stdout)
}
