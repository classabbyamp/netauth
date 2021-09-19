package interface_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/netauth/protocol"
)

func TestUpdateGroupMeta(t *testing.T) {
	m, ctx := newTreeManager(t)

	addGroup(t, ctx)

	update := &pb.Group{
		DisplayName: proto.String("SomeGroup"),
	}

	if err := m.UpdateGroupMeta("group1", update); err != nil {
		t.Fatal(err)
	}

	g, err := ctx.DB.LoadGroup("group1")
	if err != nil {
		t.Fatal(err)
	}

	if g.GetDisplayName() != "SomeGroup" {
		t.Error("Group metadata not updated")
	}
}
