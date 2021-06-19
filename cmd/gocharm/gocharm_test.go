package main

import (
	"testing"
)

func Test_getLocalPathToGoModule(t *testing.T)  {
	if getLocalPathToGoModule("example.org/hw", "example.org/hw",
		"/home/user/gocharms/hw") != "/home/user/gocharms/hw" {
		t.Fail()
	}
}

func Test_getLocalPathToGoModule_otherImportPath(t *testing.T)  {
	if getLocalPathToGoModule("example.org/foo", "example.org/foo/charms/bar",
		"/home/user/go/src/example.org/foo/charms/bar") != "/home/user/go/src/example.org/foo" {
		t.Fail()
	}
}