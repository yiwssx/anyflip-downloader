package main

import (
	"errors"
)

var (
	errAccountNotFound = errors.New("account not found")
	errAccountConflict = errors.New("account state changed")
	errBadCurrentPassword = errors.New("current password invalid")
	errLastActiveAdmin = errors.New("cannot disable last active administrator")
	errSelfDisable = errors.New("cannot disable own administrator account")
)

// revokeUserSessionsLocked must be called with accountAuth.mu held. A user's
// sessions are removed immediately after a successful account change.
func (a *accountAuth) revokeUserSessionsLocked(username string) {
	for digest, session := range a.sessions {
		if session.Username == username {
			delete(a.sessions, digest)
		}
	}
}

// setAccountDisabled never deletes job ownership or the user's history.
// Persist the change before revoking sessions; if persisting fails, restore
// the previous account record, leaving existing sessions untouched.
func (a *accountAuth) setAccountDisabled(actor, username string, disabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	user, exists := a.users[username]
	if !exists {
		return errAccountNotFound
	}
	if disabled && actor == username {
		return errSelfDisable
	}
	if user.Disabled == disabled {
		return nil
	}
	if disabled && user.Role == "admin" {
		activeAdmins := 0
		for _, other := range a.users {
			if other.Role == "admin" && !other.Disabled {
				activeAdmins++
			}
		}
		if activeAdmins <= 1 {
			return errLastActiveAdmin
		}
	}
	previous := user
	user.Disabled = disabled
	a.users[username] = user
	if err := a.saveUsersLocked(); err != nil {
		a.users[username] = previous
		return err
	}
	// Invalidate even when enabling: no old cookie may survive a disable cycle.
	a.revokeUserSessionsLocked(username)
	return nil
}

// resetAccountPassword is an explicitly privileged *administrator* operation.
// It cannot reset the administrator's own password: that requires confirming
// the current credential via changeOwnPassword.
func (a *accountAuth) resetAccountPassword(actor, username, newPassword string) error {
	if actor == username {
		return errAccountConflict
	}
	hash, err := hashAccountPassword(newPassword)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	user, exists := a.users[username]
	if !exists {
		return errAccountNotFound
	}
	previous := user
	user.PasswordHash = hash
	a.users[username] = user
	if err := a.saveUsersLocked(); err != nil {
		a.users[username] = previous
		return err
	}
	a.revokeUserSessionsLocked(username)
	return nil
}

// changeOwnPassword verifies the old credential, protects against concurrent
// resets, and revokes *all* of the account's sessions on success. The current
// browser then has to sign in again using the new password.
func (a *accountAuth) changeOwnPassword(username, oldPassword, newPassword string) error {
	a.mu.Lock()
	before, exists := a.users[username]
	a.mu.Unlock()
	if !exists || before.Disabled || !verifyAccountPassword(before.PasswordHash, oldPassword) {
		return errBadCurrentPassword
	}
	if oldPassword == newPassword {
		return errors.New("new password must differ from the old password")
	}
	hash, err := hashAccountPassword(newPassword)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	user, exists := a.users[username]
	if !exists || user.Disabled || user.PasswordHash != before.PasswordHash {
		return errAccountConflict
	}
	previous := user
	user.PasswordHash = hash
	a.users[username] = user
	if err := a.saveUsersLocked(); err != nil {
		a.users[username] = previous
		return err
	}
	a.revokeUserSessionsLocked(username)
	return nil
}
