package provider

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/bitwarden/sdk-go/v2"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestRequestGateSerializesCallbacks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(requestGate, 1)
		entered := make(chan struct{}, 16)
		release := make(chan struct{})
		var wg sync.WaitGroup
		for range cap(entered) {
			wg.Go(func() {
				if err := gate.Do(context.Background(), func() error {
					entered <- struct{}{}
					<-release
					return nil
				}); err != nil {
					t.Error(err)
				}
			})
		}
		for range cap(entered) {
			// All workers are blocked, either inside the callback or waiting
			// for the gate. Exactly one callback may have entered.
			synctest.Wait()
			if got := len(entered); got != 1 {
				t.Fatalf("concurrent callbacks entered: got %d, want 1", got)
			}
			<-entered
			release <- struct{}{}
		}
		wg.Wait()
	})
}

func TestRequestGateCancellationWhileWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(requestGate, 1)
		release := make(chan struct{})
		go func() {
			if err := gate.Do(context.Background(), func() error {
				<-release
				return nil
			}); err != nil {
				t.Error(err)
			}
		}()
		synctest.Wait()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() {
			result <- gate.Do(ctx, func() error {
				t.Error("canceled callback invoked")
				return nil
			})
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case err := <-result:
			if err != context.Canceled {
				t.Fatalf("got %v, want context.Canceled", err)
			}
		default:
			t.Fatal("canceled waiter did not return while gate was held")
		}
		close(release)
	})
}

func TestRequestGateCanceledContext(t *testing.T) {
	gate := make(requestGate, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 100 {
		if err := gate.Do(ctx, func() error {
			t.Error("canceled callback invoked with available gate")
			return nil
		}); err != context.Canceled {
			t.Fatalf("got %v, want context.Canceled", err)
		}
		if len(gate) != 0 {
			t.Fatal("canceled call did not release gate")
		}
	}
}

func TestRequestGateErrorReleasesGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(requestGate, 1)
		want := errors.New("SDK error")
		if err := gate.Do(context.Background(), func() error { return want }); err != want {
			t.Fatalf("error identity changed: got %v, want %v", err, want)
		}
		called := false
		if err := gate.Do(context.Background(), func() error {
			called = true
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("subsequent callback was not invoked")
		}
	})
}

func TestRequestGateIndependentInstances(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first := make(requestGate, 1)
		second := make(requestGate, 1)
		release := make(chan struct{})
		go func() {
			if err := first.Do(context.Background(), func() error {
				<-release
				return nil
			}); err != nil {
				t.Error(err)
			}
		}()
		synctest.Wait()
		if err := second.Do(context.Background(), func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		close(release)
	})
}

func TestRequestGateSharedByResourcesAndDataSources(t *testing.T) {
	gate := make(requestGate, 1)
	data := BitwardenSecretsManagerProviderDataStruct{
		bitwardenClient: &sdk.BitwardenClient{},
		requestGate:     gate,
		organizationId:  "test-organization",
	}
	ctx := context.Background()
	r := &secretResource{}
	var resourceResp resource.ConfigureResponse
	r.Configure(ctx, resource.ConfigureRequest{ProviderData: data}, &resourceResp)
	if resourceResp.Diagnostics.HasError() || r.requestGate != gate {
		t.Fatal("resource did not receive the shared gate")
	}
	secret := &secretDataSource{}
	list := &listSecretsDataSource{}
	projects := &projectsDataSource{}
	for _, source := range []datasource.DataSourceWithConfigure{secret, list, projects} {
		var resp datasource.ConfigureResponse
		source.Configure(ctx, datasource.ConfigureRequest{ProviderData: data}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
	}
	if secret.requestGate != gate || list.requestGate != gate || projects.requestGate != gate {
		t.Fatal("data sources did not receive the shared gate")
	}
}
