package cloudmeta

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestParsers(t *testing.T) {
	id, name := ParseLoadCodeAssist([]byte(`{"currentTier":{"id":"free","name":"Free"},"paidTier":{"id":"pro","name":"Pro"}}`))
	if id != "pro" || name != "Pro" {
		t.Fatalf("tier = %q, %q", id, name)
	}
	if got := ParseUserInfo([]byte(`{"regionCode":" TW "}`)); got != "TW" {
		t.Fatalf("region = %q", got)
	}
	model, tags := ParseAvailableModels([]byte(`{"defaultAgentModelId":"m1","models":{"m1":{"tagTitle":"New"},"m2":{}}}`))
	if model != "m1" || tags["m1"] != "New" || len(tags) != 1 {
		t.Fatalf("models = %q %#v", model, tags)
	}
}

func TestCollectorConcurrent(t *testing.T) {
	c := NewCollector()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Observe("fetchAvailableModels", []byte(`{"defaultAgentModelId":"m","models":{"m":{"tagTitle":"New"}}}`))
		}()
	}
	wg.Wait()
	got := c.Snapshot()
	if got.DefaultModel != "m" || got.ModelTags["m"] != "New" {
		t.Fatalf("snapshot = %#v", got)
	}
	got.ModelTags["x"] = "mutated"
	if c.Snapshot().ModelTags["x"] != "" {
		t.Fatal("snapshot aliases collector map")
	}
}

func TestRelayForwardsAndCollects(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Accept-Encoding") != "" || r.Host == "" {
			t.Errorf("request not sanitized: method=%s host=%q encoding=%q", r.Method, r.Host, r.Header.Get("Accept-Encoding"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "{}" {
			t.Errorf("body = %q", body)
		}
		w.Header().Set("Connection", "close")
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"regionCode":"US"}`))
	}))
	defer upstream.Close()
	r, err := Start(upstream.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	req, _ := http.NewRequest(http.MethodPost, r.URL+"/v1internal:fetchUserInfo", strings.NewReader("{}"))
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	if string(data) != `{"regionCode":"US"}` || resp.Header.Get("Content-Length") != "19" || resp.Header.Get("Connection") != "" {
		t.Fatalf("response = %d %q headers=%v", resp.StatusCode, data, resp.Header)
	}
	if got := r.Meta().RegionCode; got != "US" {
		t.Fatalf("region = %q", got)
	}
}

func TestRelayRejectsMethodAndHost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer upstream.Close()
	r, err := Start(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, tc := range []struct {
		method, host string
		status       int
	}{{http.MethodPut, "", http.StatusMethodNotAllowed}, {http.MethodGet, "evil.example", http.StatusForbidden}} {
		req, _ := http.NewRequest(tc.method, r.URL+"/", nil)
		if tc.host != "" {
			req.Host = tc.host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("%s/%s status = %d", tc.method, tc.host, resp.StatusCode)
		}
	}
}

func TestCloudMetaJSONShape(t *testing.T) {
	b, err := json.Marshal(CloudMeta{TierID: "x", ModelTags: map[string]string{"m": "tag"}})
	if err != nil || !strings.Contains(string(b), "TierID") {
		t.Fatalf("marshal = %s, err=%v", b, err)
	}
}
