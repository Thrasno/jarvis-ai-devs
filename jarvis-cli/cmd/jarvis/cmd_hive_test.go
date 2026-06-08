package main

import (
	"testing"
)

func TestHiveCmd_UseAndShortAreSet(t *testing.T) {
	if hiveCmd.Use != "hive" {
		t.Fatalf("hiveCmd.Use = %q, want %q", hiveCmd.Use, "hive")
	}
	if hiveCmd.Short == "" {
		t.Fatal("hiveCmd.Short is empty, want a non-empty description")
	}
}

func TestHiveCmd_DaemonURLFlag(t *testing.T) {
	f := hiveCmd.Flags().Lookup("daemon-url")
	if f == nil {
		t.Fatal("hiveCmd missing --daemon-url flag")
	}
	if f.DefValue != "" {
		t.Fatalf("--daemon-url default = %q, want empty string", f.DefValue)
	}
}

func TestHiveCmd_RegisteredOnRootCmd(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "hive" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("hiveCmd is not registered on rootCmd")
	}
}
