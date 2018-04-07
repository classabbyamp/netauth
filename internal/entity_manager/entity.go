package entity_manager

import (
	"log"

	"github.com/golang/protobuf/proto"

	"github.com/NetAuth/NetAuth/pkg/errors"
	pb "github.com/NetAuth/NetAuth/pkg/proto"
)

// newEntity creates a new entity given an ID, uidNumber, and secret.
// Its not necessary to set the secret upon creation and it can be set
// later.  If not set on creaction then the entity will not be usable.
// uidNumber must be a unique positive integer.  Because these are
// generally allocated in sequence the special value '-1' may be
// specified which will select the next available number.
func (emds *EMDataStore) newEntity(ID string, uidNumber int32, secret string) error {
	// Does this entity exist already?
	if _, err := emds.db.LoadEntity(ID); err == nil {
		log.Printf("Entity with ID '%s' already exists!", ID)
		return errors.E_DUPLICATE_ID
	}
	if _, err := emds.db.LoadEntityNumber(uidNumber); err == nil {
		log.Printf("Entity with uidNumber '%d' already exists!", uidNumber)
		return errors.E_DUPLICATE_UIDNUMBER
	}

	// Were we given a specific uidNumber?
	if uidNumber == -1 {
		var err error
		// -1 is a sentinel value that tells us to pick the
		// next available number and assign it.
		uidNumber, err = emds.nextUIDNumber()
		if err != nil {
			return err
		}
	}

	// Ok, they don't exist so we'll make them exist now
	newEntity := &pb.Entity{
		ID:        &ID,
		UidNumber: &uidNumber,
		Secret:    &secret,
		Meta:      &pb.EntityMeta{},
	}

	// Save the entity
	if err := emds.db.SaveEntity(newEntity); err != nil {
		return err
	}

	// Now we set the entity secret, this could be inlined, but
	// having it in the seperate function makes resetting the
	// secret trivial.
	if err := emds.setEntitySecretByID(ID, secret); err != nil {
		return err
	}

	// Successfully created we now return no errors
	log.Printf("Created entity '%s'", ID)

	return nil
}

// NewBootstrapEntity is a function that can be called during the
// startup of the srever to create an entity that has the appropriate
// authority to create more entities and otherwise manage the server.
// This can only be called once during startup, attepts to call it
// again will result in no change.  The bootstrap user will always get
// the next available number which in most cases will be 1.
func (emds *EMDataStore) MakeBootstrap(ID string, secret string) {
	if emds.bootstrap_done {
		return
	}

	// In some cases if there is an existing system that has no
	// admin, it is necessary to confer bootstrap powers to an
	// existing user.  In that case they are just selected and
	// then provided the GLOBAL_ROOT capability.
	e, err := emds.db.LoadEntity(ID)
	if err != nil {
		log.Printf("No entity with ID '%s' exists!  Creating...", ID)
	}

	// This is not a normal Go way of doing this, but this
	// function has two possible success cases, the flow may jump
	// in here and return if there is an existing entity to get
	// root powers.
	if e != nil {
		emds.setEntityCapability(e, "GLOBAL_ROOT")
		emds.bootstrap_done = true
		return
	}

	// Even in the bootstrap case its still possible this can
	// fail, in that case its useful to have the error.
	if err := emds.newEntity(ID, -1, secret); err != nil {
		log.Printf("Could not create bootstrap user! (%s)", err)
	}
	if err := emds.setEntityCapabilityByID(ID, "GLOBAL_ROOT"); err != nil {
		log.Printf("Couldn't provide root authority! (%s)", err)
	}

	emds.bootstrap_done = true
}

// DisableBootstrap disables the ability to bootstrap after the
// opportunity to do so has passed.
func (emds *EMDataStore) DisableBootstrap() {
	emds.bootstrap_done = true
}

// DeleteEntityByID deletes the named entity.  This func (emds
// *EMDataStore)tion will delete the entity in a non-atomic way, but
// will ensure that the entity cannot be authenticated with before
// returning.  If the named ID does not exist the function will return
// errors.E_NO_ENTITY, in all other cases nil is returned.
func (emds *EMDataStore) deleteEntityByID(ID string) error {
	if err := emds.db.DeleteEntity(ID); err != nil {
		return err
	}
	log.Printf("Deleted entity '%s'", ID)

	return nil
}

// SetCapability sets a capability on an entity.  The set operation is
// idempotent.
func (emds *EMDataStore) setEntityCapability(e *pb.Entity, c string) error {
	// If no capability was supplied, bail out.
	if len(c) == 0 {
		return nil
	}

	cap := pb.Capability(pb.Capability_value[c])

	for _, a := range e.Meta.Capabilities {
		if a == cap {
			// The entity already has this capability
			// directly, don't add it again.
			return nil
		}
	}

	e.Meta.Capabilities = append(e.Meta.Capabilities, cap)

	if err := emds.db.SaveEntity(e); err != nil {
		return err
	}

	log.Printf("Set capability %s on entity '%s'", c, e.GetID())
	return nil
}

// SetEntityCapabilityByID is a convenience function to get the entity
// and hand it off to the actual setEntityCapability function
func (emds *EMDataStore) setEntityCapabilityByID(ID string, c string) error {
	e, err := emds.db.LoadEntity(ID)
	if err != nil {
		return err
	}

	return emds.setEntityCapability(e, c)
}

// SetEntitySecretByID sets the secret on a given entity using the
// crypto interface.
func (emds *EMDataStore) setEntitySecretByID(ID string, secret string) error {
	e, err := emds.db.LoadEntity(ID)
	if err != nil {
		return err
	}

	ssecret, err := emds.crypto.SecureSecret(secret)
	if err != nil {
		return err
	}
	e.Secret = &ssecret

	if err := emds.db.SaveEntity(e); err != nil {
		return err
	}

	log.Printf("Secret set for '%s'", e.GetID())
	return nil
}

// ValidateSecret validates the identity of an entity by
// validating the authenticating entity with the secret.
func (emds *EMDataStore) ValidateSecret(ID string, secret string) error {
	e, err := emds.db.LoadEntity(ID)
	if err != nil {
		return err
	}

	err = emds.crypto.VerifySecret(secret, *e.Secret)
	if err != nil {
		log.Printf("Failed to authenticate '%s'", e.GetID())
		return errors.E_ENTITY_BADAUTH
	}
	log.Printf("Successfully authenticated '%s'", e.GetID())

	return nil
}

// GetEntity returns an entity to the caller after first making a safe
// copy of it to remove secure fields.
func (emds *EMDataStore) GetEntity(ID string) (*pb.Entity, error) {
	// e will be the direct internal copy, we can't give this back
	// though since it has secrets embedded.
	e, err := emds.db.LoadEntity(ID)
	if err != nil {
		return nil, err
	}

	// The safeCopyEntity will return the entity without secrets
	// in it, as well as an error if there were problems
	// marshaling the proto back and forth.
	return safeCopyEntity(e)
}

func (emds *EMDataStore) updateEntityMeta(e *pb.Entity, newMeta *pb.EntityMeta) error {
	// get the existing metadata
	meta := e.GetMeta()

	// some fields must not be merged in, so we make sure that
	// they're nulled out here
	newMeta.Capabilities = nil
	newMeta.Groups = nil

	// now we can merge the changes, this happens on the live tree
	// and doesn't require recomputing anything since its a change
	// at the leaves since the groups are not permitted to change
	// by this API.
	proto.Merge(meta, newMeta)

	// Save changes
	if err := emds.db.SaveEntity(e); err != nil {
		return err
	}

	log.Printf("Updated metadata for '%s'", e.GetID())
	return nil
}
