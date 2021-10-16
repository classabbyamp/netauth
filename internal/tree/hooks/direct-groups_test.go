package hooks

import (
	"context"
	"sort"
	"testing"

	"github.com/netauth/netauth/internal/tree"

	pb "github.com/netauth/protocol"
)

func TestAddDirectGroup(t *testing.T) {
	hook, err := NewAddDirectGroup(tree.RefContext{})
	if err != nil {
		t.Fatal(err)
	}

	e := &pb.Entity{Meta: &pb.EntityMeta{}}
	de := &pb.Entity{
		Meta: &pb.EntityMeta{
			Groups: []string{
				"group1",
				"group2",
				"group1",
			},
		},
	}

	if err := hook.Run(context.Background(), e, de); err != nil {
		t.Fatal(err)
	}

	sort.Strings(e.Meta.Groups)
	if len(e.Meta.Groups) != 2 || e.Meta.Groups[0] != "group1" || e.Meta.Groups[1] != "group2" {
		t.Log(e)
		t.Error("Spec Error - please trace hook")
	}
}

func TestDelDirectGroup(t *testing.T) {
	hook, err := NewDelDirectGroup(tree.RefContext{})
	if err != nil {
		t.Fatal(err)
	}

	e := &pb.Entity{
		Meta: &pb.EntityMeta{
			Groups: []string{
				"group1",
				"group2",
			},
		},
	}
	de := &pb.Entity{
		Meta: &pb.EntityMeta{
			Groups: []string{
				"group1",
			},
		},
	}

	if err := hook.Run(context.Background(), e, de); err != nil {
		t.Fatal(err)
	}

	if len(e.Meta.Groups) != 1 || e.Meta.Groups[0] != "group2" {
		t.Log(e)
		t.Error("Spec error - please trace hook")
	}
}
