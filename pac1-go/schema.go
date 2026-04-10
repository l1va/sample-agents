package main

import "encoding/json"

// AICODE-NOTE: This mirrors `NextStep` in pac1-py/agent.py. Pydantic
// auto-generates this schema from the Union[...] there; in Go we hand-write it
// so OpenAI strict mode can validate the structured output. Keep the union
// members in sync with the PCM runtime surface and with dispatch() in agent.go.

// nextStepSchema is the JSON Schema handed to OpenAI structured outputs. Strict
// mode requires every object to set additionalProperties:false and to list
// every property it declares in `required`.
var nextStepSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["current_state", "plan_remaining_steps_brief", "task_completed", "function"],
  "properties": {
    "current_state": {
      "type": "string",
      "description": "one-line summary of what has been learned so far"
    },
    "plan_remaining_steps_brief": {
      "type": "array",
      "description": "briefly explain the next useful steps",
      "items": {"type": "string"},
      "minItems": 1,
      "maxItems": 5
    },
    "task_completed": {"type": "boolean"},
    "function": {
      "description": "execute the first remaining step",
      "anyOf": [
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "completed_steps_laconic", "message", "grounding_refs", "outcome"],
          "properties": {
            "tool": {"type": "string", "const": "report_completion"},
            "completed_steps_laconic": {"type": "array", "items": {"type": "string"}},
            "message": {"type": "string"},
            "grounding_refs": {"type": "array", "items": {"type": "string"}},
            "outcome": {
              "type": "string",
              "enum": [
                "OUTCOME_OK",
                "OUTCOME_DENIED_SECURITY",
                "OUTCOME_NONE_CLARIFICATION",
                "OUTCOME_NONE_UNSUPPORTED",
                "OUTCOME_ERR_INTERNAL"
              ]
            }
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool"],
          "properties": {
            "tool": {"type": "string", "const": "context"}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "level", "root"],
          "properties": {
            "tool": {"type": "string", "const": "tree"},
            "level": {"type": "integer", "description": "max tree depth, 0 means unlimited"},
            "root": {"type": "string", "description": "tree root, empty means repository root"}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "name", "root", "kind", "limit"],
          "properties": {
            "tool": {"type": "string", "const": "find"},
            "name": {"type": "string"},
            "root": {"type": "string"},
            "kind": {"type": "string", "enum": ["all", "files", "dirs"]},
            "limit": {"type": "integer", "minimum": 1, "maximum": 20}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "pattern", "root", "limit"],
          "properties": {
            "tool": {"type": "string", "const": "search"},
            "pattern": {"type": "string"},
            "root": {"type": "string"},
            "limit": {"type": "integer", "minimum": 1, "maximum": 20}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "path"],
          "properties": {
            "tool": {"type": "string", "const": "list"},
            "path": {"type": "string"}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "path", "number", "start_line", "end_line"],
          "properties": {
            "tool": {"type": "string", "const": "read"},
            "path": {"type": "string"},
            "number": {"type": "boolean", "description": "return 1-based line numbers"},
            "start_line": {"type": "integer", "minimum": 0, "description": "1-based inclusive linum; 0 == from the first line"},
            "end_line": {"type": "integer", "minimum": 0, "description": "1-based inclusive linum; 0 == through the last line"}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "path", "content", "start_line", "end_line"],
          "properties": {
            "tool": {"type": "string", "const": "write"},
            "path": {"type": "string"},
            "content": {"type": "string"},
            "start_line": {"type": "integer", "minimum": 0, "description": "1-based inclusive line number; 0 keeps whole-file overwrite behavior"},
            "end_line": {"type": "integer", "minimum": 0, "description": "1-based inclusive line number; 0 means through the last line for ranged writes"}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "path"],
          "properties": {
            "tool": {"type": "string", "const": "delete"},
            "path": {"type": "string"}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "path"],
          "properties": {
            "tool": {"type": "string", "const": "mkdir"},
            "path": {"type": "string"}
          }
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["tool", "from_name", "to_name"],
          "properties": {
            "tool": {"type": "string", "const": "move"},
            "from_name": {"type": "string"},
            "to_name": {"type": "string"}
          }
        }
      ]
    }
  }
}`)

// nextStepEnvelope holds the OpenAI response before the `function` union is
// dispatched on the `tool` discriminator.
type nextStepEnvelope struct {
	CurrentState            string          `json:"current_state"`
	PlanRemainingStepsBrief []string        `json:"plan_remaining_steps_brief"`
	TaskCompleted           bool            `json:"task_completed"`
	Function                json.RawMessage `json:"function"`
}

// cmdHeader peeks at the `tool` field to pick the concrete variant.
type cmdHeader struct {
	Tool string `json:"tool"`
}

type reportCompletion struct {
	Tool                  string   `json:"tool"`
	CompletedStepsLaconic []string `json:"completed_steps_laconic"`
	Message               string   `json:"message"`
	GroundingRefs         []string `json:"grounding_refs"`
	Outcome               string   `json:"outcome"`
}

type reqContext struct {
	Tool string `json:"tool"`
}

type reqTree struct {
	Tool  string `json:"tool"`
	Level int32  `json:"level"`
	Root  string `json:"root"`
}

type reqFind struct {
	Tool  string `json:"tool"`
	Name  string `json:"name"`
	Root  string `json:"root"`
	Kind  string `json:"kind"`
	Limit int32  `json:"limit"`
}

type reqSearch struct {
	Tool    string `json:"tool"`
	Pattern string `json:"pattern"`
	Root    string `json:"root"`
	Limit   int32  `json:"limit"`
}

type reqList struct {
	Tool string `json:"tool"`
	Path string `json:"path"`
}

type reqRead struct {
	Tool      string `json:"tool"`
	Path      string `json:"path"`
	Number    bool   `json:"number"`
	StartLine int32  `json:"start_line"`
	EndLine   int32  `json:"end_line"`
}

type reqWrite struct {
	Tool      string `json:"tool"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	StartLine int32  `json:"start_line"`
	EndLine   int32  `json:"end_line"`
}

type reqDelete struct {
	Tool string `json:"tool"`
	Path string `json:"path"`
}

type reqMkDir struct {
	Tool string `json:"tool"`
	Path string `json:"path"`
}

type reqMove struct {
	Tool     string `json:"tool"`
	FromName string `json:"from_name"`
	ToName   string `json:"to_name"`
}
