package calsync

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestWaitForCodeSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	resCh := make(chan struct {
		code string
		err  error
	}, 1)
	go func() {
		code, err := waitForCode(context.Background(), ln, "want-state")
		resCh <- struct {
			code string
			err  error
		}{code, err}
	}()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?state=want-state&code=abc123", port))
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	resp.Body.Close()

	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("waitForCode err = %v, want nil", res.err)
		}
		if res.code != "abc123" {
			t.Errorf("waitForCode code = %q, want abc123", res.code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForCode did not return after a valid callback")
	}
}

func TestWaitForCodeStateMismatch(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	errCh := make(chan error, 1)
	go func() {
		_, err := waitForCode(context.Background(), ln, "want-state")
		errCh <- err
	}()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?state=wrong-state&code=abc123", port))
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("waitForCode err = nil, want error for state mismatch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForCode did not return after a state-mismatched callback")
	}
}

func TestWaitForCodeAuthorizationDenied(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	errCh := make(chan error, 1)
	go func() {
		_, err := waitForCode(context.Background(), ln, "want-state")
		errCh <- err
	}()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?state=want-state&error=access_denied", port))
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("waitForCode err = nil, want error for denied authorization")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForCode did not return after a denied callback")
	}
}

func TestWaitForCodeTimesOut(t *testing.T) {
	orig := connectTimeout
	connectTimeout = 50 * time.Millisecond
	defer func() { connectTimeout = orig }()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	_, err = waitForCode(context.Background(), ln, "want-state")
	if err == nil {
		t.Fatal("waitForCode err = nil, want timeout error")
	}
}

func TestWaitForCodeRespectsContextCancellation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := waitForCode(ctx, ln, "want-state")
		errCh <- err
	}()
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("waitForCode err = nil, want context.Canceled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForCode did not return after context cancellation")
	}
}
