package tree

import (
	pb "github.com/NetAuth/Protocol"
)

// ListGroups literally returns a list of groups
func (m *Manager) ListGroups() ([]*pb.Group, error) {
	names, err := m.db.DiscoverGroupNames()
	if err != nil {
		return nil, err
	}

	groups := []*pb.Group{}
	for _, name := range names {
		g, err := m.db.LoadGroup(name)
		if err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// allEntities is a convenient way to return all the entities
func (m *Manager) allEntities() ([]*pb.Entity, error) {
	var entities []*pb.Entity
	el, err := m.db.DiscoverEntityIDs()
	if err != nil {
		return nil, err
	}
	for _, en := range el {
		e, err := m.db.LoadEntity(en)
		if err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, nil
}
