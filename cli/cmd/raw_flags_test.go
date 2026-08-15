package cmd

import (
	"reflect"
	"testing"
)

func preserveRawFlagState(t *testing.T) {
	t.Helper()
	oldProjectDir := flagProjectDir
	oldVerbose := flagVerbose
	t.Cleanup(func() {
		flagProjectDir = oldProjectDir
		flagVerbose = oldVerbose
	})
}

func TestConsumeRawPersistentFlagsBeforeDebugCommand(t *testing.T) {
	preserveRawFlagState(t)
	flagProjectDir = ""
	flagVerbose = false

	got, err := consumeRawPersistentFlags([]string{"--project-dir", "/game", "--verbose", "input", "actions"})
	if err != nil {
		t.Fatalf("consumeRawPersistentFlags() error = %v", err)
	}
	if want := []string{"input", "actions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining args = %q, want %q", got, want)
	}
	if flagProjectDir != "/game" {
		t.Fatalf("flagProjectDir = %q, want /game", flagProjectDir)
	}
	if !flagVerbose {
		t.Fatal("flagVerbose = false, want true")
	}
}

func TestConsumeRawPersistentFlagsEqualsForm(t *testing.T) {
	preserveRawFlagState(t)
	flagProjectDir = ""
	flagVerbose = true

	got, err := consumeRawPersistentFlags([]string{"--project-dir=/game", "--verbose=false", "screenshot", "--width", "320"})
	if err != nil {
		t.Fatalf("consumeRawPersistentFlags() error = %v", err)
	}
	if want := []string{"screenshot", "--width", "320"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining args = %q, want %q", got, want)
	}
	if flagProjectDir != "/game" || flagVerbose {
		t.Fatalf("root flags = (%q, %v), want (/game, false)", flagProjectDir, flagVerbose)
	}
}

func TestConsumeRawPersistentFlagsPreservesProjectArguments(t *testing.T) {
	preserveRawFlagState(t)
	flagProjectDir = "original"

	got, err := consumeRawPersistentFlags([]string{"custom-command", "--path", "project-owned"})
	if err != nil {
		t.Fatalf("consumeRawPersistentFlags() error = %v", err)
	}
	if want := []string{"custom-command", "--path", "project-owned"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining args = %q, want %q", got, want)
	}
	if flagProjectDir != "original" {
		t.Fatalf("flagProjectDir changed to %q", flagProjectDir)
	}
}

func TestConsumeRawPersistentFlagsRejectsMissingValue(t *testing.T) {
	preserveRawFlagState(t)
	if _, err := consumeRawPersistentFlags([]string{"--project-dir"}); err == nil {
		t.Fatal("expected missing project directory value to fail")
	}
}
