package main

import (
	"fmt"
	"strings"

	pcm "bitgn.com/samples/pac1-go/gen/bitgn/vm/pcm"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// AICODE-NOTE: PAC1 feeds tool output back into the LLM verbatim, so keep
// tree/ls/cat/rg output in the same shell-like shape as pac1-py/agent.py.
// The model relies on recognizing these formats after several steps, so do
// not swap them for protobuf JSON on a whim.

func renderCommand(command, body string) string {
	return command + "\n" + body
}

func formatTreeEntry(entry *pcm.TreeResponse_Entry, prefix string, isLast bool) []string {
	branch := "├── "
	if isLast {
		branch = "└── "
	}
	lines := []string{prefix + branch + entry.Name}
	childPrefix := prefix + "│   "
	if isLast {
		childPrefix = prefix + "    "
	}
	children := entry.Children
	for i, child := range children {
		lines = append(lines, formatTreeEntry(child, childPrefix, i == len(children)-1)...)
	}
	return lines
}

func formatTree(cmd reqTree, result *pcm.TreeResponse) string {
	root := result.Root
	var body string
	if root == nil || root.Name == "" {
		body = "."
	} else {
		lines := []string{root.Name}
		children := root.Children
		for i, child := range children {
			lines = append(lines, formatTreeEntry(child, "", i == len(children)-1)...)
		}
		body = strings.Join(lines, "\n")
	}

	rootArg := cmd.Root
	if rootArg == "" {
		rootArg = "/"
	}
	levelArg := ""
	if cmd.Level > 0 {
		levelArg = fmt.Sprintf(" -L %d", cmd.Level)
	}
	return renderCommand(fmt.Sprintf("tree%s %s", levelArg, rootArg), body)
}

func formatList(cmd reqList, result *pcm.ListResponse) string {
	body := "."
	if len(result.Entries) > 0 {
		parts := make([]string, 0, len(result.Entries))
		for _, entry := range result.Entries {
			if entry.IsDir {
				parts = append(parts, entry.Name+"/")
			} else {
				parts = append(parts, entry.Name)
			}
		}
		body = strings.Join(parts, "\n")
	}
	return renderCommand(fmt.Sprintf("ls %s", cmd.Path), body)
}

func formatRead(cmd reqRead, result *pcm.ReadResponse) string {
	var command string
	switch {
	case cmd.StartLine > 0 || cmd.EndLine > 0:
		start := cmd.StartLine
		if start == 0 {
			start = 1
		}
		end := "$"
		if cmd.EndLine > 0 {
			end = fmt.Sprintf("%d", cmd.EndLine)
		}
		command = fmt.Sprintf("sed -n '%d,%sp' %s", start, end, cmd.Path)
	case cmd.Number:
		command = fmt.Sprintf("cat -n %s", cmd.Path)
	default:
		command = fmt.Sprintf("cat %s", cmd.Path)
	}
	return renderCommand(command, result.Content)
}

// shQuote is a tiny shell-quoter: wraps `s` in single quotes iff it contains
// characters a POSIX shell would interpret, matching what Python's shlex.quote
// would produce for the `rg` command header in pac1-py.
func shQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			continue
		case r == '@', r == '%', r == '+', r == '=', r == ':', r == ',',
			r == '.', r == '/', r == '-', r == '_':
			continue
		default:
			safe = false
		}
		if !safe {
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func formatSearch(cmd reqSearch, result *pcm.SearchResponse) string {
	// AICODE-NOTE: Keep PCM search output in `rg -n --no-heading` shape so the
	// LLM sees the familiar `path:line:text` contract.
	root := cmd.Root
	if root == "" {
		root = "/"
	}
	header := fmt.Sprintf("rg -n --no-heading -e %s %s", shQuote(cmd.Pattern), shQuote(root))
	parts := make([]string, 0, len(result.Matches))
	for _, m := range result.Matches {
		parts = append(parts, fmt.Sprintf("%s:%d:%s", m.Path, m.Line, m.LineText))
	}
	return renderCommand(header, strings.Join(parts, "\n"))
}

// formatResult prefers shell-like output for the commands the LLM saw in the
// Python sample, and falls back to pretty-printed protojson for everything
// else (mirrors Python's MessageToDict fallback in _format_result).
func formatResult(cmd any, result proto.Message) string {
	if result == nil {
		return "{}"
	}
	switch c := cmd.(type) {
	case reqTree:
		if r, ok := result.(*pcm.TreeResponse); ok {
			return formatTree(c, r)
		}
	case reqList:
		if r, ok := result.(*pcm.ListResponse); ok {
			return formatList(c, r)
		}
	case reqRead:
		if r, ok := result.(*pcm.ReadResponse); ok {
			return formatRead(c, r)
		}
	case reqSearch:
		if r, ok := result.(*pcm.SearchResponse); ok {
			return formatSearch(c, r)
		}
	}
	opts := protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: false}
	b, err := opts.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result)
	}
	return string(b)
}
