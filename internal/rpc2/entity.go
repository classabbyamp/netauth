package rpc2

import (
	"context"

	"github.com/netauth/netauth/internal/db"
	"github.com/netauth/netauth/internal/tree"

	types "github.com/netauth/protocol"
	pb "github.com/netauth/protocol/v2"
)

// EntityCreate creates entities.  This call will validate that a
// correct token is held, which must contain either CREATE_ENTITY or
// GLOBAL_ROOT permissions.
func (s *Server) EntityCreate(ctx context.Context, r *pb.EntityRequest) (*pb.Empty, error) {
	if err := s.mutablePrequisitesMet(ctx, types.Capability_CREATE_ENTITY); err != nil {
		return &pb.Empty{}, err
	}

	e := r.GetEntity()
	switch err := s.CreateEntity(ctx, e.GetID(), e.GetNumber(), e.GetSecret()); err {
	case tree.ErrDuplicateEntityID, tree.ErrDuplicateNumber:
		s.log.Warn("Attempt to create duplicate entity",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrExists
	case nil:
		s.log.Info("Entity Created",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, nil
	default:
		s.log.Warn("Error Creating Entity",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrInternal
	}
}

// EntityUpdate provides a change to specific entity metadata that is
// in the typed data fields.  This method does not update keys,
// groups, untyped metadata, or capabilities.  To call this method you
// must be in possession of a token with MODIFY_ENTITY_META
// capabilities.
func (s *Server) EntityUpdate(ctx context.Context, r *pb.EntityRequest) (*pb.Empty, error) {
	if err := s.mutablePrequisitesMet(ctx, types.Capability_MODIFY_ENTITY_META); err != nil {
		return &pb.Empty{}, err
	}

	de := r.GetData()
	switch err := s.UpdateEntityMeta(ctx, de.GetID(), de.GetMeta()); err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUpdate",
			"entity", de.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist

	case nil:
		s.log.Info("Entity Updated",
			"entity", de.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, nil
	default:
		s.log.Warn("Error Updating Entity",
			"entity", de.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrInternal
	}
}

// EntityInfo provides information on a single entity.  The list
// returned is guaranteed to be of length 1.
func (s *Server) EntityInfo(ctx context.Context, r *pb.EntityRequest) (*pb.ListOfEntities, error) {
	e := r.GetEntity()

	switch ent, err := s.FetchEntity(ctx, e.GetID()); err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUpdate",
			"entity", e.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.ListOfEntities{}, ErrDoesNotExist
	case nil:
		s.log.Info("Dumped Entity Info",
			"entity", e.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.ListOfEntities{Entities: []*types.Entity{ent}}, nil
	default:
		s.log.Warn("Error fetching entity",
			"entity", e.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.ListOfEntities{}, ErrInternal
	}
}

// EntitySearch searches all entities and returns the entities that
// had been found.
func (s *Server) EntitySearch(ctx context.Context, r *pb.SearchRequest) (*pb.ListOfEntities, error) {
	expr := r.GetExpression()

	res, err := s.SearchEntities(ctx, db.SearchRequest{Expression: expr})
	if err != nil {
		s.log.Warn("Search Error",
			"expr", expr,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.ListOfEntities{}, ErrInternal
	}

	return &pb.ListOfEntities{Entities: res}, nil
}

// EntityUM handles both updates, and reads to the untyped metadata
// that's stored on Entities.
func (s *Server) EntityUM(ctx context.Context, r *pb.KVRequest) (*pb.ListOfStrings, error) {
	if r.GetAction() != pb.Action_READ &&
		r.GetAction() != pb.Action_UPSERT &&
		r.GetAction() != pb.Action_CLEAREXACT &&
		r.GetAction() != pb.Action_CLEARFUZZY {
		return &pb.ListOfStrings{}, ErrMalformedRequest
	}

	if r.GetAction() != pb.Action_READ {
		if s.readonly {
			s.log.Warn("Mutable request in read-only mode!",
				"method", "EntityUM",
				"client", getClientName(ctx),
				"service", getServiceName(ctx),
			)
			return &pb.ListOfStrings{}, ErrReadOnly
		}

		// Token validation and authorization
		var err error
		ctx, err = s.checkToken(ctx)
		if err != nil {
			return &pb.ListOfStrings{}, err
		}
		if err := s.isAuthorized(ctx, types.Capability_MODIFY_ENTITY_META); err != nil {
			return &pb.ListOfStrings{}, err
		}
	}

	// At this point, we're either in a read-only query, or in a
	// write one that has been authorized.
	meta, err := s.ManageUntypedEntityMeta(ctx, r.GetTarget(), r.GetAction().String(), r.GetKey(), r.GetValue())
	switch err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.ListOfStrings{}, ErrDoesNotExist
	case nil:
		s.log.Info("Entity Updated",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.ListOfStrings{Strings: meta}, nil
	default:
		s.log.Warn("Error Updating Entity",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.ListOfStrings{}, ErrInternal
	}
}

// EntityKVGet returns key/value data from a single entity.
func (s *Server) EntityKVGet(ctx context.Context, r *pb.KV2Request) (*pb.ListOfKVData, error) {
	res, err := s.Manager.EntityKVGet(ctx, r.GetTarget(), []*types.KVData{r.GetData()})
	out := &pb.ListOfKVData{KVData: res}
	switch err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return out, ErrDoesNotExist
	case tree.ErrNoSuchKey:
		s.log.Warn("Key does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return out, ErrDoesNotExist
	case nil:
		s.log.Info("Entity KV Data Dumped",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return out, nil
	default:
		s.log.Warn("Error Loading Entity",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return out, ErrInternal
	}
}

// EntityKVAdd takes the input KV2 data and adds it to an entity if an
// only if it does not conflict with an existing key.
func (s *Server) EntityKVAdd(ctx context.Context, r *pb.KV2Request) (*pb.Empty, error) {
	if err := s.mutablePrequisitesMet(ctx, types.Capability_MODIFY_ENTITY_META); err != nil {
		return &pb.Empty{}, err
	}

	err := s.Manager.EntityKVAdd(ctx, r.GetTarget(), []*types.KVData{r.GetData()})
	switch err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist
	case tree.ErrKeyExists:
		s.log.Warn("Error Updating Entity",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrExists
	case nil:
		s.log.Info("Entity KV Updated",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, nil
	default:
		s.log.Warn("Error Updating Entity",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrInternal
	}
}

// EntityKVDel removes an existing key from an entity.  If the key is
// not present an error will be returned.
func (s *Server) EntityKVDel(ctx context.Context, r *pb.KV2Request) (*pb.Empty, error) {
	if err := s.mutablePrequisitesMet(ctx, types.Capability_MODIFY_ENTITY_META); err != nil {
		return &pb.Empty{}, err
	}

	err := s.Manager.EntityKVDel(ctx, r.GetTarget(), []*types.KVData{r.GetData()})
	switch err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist
	case tree.ErrNoSuchKey:
		s.log.Warn("Key does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist
	case nil:
		s.log.Info("Entity KV Data Dumped",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, nil
	default:
		s.log.Warn("Error Updating Entity",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrInternal
	}
}

// EntityKVReplace replaces an existing key with new values provided.
// The key must already exist on the entity or an error will be
// returned.
func (s *Server) EntityKVReplace(ctx context.Context, r *pb.KV2Request) (*pb.Empty, error) {
	if err := s.mutablePrequisitesMet(ctx, types.Capability_MODIFY_ENTITY_META); err != nil {
		return &pb.Empty{}, err
	}

	err := s.Manager.EntityKVReplace(ctx, r.GetTarget(), []*types.KVData{r.GetData()})
	switch err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist
	case tree.ErrNoSuchKey:
		s.log.Warn("Key does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist
	case nil:
		s.log.Info("Entity KV Data Updated",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, nil
	default:
		s.log.Warn("Error Updating Entity",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrInternal
	}
}

// EntityKeys handles updates and reads to keys for entities.
func (s *Server) EntityKeys(ctx context.Context, r *pb.KVRequest) (*pb.ListOfStrings, error) {
	if r.GetAction() != pb.Action_READ &&
		r.GetAction() != pb.Action_ADD &&
		r.GetAction() != pb.Action_DROP {
		return &pb.ListOfStrings{}, ErrMalformedRequest
	}

	if r.GetAction() != pb.Action_READ {
		if s.readonly {
			s.log.Warn("Mutable request in read-only mode!",
				"method", "EntityUM",
				"client", getClientName(ctx),
				"service", getServiceName(ctx),
			)
			return &pb.ListOfStrings{}, ErrReadOnly
		}

		// Token validation and authorization
		var err error
		ctx, err = s.checkToken(ctx)
		if err != nil {
			return &pb.ListOfStrings{}, err
		}
		err = s.isAuthorized(ctx, types.Capability_MODIFY_ENTITY_KEYS)
		if err != nil && getTokenClaims(ctx).EntityID != r.GetTarget() {
			return &pb.ListOfStrings{}, err
		}
	}

	// At this point, we're either in a read-only query, or in a
	// write one that has been authorized.
	keys, err := s.UpdateEntityKeys(ctx, r.GetTarget(), r.GetAction().String(), r.GetKey(), r.GetValue())
	switch err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUM",
			"entity", r.GetTarget(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.ListOfStrings{}, ErrDoesNotExist
	case nil:
		s.log.Info("Entity Updated",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.ListOfStrings{Strings: keys}, nil
	default:
		s.log.Warn("Error Updating Entity",
			"entity", r.GetTarget(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.ListOfStrings{}, ErrInternal
	}
}

// EntityDestroy will remove an entity from the system.  This is
// generally discouraged, but if you must then this function will do
// it.
func (s *Server) EntityDestroy(ctx context.Context, r *pb.EntityRequest) (*pb.Empty, error) {
	if err := s.mutablePrequisitesMet(ctx, types.Capability_DESTROY_ENTITY); err != nil {
		return &pb.Empty{}, err
	}

	e := r.GetEntity()
	switch err := s.DestroyEntity(ctx, e.GetID()); err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityDestroy",
			"entity", e.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist
	case nil:
		s.log.Info("Entity Updated",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, nil
	default:
		s.log.Warn("Error Updating Entity",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrInternal
	}
}

// EntityLock sets the lock flag on an entity.
func (s *Server) EntityLock(ctx context.Context, r *pb.EntityRequest) (*pb.Empty, error) {
	if err := s.mutablePrequisitesMet(ctx, types.Capability_LOCK_ENTITY); err != nil {
		return &pb.Empty{}, err
	}

	e := r.GetEntity()
	switch err := s.LockEntity(ctx, e.GetID()); err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityLock",
			"entity", e.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist
	case nil:
		s.log.Info("Entity Locked",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, nil
	default:
		s.log.Warn("Error Locking Entity",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrInternal
	}
}

// EntityUnlock clears the lock flag on an entity.
func (s *Server) EntityUnlock(ctx context.Context, r *pb.EntityRequest) (*pb.Empty, error) {
	if err := s.mutablePrequisitesMet(ctx, types.Capability_UNLOCK_ENTITY); err != nil {
		return &pb.Empty{}, err
	}

	e := r.GetEntity()
	switch err := s.UnlockEntity(ctx, e.GetID()); err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityUnlock",
			"entity", e.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.Empty{}, ErrDoesNotExist
	case nil:
		s.log.Info("Entity Unlocked",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, nil
	default:
		s.log.Warn("Error Unlocking Entity",
			"entity", e.GetID(),
			"authority", getTokenClaims(ctx).EntityID,
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.Empty{}, ErrInternal
	}
}

// EntityGroups returns the full membership for a given entity.
func (s *Server) EntityGroups(ctx context.Context, r *pb.EntityRequest) (*pb.ListOfGroups, error) {
	e := r.GetEntity()

	ent, err := s.FetchEntity(ctx, e.GetID())
	switch err {
	case db.ErrUnknownEntity:
		s.log.Warn("Entity does not exist!",
			"method", "EntityGroups",
			"entity", e.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
		)
		return &pb.ListOfGroups{}, ErrDoesNotExist
	case nil:
		break
	default:
		s.log.Warn("Error getting groups for entity",
			"entity", e.GetID(),
			"service", getServiceName(ctx),
			"client", getClientName(ctx),
			"error", err,
		)
		return &pb.ListOfGroups{}, ErrInternal
	}

	groups := s.GetMemberships(ctx, ent)

	out := make([]*types.Group, len(groups))
	for i := range groups {
		// We throw this error out here, as its logged at a
		// lower level, and the side effect here is that only
		// a partial result gets returned.
		tmp, _ := s.FetchGroup(ctx, groups[i])
		out[i] = tmp
	}

	return &pb.ListOfGroups{Groups: out}, nil
}
