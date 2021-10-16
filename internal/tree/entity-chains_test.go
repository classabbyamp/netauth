package tree

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/go-hclog"

	pb "github.com/netauth/protocol"
)

func resetEntityConstructorMap() {
	eHookConstructors = make(map[string]EntityHookConstructor)
}

func TestECRegisterAndInitialize(t *testing.T) {
	resetEntityConstructorMap()
	defer resetEntityConstructorMap()

	RegisterEntityHookConstructor("null-hook", goodEntityConstructor)
	RegisterEntityHookConstructor("null-hook", goodEntityConstructor)

	if len(eHookConstructors) != 1 {
		t.Error("Duplicate hook registered")
	}

	RegisterEntityHookConstructor("bad-hook", badEntityConstructor)

	if len(eHookConstructors) != 2 {
		t.Error("bad-hook wasn't registered")
	}

	em := Manager{
		entityHooks: make(map[string]EntityHook),
		log:         hclog.NewNullLogger(),
	}

	em.InitializeEntityHooks()
	if len(em.entityHooks) != 1 {
		t.Error("bad-hook was initialized")
	}
}

func TestECInitializeChainsOK(t *testing.T) {
	resetEntityConstructorMap()
	defer resetEntityConstructorMap()

	RegisterEntityHookConstructor("null-hook", goodEntityConstructor)
	RegisterEntityHookConstructor("null-hook2", goodEntityConstructor2)
	em := Manager{
		entityHooks:     make(map[string]EntityHook),
		entityProcesses: make(map[string][]EntityHook),
		log:             hclog.NewNullLogger(),
	}
	em.InitializeEntityHooks()

	c := map[string][]string{
		"TEST": {"null-hook", "null-hook2"},
	}

	if err := em.InitializeEntityChains(c); err != nil {
		t.Error(err)
	}
}

func TestECInitializeBadHook(t *testing.T) {
	resetEntityConstructorMap()
	defer resetEntityConstructorMap()

	em := Manager{
		entityHooks:     make(map[string]EntityHook),
		entityProcesses: make(map[string][]EntityHook),
		log:             hclog.NewNullLogger(),
	}
	em.InitializeEntityHooks()

	c := map[string][]string{
		"TEST": {"unknown-hook"},
	}

	if err := em.InitializeEntityChains(c); err != ErrUnknownHook {
		t.Error(err)
	}
}

func TestECCheckRequiredMissing(t *testing.T) {
	resetEntityConstructorMap()
	defer resetEntityConstructorMap()

	em := Manager{
		entityHooks:     make(map[string]EntityHook),
		entityProcesses: make(map[string][]EntityHook),
		log:             hclog.NewNullLogger(),
	}

	if err := em.CheckRequiredEntityChains(); err != ErrUnknownHookChain {
		t.Error("Passed with a required chain missing")
	}
}

func TestECCheckRequiredEmpty(t *testing.T) {
	resetEntityConstructorMap()
	defer resetEntityConstructorMap()

	em := Manager{
		entityHooks:     make(map[string]EntityHook),
		entityProcesses: make(map[string][]EntityHook),
		log:             hclog.NewNullLogger(),
	}

	// This lets us do this without having hooks loaded, we just
	// register something into all the chains, and then kill one
	// of them at the end.
	for k := range defaultEntityChains {
		em.entityProcesses[k] = []EntityHook{
			&nullEntityHook{},
		}
	}

	em.entityProcesses["CREATE"] = nil

	if err := em.CheckRequiredEntityChains(); err != ErrEmptyHookChain {
		t.Error("Passed with an empty required chain")
	}
}

type nullEntityHook struct{}

func (*nullEntityHook) Name() string                                 { return "null-hook" }
func (*nullEntityHook) Priority() int                                { return 50 }
func (*nullEntityHook) Run(_ context.Context, _, _ *pb.Entity) error { return nil }
func goodEntityConstructor(_ RefContext) (EntityHook, error) {
	return &nullEntityHook{}, nil
}

type nullEntityHook2 struct{}

func (*nullEntityHook2) Name() string                                 { return "null-hook2" }
func (*nullEntityHook2) Priority() int                                { return 40 }
func (*nullEntityHook2) Run(_ context.Context, _, _ *pb.Entity) error { return nil }

func goodEntityConstructor2(_ RefContext) (EntityHook, error) {
	return &nullEntityHook2{}, nil
}

func badEntityConstructor(_ RefContext) (EntityHook, error) {
	return nil, errors.New("initialization error")
}
