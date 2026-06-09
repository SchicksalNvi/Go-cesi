package models

import "testing"

func TestUserCanAccessNodeWithoutACLReturnsTrue(t *testing.T) {
	user := &User{}

	actions := []string{"read", "write", "execute", "delete", "log"}
	for _, action := range actions {
		if !user.CanAccessNode(1, action) {
			t.Fatalf("expected action %q to be allowed when no node ACL exists", action)
		}
	}
}

func TestUserCanAccessNodeWithACLRestrictsByNodeAndAction(t *testing.T) {
	user := &User{
		NodeAccess: []NodeAccess{
			{NodeID: 1, CanRead: true, CanWrite: false, CanDelete: false},
			{NodeID: 2, CanRead: true, CanWrite: true, CanDelete: false},
		},
	}

	if !user.CanAccessNode(1, "read") {
		t.Fatal("expected read access on node 1")
	}
	if user.CanAccessNode(1, "write") {
		t.Fatal("did not expect write access on node 1")
	}
	if user.CanAccessNode(1, "execute") {
		t.Fatal("did not expect execute access on node 1")
	}
	if !user.CanAccessNode(2, "execute") {
		t.Fatal("expected execute access on node 2")
	}
	if user.CanAccessNode(3, "read") {
		t.Fatal("did not expect access on unrelated node")
	}
}

func TestUserCanAccessNodeSuperAdminBypassesACL(t *testing.T) {
	user := &User{
		IsAdmin: true,
		NodeAccess: []NodeAccess{
			{NodeID: 1, CanRead: false, CanWrite: false, CanDelete: false},
		},
	}

	if !user.CanAccessNode(1, "delete") {
		t.Fatal("expected super admin to bypass node ACL")
	}
	if !user.CanAccessNode(99, "read") {
		t.Fatal("expected super admin to access any node")
	}
}
