package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// The ports send Anthropic Messages requests and read Anthropic replies.
// A provider is the API the backend forwards them to: the Anthropic
// Messages API as it is, or an OpenAI chat completions API, which local
// servers such as Ollama and llama.cpp also serve, through the
// translations below. The ports do not change with the provider.
const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
)

// ProviderDefaultUpstream is the URL of each provider's hosted API.
var ProviderDefaultUpstream = map[string]string{
	ProviderAnthropic: "https://api.anthropic.com/v1/messages",
	ProviderOpenAI:    "https://api.openai.com/v1/chat/completions",
}

// StoreKey is the key of a request in the store: the hex SHA-256 of the
// provider, the model the request goes to and the request bytes, so the
// replies of two providers or two models never meet.
func StoreKey(provider, model string, body []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "backendstore key 2\x00%s\x00%s\x00", provider, model)
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// RequestModel is the model an Anthropic request names, or "" when the body
// is not JSON or names none.
func RequestModel(body []byte) string {
	var r struct {
		Model string `json:"model"`
	}
	json.Unmarshal(body, &r)
	return r.Model
}

// requestModelSet returns the request with its model replaced, keeping the
// other fields as they are.
func requestModelSet(body []byte, model string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(model)
	fields["model"] = encoded
	return json.Marshal(fields)
}

// providerForward sends an Anthropic request body to the provider at
// upstream and returns the reply as an Anthropic reply.
func providerForward(provider, upstream, apiKey, apiVersion string, body []byte) (int, []byte, error) {
	if upstream == "" {
		return 0, nil, errors.New("a live request with no upstream")
	}
	outgoing := body
	if provider == ProviderOpenAI {
		var err error
		if outgoing, err = OpenAIRequestFromAnthropic(body); err != nil {
			return 0, nil, err
		}
	}
	req, err := http.NewRequest(http.MethodPost, upstream, bytes.NewReader(outgoing))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	switch provider {
	case ProviderAnthropic:
		req.Header.Set("anthropic-version", apiVersion)
		if apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
		}
	case ProviderOpenAI:
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	default:
		return 0, nil, fmt.Errorf("unknown provider %q", provider)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	reply, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	if provider == ProviderOpenAI {
		return AnthropicReplyFromOpenAI(resp.StatusCode, reply)
	}
	return resp.StatusCode, reply, nil
}

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens,omitempty"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicReply struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Role       string           `json:"role"`
	Model      string           `json:"model"`
	Content    []anthropicBlock `json:"content"`
	StopReason string           `json:"stop_reason"`
	Usage      *anthropicUsage  `json:"usage,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openAIRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Messages  []openAIMessage `json:"messages"`
	Tools     []openAITool    `json:"tools,omitempty"`
}

type openAIReply struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func stringPointer(s string) *string { return &s }

// anthropicBlocks reads message content, a string or a list of blocks.
func anthropicBlocks(content json.RawMessage) ([]anthropicBlock, error) {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return []anthropicBlock{{Type: "text", Text: text}}, nil
	}
	var blocks []anthropicBlock
	err := json.Unmarshal(content, &blocks)
	return blocks, err
}

// toolResultText is the text of a tool_result content, a string or text
// blocks.
func toolResultText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text
	}
	var blocks []anthropicBlock
	json.Unmarshal(content, &blocks)
	var b bytes.Buffer
	for _, block := range blocks {
		b.WriteString(block.Text)
	}
	return b.String()
}

// OpenAIRequestFromAnthropic translates an Anthropic Messages request into
// an OpenAI chat completions request. The system prompt becomes the first
// message, a tool_use block a tool call of the assistant message and a
// tool_result block a message of role tool.
func OpenAIRequestFromAnthropic(body []byte) ([]byte, error) {
	var in anthropicRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	out := openAIRequest{Model: in.Model, MaxTokens: in.MaxTokens}
	if in.System != "" {
		out.Messages = append(out.Messages, openAIMessage{Role: "system", Content: stringPointer(in.System)})
	}
	for _, m := range in.Messages {
		blocks, err := anthropicBlocks(m.Content)
		if err != nil {
			return nil, err
		}
		message := openAIMessage{Role: m.Role}
		var text bytes.Buffer
		for _, b := range blocks {
			switch b.Type {
			case "text":
				text.WriteString(b.Text)
			case "tool_use":
				call := openAIToolCall{ID: b.ID, Type: "function"}
				call.Function.Name = b.Name
				call.Function.Arguments = string(b.Input)
				message.ToolCalls = append(message.ToolCalls, call)
			case "tool_result":
				result := toolResultText(b.Content)
				if b.IsError {
					result = "error: " + result
				}
				out.Messages = append(out.Messages, openAIMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: stringPointer(result)})
			default:
				return nil, fmt.Errorf("content block of type %q has no translation", b.Type)
			}
		}
		if text.Len() > 0 || len(message.ToolCalls) > 0 {
			if text.Len() > 0 || m.Role != "assistant" {
				message.Content = stringPointer(text.String())
			}
			out.Messages = append(out.Messages, message)
		}
	}
	for _, t := range in.Tools {
		tool := openAITool{Type: "function"}
		tool.Function.Name = t.Name
		tool.Function.Description = t.Description
		tool.Function.Parameters = t.InputSchema
		out.Tools = append(out.Tools, tool)
	}
	return json.Marshal(out)
}

// openAIStopReasons maps an OpenAI finish reason to an Anthropic stop
// reason.
var openAIStopReasons = map[string]string{
	"stop":           "end_turn",
	"tool_calls":     "tool_use",
	"function_call":  "tool_use",
	"length":         "max_tokens",
	"content_filter": "refusal",
}

// AnthropicReplyFromOpenAI translates an OpenAI chat completions reply and
// its status into an Anthropic Messages reply. An error reply becomes an
// Anthropic error with the same status.
func AnthropicReplyFromOpenAI(status int, body []byte) (int, []byte, error) {
	var in openAIReply
	if err := json.Unmarshal(body, &in); err != nil || status != http.StatusOK || in.Error != nil || len(in.Choices) == 0 {
		message := string(body)
		kind := "api_error"
		if in.Error != nil {
			message, kind = in.Error.Message, in.Error.Type
		}
		if status == http.StatusOK {
			status = http.StatusBadGateway
		}
		reply, err := json.Marshal(stubError{Type: "error", Error: stubErrorDetail{Type: kind, Message: message}})
		return status, reply, err
	}
	choice := in.Choices[0]
	out := anthropicReply{ID: in.ID, Type: "message", Role: "assistant", Model: in.Model, StopReason: openAIStopReasons[choice.FinishReason]}
	if out.StopReason == "" {
		out.StopReason = "end_turn"
	}
	if choice.Message.Content != nil && *choice.Message.Content != "" {
		out.Content = append(out.Content, anthropicBlock{Type: "text", Text: *choice.Message.Content})
	}
	for _, call := range choice.Message.ToolCalls {
		input := json.RawMessage(call.Function.Arguments)
		if !json.Valid(input) || len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		out.Content = append(out.Content, anthropicBlock{Type: "tool_use", ID: call.ID, Name: call.Function.Name, Input: input})
	}
	if len(choice.Message.ToolCalls) > 0 {
		out.StopReason = "tool_use"
	}
	if in.Usage != nil {
		out.Usage = &anthropicUsage{InputTokens: in.Usage.PromptTokens, OutputTokens: in.Usage.CompletionTokens}
	}
	reply, err := json.Marshal(out)
	return status, reply, err
}

// AnthropicRequestFromOpenAI translates an OpenAI chat completions request
// back into an Anthropic Messages request, for the stub in the OpenAI
// format. Tool messages that follow one another become one user message of
// tool_result blocks.
func AnthropicRequestFromOpenAI(body []byte) ([]byte, error) {
	var in openAIRequest
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	out := anthropicRequest{Model: in.Model, MaxTokens: in.MaxTokens}
	var results []anthropicBlock
	flush := func() {
		if len(results) > 0 {
			content, _ := json.Marshal(results)
			out.Messages = append(out.Messages, anthropicMessage{Role: "user", Content: content})
			results = nil
		}
	}
	for _, m := range in.Messages {
		text := ""
		if m.Content != nil {
			text = *m.Content
		}
		if m.Role == "tool" {
			content, _ := json.Marshal(text)
			results = append(results, anthropicBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: content})
			continue
		}
		flush()
		if m.Role == "system" {
			out.System = text
			continue
		}
		if len(m.ToolCalls) == 0 {
			content, _ := json.Marshal(text)
			out.Messages = append(out.Messages, anthropicMessage{Role: m.Role, Content: content})
			continue
		}
		var blocks []anthropicBlock
		if text != "" {
			blocks = append(blocks, anthropicBlock{Type: "text", Text: text})
		}
		for _, call := range m.ToolCalls {
			blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: call.ID, Name: call.Function.Name, Input: json.RawMessage(call.Function.Arguments)})
		}
		content, _ := json.Marshal(blocks)
		out.Messages = append(out.Messages, anthropicMessage{Role: m.Role, Content: content})
	}
	flush()
	for _, t := range in.Tools {
		out.Tools = append(out.Tools, anthropicTool{Name: t.Function.Name, Description: t.Function.Description, InputSchema: t.Function.Parameters})
	}
	return json.Marshal(out)
}

// OpenAIReplyFromAnthropic translates an Anthropic reply and its status
// into an OpenAI chat completions reply, for the stub in the OpenAI format.
func OpenAIReplyFromAnthropic(status int, body []byte) (int, []byte, error) {
	if status != http.StatusOK {
		var e stubError
		json.Unmarshal(body, &e)
		reply, err := json.Marshal(map[string]any{"error": map[string]string{"message": e.Error.Message, "type": e.Error.Type}})
		return status, reply, err
	}
	var in anthropicReply
	if err := json.Unmarshal(body, &in); err != nil {
		return 0, nil, err
	}
	message := openAIMessage{Role: "assistant"}
	var text bytes.Buffer
	for _, b := range in.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			call := openAIToolCall{ID: b.ID, Type: "function"}
			call.Function.Name = b.Name
			call.Function.Arguments = string(b.Input)
			message.ToolCalls = append(message.ToolCalls, call)
		}
	}
	message.Content = stringPointer(text.String())
	finish := "stop"
	if in.StopReason == "tool_use" {
		finish = "tool_calls"
	}
	reply, err := json.Marshal(map[string]any{
		"id": "chatcmpl-" + in.ID, "object": "chat.completion", "model": in.Model,
		"choices": []map[string]any{{"index": 0, "message": message, "finish_reason": finish}},
	})
	return status, reply, err
}

// StubReplyInFormat answers a request in the format of a provider: an
// OpenAI request is translated to the Anthropic request the stub reads, and
// the stub's reply back to an OpenAI reply.
func StubReplyInFormat(format string, body []byte) (int, []byte, error) {
	if format != ProviderOpenAI {
		return StubReplyFor(body)
	}
	request, err := AnthropicRequestFromOpenAI(body)
	if err != nil {
		return 0, nil, err
	}
	status, reply, err := StubReplyFor(request)
	if err != nil {
		return 0, nil, err
	}
	return OpenAIReplyFromAnthropic(status, reply)
}
