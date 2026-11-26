package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// providerTestRequest has every part the ports send: a system prompt, tools,
// a text message, an assistant turn with a tool call and a tool result.
const providerTestRequest = `{"model":"claude-opus-4-5","max_tokens":4096,"system":"You are terse.",` +
	`"messages":[{"role":"user","content":"read /spiffs/a.md"},` +
	`{"role":"assistant","content":[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"toolu_1","name":"read_file","input":{"path":"/spiffs/a.md"}}]},` +
	`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"hello"}]}],` +
	`"tools":[{"name":"read_file","description":"Read a file.","input_schema":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}]}`

func TestOpenAIRequestFromAnthropic(t *testing.T) {
	got, err := OpenAIRequestFromAnthropic([]byte(providerTestRequest))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"claude-opus-4-5","max_tokens":4096,"messages":[` +
		`{"role":"system","content":"You are terse."},` +
		`{"role":"user","content":"read /spiffs/a.md"},` +
		`{"role":"assistant","content":"Let me check.","tool_calls":[{"id":"toolu_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"/spiffs/a.md\"}"}}]},` +
		`{"role":"tool","content":"hello","tool_call_id":"toolu_1"}],` +
		`"tools":[{"type":"function","function":{"name":"read_file","description":"Read a file.","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}]}`
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestAnthropicRequestThroughOpenAIRoundTrip(t *testing.T) {
	openai, err := OpenAIRequestFromAnthropic([]byte(providerTestRequest))
	if err != nil {
		t.Fatal(err)
	}
	back, err := AnthropicRequestFromOpenAI(openai)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	json.Unmarshal([]byte(providerTestRequest), &a)
	json.Unmarshal(back, &b)
	// The user text comes back as a string, and the tool result content too.
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("round trip changed the request:\n%s\n%s", providerTestRequest, back)
	}
}

func TestAnthropicReplyFromOpenAI(t *testing.T) {
	reply := `{"id":"chatcmpl-1","model":"m","choices":[{"message":{"role":"assistant","content":"On it.",` +
		`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"/spiffs/a.md\"}"}}]},` +
		`"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`
	status, got, err := AnthropicReplyFromOpenAI(200, []byte(reply))
	if err != nil || status != 200 {
		t.Fatal(status, err)
	}
	want := `{"id":"chatcmpl-1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"On it."},` +
		`{"type":"tool_use","id":"call_1","name":"read_file","input":{"path":"/spiffs/a.md"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":3}}`
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	status, got, _ = AnthropicReplyFromOpenAI(429, []byte(`{"error":{"message":"slow down","type":"rate_limit"}}`))
	if status != 429 || string(got) != `{"type":"error","error":{"type":"rate_limit","message":"slow down"}}` {
		t.Fatalf("error reply: %d %s", status, got)
	}
}

func TestStoreKeyNamesProviderAndModel(t *testing.T) {
	body := []byte(providerTestRequest)
	keys := map[string]bool{
		StoreKey(ProviderAnthropic, "a", body): true,
		StoreKey(ProviderOpenAI, "a", body):    true,
		StoreKey(ProviderAnthropic, "b", body): true,
		StoreKey(ProviderOpenAI, "b", body):    true,
	}
	if len(keys) != 4 {
		t.Fatalf("%d distinct keys of 4", len(keys))
	}
}

// upstreamCounting serves the stub in format and counts the requests.
func upstreamCounting(t *testing.T, format string, count *int64, seen *[]byte) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(count, 1)
		body, _ := io.ReadAll(r.Body)
		if seen != nil {
			*seen = body
		}
		status, reply, err := StubReplyInFormat(format, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(status)
		w.Write(reply)
	}))
	t.Cleanup(server.Close)
	return server
}

func memoServerPost(t *testing.T, s *memoServer, body string) (int, []byte) {
	server := httptest.NewServer(s)
	defer server.Close()
	resp, err := http.Post(server.URL+"/v1/messages", "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reply, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, reply
}

// TestServeRecordReplayPerProvider records a request through the stub of
// each provider format, checks the port gets an Anthropic reply with the
// tool call, and replays it from the store with no upstream.
func TestServeRecordReplayPerProvider(t *testing.T) {
	request := `{"model":"claude-opus-4-5","max_tokens":1024,"system":"s","messages":[{"role":"user","content":"go [[tool read_file {\"path\":\"/spiffs/a.md\"}]]"}],` +
		`"tools":[{"name":"read_file","description":"Read a file.","input_schema":{"type":"object"}}]}`
	for _, provider := range []string{ProviderAnthropic, ProviderOpenAI} {
		t.Run(provider, func(t *testing.T) {
			var count int64
			upstream := upstreamCounting(t, provider, &count, nil)
			path := filepath.Join(t.TempDir(), "store.bolt")
			db, err := bolt.Open(path, 0o644, &bolt.Options{Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			recorder := &memoServer{db: db, memo: true, provider: provider, upstream: upstream.URL}
			status, recorded := memoServerPost(t, recorder, request)
			db.Close()
			var reply anthropicReply
			if err := json.Unmarshal(recorded, &reply); err != nil || status != 200 {
				t.Fatalf("record: %d %s %v", status, recorded, err)
			}
			if reply.StopReason != "tool_use" || len(reply.Content) != 2 || reply.Content[1].Name != "read_file" || string(reply.Content[1].Input) != `{"path":"/spiffs/a.md"}` {
				t.Fatalf("record: not the tool call: %s", recorded)
			}
			db, err = bolt.Open(path, 0o644, &bolt.Options{Timeout: time.Second, ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			replayer := &memoServer{db: db, memo: true, replay: true, provider: provider}
			status, replayed := memoServerPost(t, replayer, request)
			if status != 200 || !bytes.Equal(replayed, recorded) || count != 1 {
				t.Fatalf("replay: %d, %d upstream requests, %s", status, count, replayed)
			}
			other := &memoServer{db: db, memo: true, replay: true, provider: ProviderAnthropic, model: "other-model"}
			if status, _ := memoServerPost(t, other, request); status != http.StatusBadGateway {
				t.Fatalf("a replay with another model found the reply: %d", status)
			}
		})
	}
}

func TestServeMemoOffForwardsEveryRequest(t *testing.T) {
	var count int64
	var seen []byte
	upstream := upstreamCounting(t, ProviderAnthropic, &count, &seen)
	s := &memoServer{memo: false, provider: ProviderAnthropic, model: "claude-haiku-5-5", upstream: upstream.URL}
	request := `{"model":"claude-opus-4-5","messages":[{"role":"user","content":"hi"}]}`
	for i := 0; i < 2; i++ {
		if status, _ := memoServerPost(t, s, request); status != 200 {
			t.Fatalf("status %d", status)
		}
	}
	if count != 2 {
		t.Fatalf("%d upstream requests, want 2", count)
	}
	if RequestModel(seen) != "claude-haiku-5-5" {
		t.Fatalf("upstream got model %q, want the -model one", RequestModel(seen))
	}
}
