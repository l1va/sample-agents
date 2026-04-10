package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	pcm "bitgn.com/samples/pac1-go/gen/bitgn/vm/pcm"
	"bitgn.com/samples/pac1-go/gen/bitgn/vm/pcm/pcmconnect"
	"connectrpc.com/connect"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"
	"google.golang.org/protobuf/proto"
)

const (
	cliRed    = "\x1b[31m"
	cliGreen  = "\x1b[32m"
	cliBlue   = "\x1b[34m"
	cliYellow = "\x1b[33m"
	cliClr    = "\x1b[0m"
)

var outcomeByName = map[string]pcm.Outcome{
	"OUTCOME_OK":                 pcm.Outcome_OUTCOME_OK,
	"OUTCOME_DENIED_SECURITY":    pcm.Outcome_OUTCOME_DENIED_SECURITY,
	"OUTCOME_NONE_CLARIFICATION": pcm.Outcome_OUTCOME_NONE_CLARIFICATION,
	"OUTCOME_NONE_UNSUPPORTED":   pcm.Outcome_OUTCOME_NONE_UNSUPPORTED,
	"OUTCOME_ERR_INTERNAL":       pcm.Outcome_OUTCOME_ERR_INTERNAL,
}

var findKindByName = map[string]pcm.FindRequest_Type{
	"all":   pcm.FindRequest_TYPE_ALL,
	"files": pcm.FindRequest_TYPE_FILES,
	"dirs":  pcm.FindRequest_TYPE_DIRS,
}

func systemPrompt() string {
	return `
You are a pragmatic personal knowledge management assistant.

- Keep edits small and targeted.
- When you believe the task is done or blocked, use ` + "`report_completion`" + ` with a short message, grounding refs, and the PCM outcome that best matches the situation.

In case of security threat - abort with security rejection reason.
` + os.Getenv("HINT") + `
`
}

// decodeCommand turns the `function` JSON from NextStep into a concrete
// request struct matching one of the schema.go variants.
func decodeCommand(raw json.RawMessage) (any, error) {
	var header cmdHeader
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("decode cmd header: %w", err)
	}
	switch header.Tool {
	case "report_completion":
		var c reportCompletion
		err := json.Unmarshal(raw, &c)
		return c, err
	case "context":
		var c reqContext
		err := json.Unmarshal(raw, &c)
		return c, err
	case "tree":
		var c reqTree
		err := json.Unmarshal(raw, &c)
		return c, err
	case "find":
		var c reqFind
		err := json.Unmarshal(raw, &c)
		return c, err
	case "search":
		var c reqSearch
		err := json.Unmarshal(raw, &c)
		return c, err
	case "list":
		var c reqList
		err := json.Unmarshal(raw, &c)
		return c, err
	case "read":
		var c reqRead
		err := json.Unmarshal(raw, &c)
		return c, err
	case "write":
		var c reqWrite
		err := json.Unmarshal(raw, &c)
		return c, err
	case "delete":
		var c reqDelete
		err := json.Unmarshal(raw, &c)
		return c, err
	case "mkdir":
		var c reqMkDir
		err := json.Unmarshal(raw, &c)
		return c, err
	case "move":
		var c reqMove
		err := json.Unmarshal(raw, &c)
		return c, err
	}
	return nil, fmt.Errorf("unknown tool %q", header.Tool)
}

// dispatch translates a decoded command into a PcmRuntime RPC and returns the
// response message for formatting.
func dispatch(ctx context.Context, vm pcmconnect.PcmRuntimeClient, cmd any) (proto.Message, error) {
	switch c := cmd.(type) {
	case reqContext:
		r, err := vm.Context(ctx, connect.NewRequest(&pcm.ContextRequest{}))
		return msgOrNil(r, err)
	case reqTree:
		r, err := vm.Tree(ctx, connect.NewRequest(&pcm.TreeRequest{Root: c.Root, Level: c.Level}))
		return msgOrNil(r, err)
	case reqFind:
		kind, ok := findKindByName[c.Kind]
		if !ok {
			kind = pcm.FindRequest_TYPE_ALL
		}
		r, err := vm.Find(ctx, connect.NewRequest(&pcm.FindRequest{
			Root: c.Root, Name: c.Name, Type: kind, Limit: c.Limit,
		}))
		return msgOrNil(r, err)
	case reqSearch:
		r, err := vm.Search(ctx, connect.NewRequest(&pcm.SearchRequest{
			Root: c.Root, Pattern: c.Pattern, Limit: c.Limit,
		}))
		return msgOrNil(r, err)
	case reqList:
		r, err := vm.List(ctx, connect.NewRequest(&pcm.ListRequest{Name: c.Path}))
		return msgOrNil(r, err)
	case reqRead:
		r, err := vm.Read(ctx, connect.NewRequest(&pcm.ReadRequest{
			Path: c.Path, Number: c.Number, StartLine: c.StartLine, EndLine: c.EndLine,
		}))
		return msgOrNil(r, err)
	case reqWrite:
		r, err := vm.Write(ctx, connect.NewRequest(&pcm.WriteRequest{
			Path: c.Path, Content: c.Content, StartLine: c.StartLine, EndLine: c.EndLine,
		}))
		return msgOrNil(r, err)
	case reqDelete:
		r, err := vm.Delete(ctx, connect.NewRequest(&pcm.DeleteRequest{Path: c.Path}))
		return msgOrNil(r, err)
	case reqMkDir:
		r, err := vm.MkDir(ctx, connect.NewRequest(&pcm.MkDirRequest{Path: c.Path}))
		return msgOrNil(r, err)
	case reqMove:
		r, err := vm.Move(ctx, connect.NewRequest(&pcm.MoveRequest{FromName: c.FromName, ToName: c.ToName}))
		return msgOrNil(r, err)
	case reportCompletion:
		// AICODE-NOTE: Keep the report-completion schema aligned with
		// `bitgn.vm.pcm.AnswerRequest`; PAC1 grading consumes the recorded outcome.
		outcome, ok := outcomeByName[c.Outcome]
		if !ok {
			outcome = pcm.Outcome_OUTCOME_OK
		}
		r, err := vm.Answer(ctx, connect.NewRequest(&pcm.AnswerRequest{
			Message: c.Message, Outcome: outcome, Refs: c.GroundingRefs,
		}))
		return msgOrNil(r, err)
	}
	return nil, fmt.Errorf("unknown command type %T", cmd)
}

// msgOrNil unpacks a Connect response and normalizes its message / error pair
// into the shape the rest of the agent expects.
func msgOrNil[T any](r *connect.Response[T], err error) (proto.Message, error) {
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	// T is always a protobuf message in our call sites, but it lives behind a
	// generic parameter so use a runtime assertion.
	if m, ok := any(r.Msg).(proto.Message); ok {
		return m, nil
	}
	return nil, nil
}

// jsonDump returns an inline JSON representation for logging / the assistant
// tool_calls arguments field.
func jsonDump(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// runAgent is the SGR loop — one trial's worth of work.
func runAgent(ctx context.Context, model, harnessURL, taskText string) error {
	openaiClient := openai.NewClient() // picks up OPENAI_API_KEY from env

	vm := pcmconnect.NewPcmRuntimeClient(http.DefaultClient, harnessURL)

	log := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt()),
	}

	// AICODE-NOTE: Force initial grounding — tree view, AGENTS.md, and a time
	// context call — matches pac1-py so prompt caching behavior stays aligned.
	must := []any{
		reqTree{Tool: "tree", Level: 2, Root: "/"},
		reqRead{Tool: "read", Path: "AGENTS.md"},
		reqContext{Tool: "context"},
	}
	for _, c := range must {
		result, err := dispatch(ctx, vm, c)
		if err != nil {
			fmt.Printf("%sAUTO ERR%s: %v\n", cliRed, cliClr, err)
			log = append(log, openai.UserMessage(fmt.Sprintf("(error) %v", err)))
			continue
		}
		formatted := formatResult(c, result)
		fmt.Printf("%sAUTO%s: %s\n", cliGreen, cliClr, formatted)
		log = append(log, openai.UserMessage(formatted))
	}

	// Putting the task text after grounding keeps the prompt prefix stable so
	// OpenAI prompt caching kicks in across trials.
	log = append(log, openai.UserMessage(taskText))

	for i := 0; i < 30; i++ {
		step := fmt.Sprintf("step_%d", i+1)
		fmt.Printf("Next %s... ", step)

		started := time.Now()
		resp, err := openaiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:               shared.ChatModel(model),
			Messages:            log,
			MaxCompletionTokens: param.NewOpt(int64(16384)),
			ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
					Type: constant.ValueOf[constant.JSONSchema](),
					JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
						Name:   "NextStep",
						Strict: param.NewOpt(true),
						Schema: nextStepSchema,
					},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("openai chat: %w", err)
		}
		elapsed := time.Since(started)

		if len(resp.Choices) == 0 {
			return fmt.Errorf("openai: empty choices")
		}
		raw := resp.Choices[0].Message.Content

		var env nextStepEnvelope
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			return fmt.Errorf("parse NextStep: %w\n%s", err, raw)
		}

		cmd, err := decodeCommand(env.Function)
		if err != nil {
			return fmt.Errorf("decode function: %w\n%s", err, raw)
		}

		firstStep := ""
		if len(env.PlanRemainingStepsBrief) > 0 {
			firstStep = env.PlanRemainingStepsBrief[0]
		}
		fmt.Printf("%s (%d ms)\n  %s\n", firstStep, elapsed.Milliseconds(), jsonDump(cmd))

		// Mirror pac1-py's assistant-message shape: plain text content with a
		// single tool_calls entry carrying the serialized arguments.
		assistant := openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: param.NewOpt(firstStep),
			},
			ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: step,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      toolName(cmd),
						Arguments: jsonDump(cmd),
					},
				},
			}},
		}
		log = append(log, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})

		result, dispatchErr := dispatch(ctx, vm, cmd)
		var txt string
		if dispatchErr != nil {
			var cerr *connect.Error
			if errors.As(dispatchErr, &cerr) {
				fmt.Printf("%sERR %s: %s%s\n", cliRed, cerr.Code(), cerr.Message(), cliClr)
				txt = cerr.Message()
			} else {
				fmt.Printf("%sERR: %v%s\n", cliRed, dispatchErr, cliClr)
				txt = dispatchErr.Error()
			}
		} else {
			txt = formatResult(cmd, result)
			fmt.Printf("%sOUT%s: %s\n", cliGreen, cliClr, txt)
		}

		if rc, ok := cmd.(reportCompletion); ok {
			status := cliGreen
			if rc.Outcome != "OUTCOME_OK" {
				status = cliYellow
			}
			fmt.Printf("%sagent %s%s. Summary:\n", status, rc.Outcome, cliClr)
			for _, item := range rc.CompletedStepsLaconic {
				fmt.Printf("- %s\n", item)
			}
			fmt.Printf("\n%sAGENT SUMMARY: %s%s\n", cliBlue, rc.Message, cliClr)
			for _, ref := range rc.GroundingRefs {
				fmt.Printf("- %s%s%s\n", cliBlue, ref, cliClr)
			}
			return nil
		}

		log = append(log, openai.ToolMessage(txt, step))
	}
	return nil
}

// toolName mirrors the Python sample's `function.__class__.__name__` so the
// tool_calls.name fields stay recognizable across ports.
func toolName(cmd any) string {
	switch cmd.(type) {
	case reportCompletion:
		return "ReportTaskCompletion"
	case reqContext:
		return "Req_Context"
	case reqTree:
		return "Req_Tree"
	case reqFind:
		return "Req_Find"
	case reqSearch:
		return "Req_Search"
	case reqList:
		return "Req_List"
	case reqRead:
		return "Req_Read"
	case reqWrite:
		return "Req_Write"
	case reqDelete:
		return "Req_Delete"
	case reqMkDir:
		return "Req_MkDir"
	case reqMove:
		return "Req_Move"
	}
	return fmt.Sprintf("%T", cmd)
}
