package exitcode

import (
	"errors"
	"fmt"
	"testing"
)

func TestOfNilIsSuccess(t *testing.T) {
	if Of(nil) != Success {
		t.Fatal("nil should be Success")
	}
}

func TestOfPlainErrorIsFailure(t *testing.T) {
	if Of(errors.New("boom")) != Failure {
		t.Fatal("plain error should be Failure")
	}
}

func TestOfWrappedCode(t *testing.T) {
	err := fmt.Errorf("outer: %w", New(Failure, errors.New("inner")))
	if Of(err) != Failure {
		t.Fatal("wrapped code should surface")
	}
}

func TestNewCarriesExitCode(t *testing.T) {
	e := New(Failure, errors.New("x"))
	var c *codedError
	if !errors.As(e, &c) {
		t.Fatal("expected codedError")
	}
	if c.ExitCode() != Failure {
		t.Fatalf("ExitCode = %d", c.ExitCode())
	}
}

func TestOfClusterCode(t *testing.T) {
	if Of(New(Cluster, errors.New("rbac"))) != Cluster {
		t.Fatal("cluster code should surface")
	}
}

func TestCodedErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	err := New(Cluster, inner)
	if !errors.Is(err, inner) {
		t.Fatal("Unwrap/Is should find inner")
	}
}
