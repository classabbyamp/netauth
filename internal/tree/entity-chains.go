package tree

import (
	"context"
	"sort"

	pb "github.com/netauth/protocol"
)

// EntityHookConstructor functions construct EntityHook instances and
// return the hooks for registration into the map of hooks.  This
// allows the hooks to notify the module of their presence and defer
// construction until a RefContext can be prepared.
type EntityHookConstructor func(RefContext) (EntityHook, error)

// An EntityHook is a function that transforms an entity as
// part of an EntityProcessor pipeline.
type EntityHook interface {
	Priority() int
	Name() string
	Run(context.Context, *pb.Entity, *pb.Entity) error
}

var (
	eHookConstructors map[string]EntityHookConstructor
)

func init() {
	eHookConstructors = make(map[string]EntityHookConstructor)
}

// RegisterEntityHookConstructor registers the entity hook
// constructors to be called during the initialization of the main
// tree manager.
func RegisterEntityHookConstructor(name string, c EntityHookConstructor) {
	if _, ok := eHookConstructors[name]; ok {
		// Already registered
		log().Trace("Duplicate EntityHookConstructor registration attempt", "hook", name)
		return
	}
	eHookConstructors[name] = c
	log().Trace("EntityHookConstructor registered", "constructor", name)
}

// InitializeEntityHooks runs all the EntityHookConstructors and
// registers the resulting hooks by name into m.entityProcessorHooks
func (m *Manager) InitializeEntityHooks() {
	m.log.Debug("Executing EntityHookConstructor callbacks")
	for _, v := range eHookConstructors {
		hook, err := v(m.refContext)
		if err != nil {
			m.log.Warn("Error initializing hook", "hook", hook, "error", err)
			continue
		}
		m.entityHooks[hook.Name()] = hook
		m.log.Trace("EntityHook registered", "hook", hook.Name())
	}
}

// InitializeEntityChains initializes the map of chains stored on the
// manager.  It is expected that any merging of an external
// configuration has happened before this function is called.
func (m *Manager) InitializeEntityChains(c ChainConfig) error {
	for chain, hooks := range c {
		m.log.Debug("Initializing Entity Chain", "chain", chain)
		for _, h := range hooks {
			if err := m.RegisterEntityHookToChain(h, chain); err != nil {
				return err
			}
		}
	}
	return nil
}

// RegisterEntityHookToChain registers a hook to a given chain.
func (m *Manager) RegisterEntityHookToChain(hook, chain string) error {
	eph, ok := m.entityHooks[hook]
	if !ok {
		m.log.Warn("Missing hook during chain initializtion", "chain", chain, "hook", hook)
		return ErrUnknownHook
	}
	m.entityProcesses[chain] = append(m.entityProcesses[chain], eph)
	sort.Slice(m.entityProcesses[chain], func(i, j int) bool {
		return m.entityProcesses[chain][i].Priority() < m.entityProcesses[chain][j].Priority()
	})
	m.log.Trace("Registered hook to chain", "chain", chain, "hook", hook)
	return nil
}

// CheckRequiredEntityChains searches for all chains in the default
// chains list and logs a fatal error if one isn't found in the
// configured list.  This allows the system to later assert the
// presence of chains without checking, since they cannot be modified
// after loading.
func (m *Manager) CheckRequiredEntityChains() error {
	for k := range defaultEntityChains {
		if _, ok := m.entityProcesses[k]; !ok {
			m.log.Error("Missing required chain", "chain", k)
			return ErrUnknownHookChain
		}
		if len(m.entityProcesses[k]) == 0 {
			m.log.Error("A required chain is empty", "chain", k)
			return ErrEmptyHookChain
		}
	}
	return nil
}

// RunEntityChain runs the specified chain with de specifying values
// to be consumed by the chain.
func (m *Manager) RunEntityChain(ctx context.Context, chain string, de *pb.Entity) (*pb.Entity, error) {
	e := new(pb.Entity)
	hookChain := m.entityProcesses[chain]
	for _, h := range hookChain {
		m.log.Trace("Executing entity hook", "chain", chain, "hook", h.Name())
		if err := h.Run(ctx, e, de); err != nil {
			m.log.Trace("Error during chain execution", "chain", chain, "hook", h.Name(), "error", err)
			return nil, err
		}
	}
	return e, nil
}
