package hiveclient_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

func TestApplyProgressResultReportsEffectiveProjectKey(t *testing.T) {
	fixture := string(sharedApplyProgressGetFixture(t))
	withoutKey := strings.Replace(fixture, `"project_key":"project",`, "", 1)
	if withoutKey == fixture {
		t.Fatal("shared fixture no longer carries project_key")
	}
	for _, tt := range []struct {
		name      string
		status    int
		body      string
		requested projectidentity.Key
		want      projectidentity.Key
		wantErr   bool
	}{
		{name: "daemon key on success", status: http.StatusOK, body: strings.Replace(fixture, `"project_key":"project"`, `"project_key":"target"`, 1), requested: "retired", want: "target"},
		{name: "older daemon falls back to requested key", status: http.StatusOK, body: withoutKey, requested: "project", want: "project"},
		{name: "daemon key on typed not found", status: http.StatusNotFound, body: `{"outcome":"not_found","code":"not_found","recovery":"initialize apply progress before advancing","project_key":"target"}`, requested: "retired", want: "target", wantErr: true},
		{name: "older typed not found falls back", status: http.StatusNotFound, body: `{"outcome":"not_found","code":"not_found"}`, requested: "project", want: "project", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			result, err := client.GetApplyProgress(context.Background(), tt.requested.String(), "change")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %t", err, tt.wantErr)
			}
			if got := result.EffectiveProjectKey(tt.requested); got != tt.want {
				t.Fatalf("effective key = %q, want %q", got, tt.want)
			}
			var typed *hiveclient.ApplyProgressError
			if tt.wantErr && (!errors.As(err, &typed) || typed.Result.EffectiveProjectKey(tt.requested) != tt.want) {
				t.Fatalf("typed error = %#v, want effective key %q", err, tt.want)
			}
		})
	}
}

func TestSDDStoreBindingReportsDaemonRedirectAsTypedEffectiveKey(t *testing.T) {
	created := true
	for _, tt := range []struct {
		name string
		call func(*hiveclient.Client) error
		body string
	}{
		{name: "get", body: sddStoreBindingResponse("target", "change", "1", "hive", "cli", "2026-08-01T10:00:00Z", nil), call: func(client *hiveclient.Client) error {
			_, _, err := client.GetSDDStoreBinding(context.Background(), "Retired", "change")
			return err
		}},
		{name: "adopt", body: sddStoreBindingResponse("target", "change", "1", "hive", "cli", "2026-08-01T10:00:00Z", &created), call: func(client *hiveclient.Client) error {
			_, _, err := client.AdoptSDDStoreBinding(context.Background(), "Retired", "change", hiveclient.SDDStoreBindingRequest{Mode: hiveclient.SDDStoreModeHive, Provenance: "cli"})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.name == "adopt" {
					w.WriteHeader(http.StatusCreated)
				}
				_, _ = w.Write([]byte(tt.body))
			})
			err := tt.call(client)
			var redirect *hiveclient.SDDStoreBindingRedirectError
			if !errors.As(err, &redirect) {
				t.Fatalf("error = %#v, want typed redirect", err)
			}
			if redirect.Requested != "retired" || redirect.Effective != "target" || redirect.Binding.Project != "target" {
				t.Fatalf("redirect = %#v", redirect)
			}
		})
	}
}

func TestSDDStoreBindingRejectsNonCanonicalEffectiveKey(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sddStoreBindingResponse("Target", "change", "1", "hive", "cli", "2026-08-01T10:00:00Z", nil)))
	})
	_, _, err := client.GetSDDStoreBinding(context.Background(), "project", "change")
	var redirect *hiveclient.SDDStoreBindingRedirectError
	var protocolErr *hiveclient.SDDStoreBindingProtocolError
	if errors.As(err, &redirect) || !errors.As(err, &protocolErr) {
		t.Fatalf("error = %#v, want protocol error without redirect", err)
	}
}

func TestSDDStoreBindingCanonicalSpellingWithoutAliasStillResolves(t *testing.T) {
	client := newSDDClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("project"); got != "my-project" {
			t.Errorf("project query = %q", got)
		}
		_, _ = w.Write([]byte(sddStoreBindingResponse("my-project", "change", "1", "hive", "cli", "2026-08-01T10:00:00Z", nil)))
	})
	binding, found, err := client.GetSDDStoreBinding(context.Background(), "my-project", "change")
	if err != nil || !found || binding.Project != "my-project" {
		t.Fatalf("binding=%#v found=%t err=%v", binding, found, err)
	}
}
