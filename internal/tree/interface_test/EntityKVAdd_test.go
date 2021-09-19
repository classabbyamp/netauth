package interface_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/netauth/protocol"
)

func TestEntityKVAdd(t *testing.T) {
	m, ctx := newTreeManager(t)

	addEntity(t, ctx)

	kv1 := []*pb.KVData{{
		Key: proto.String("key1"),
		Values: []*pb.KVValue{{
			Value: proto.String("value1"),
		}},
	}}

	if err := m.EntityKVAdd("entity1", kv1); err != nil {
		t.Fatal(err)
	}

	kvtest, err := m.EntityKVGet("entity1", kv1)
	if err != nil {
		t.Fatal(err)
	}

	if !proto.Equal(kvtest[0], kv1[0]) {
		t.Error("Set a key and got different data back")
	}
}
