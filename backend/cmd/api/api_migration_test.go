package main

import "testing"

func TestPermissionSeedsContainElementCapturePermissions(t *testing.T) {
	seedCodes := permissionSeedCodes()
	for _, want := range []string{
		"ui.element.read",
		"ui.element.capture",
		"ui.element.manage",
	} {
		if !seedCodes[want] {
			t.Fatalf("权限种子缺少 %q", want)
		}
	}
}
