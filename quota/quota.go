// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package quota implements per-user/app quota management.
//
// It has a Usage type, that is used to manage generic quotas, and functions
// and methods to interact with the Usage type.
package quota

import (
	"errors"
	"github.com/globocom/tsuru/db"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"sync"
)

var (
	ErrQuotaAlreadyExists = errors.New("Quota already exists")
	ErrQuotaExceeded      = errors.New("Quota exceeded")
	ErrQuotaNotFound      = errors.New("Quota not found")
)

var locker = multiLocker{m: make(map[string]*sync.Mutex)}

// Usage represents the usage of a user/app. It contains information about the
// limit of items, and the current amount of items in use by the user.
type usage struct {
	// Owner identifier (e.g.: the email).
	Owner string
	// Slice of items, each identified by a string.
	Items []string
	// Maximum length of Items.
	Limit uint
	mut   sync.Mutex
}

// Create stores a new quota in the database.
func Create(owner string, quota uint) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.Quota().Insert(usage{Owner: owner, Limit: quota})
	if e, ok := err.(*mgo.LastError); ok && e.Code == 11000 {
		return ErrQuotaAlreadyExists
	}
	return err
}

// Delete destroys the quota allocated for the owner.
func Delete(owner string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	q := map[string]interface{}{"owner": owner}
	err = conn.Quota().Remove(q)
	if err != nil && err.Error() == "not found" {
		return ErrQuotaNotFound
	}
	return err
}

// Reserve increases the number of items in use for the owner.
func Reserve(owner, item string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	locker.Lock(owner)
	defer locker.Unlock(owner)
	var u usage
	err = conn.Quota().Find(bson.M{"owner": owner}).One(&u)
	if err != nil {
		return ErrQuotaNotFound
	}
	if uint(len(u.Items)) == u.Limit {
		return ErrQuotaExceeded
	}
	update := bson.M{"$addToSet": bson.M{"items": item}}
	return conn.Quota().Update(bson.M{"owner": owner}, update)
}

// Release releases the given item from the owner.
//
// It returns an error when the given owner does not exist, and does nothing
// when the given item does not belong to the owner.
func Release(owner, item string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	update := bson.M{"$pull": bson.M{"items": item}}
	err = conn.Quota().Update(bson.M{"owner": owner}, update)
	if err != nil && err.Error() == "not found" {
		return ErrQuotaNotFound
	}
	return err
}

// Set defines a new value for the quota of the given owner.
//
// It allows the database to become in an inconsistent state: a owner may be
// able to have 8 items, and a limit of 7. See the example for more details.
func Set(owner string, value uint) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	update := bson.M{"$set": bson.M{"limit": value}}
	err = conn.Quota().Update(bson.M{"owner": owner}, update)
	if err != nil && err.Error() == "not found" {
		return ErrQuotaNotFound
	}
	return err
}