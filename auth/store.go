// Copyright 2016 Nippon Telegraph and Telephone Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"github.com/coreos/etcd/auth/authpb"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	pb "github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/coreos/etcd/storage/backend"
	"github.com/coreos/pkg/capnslog"
	"golang.org/x/crypto/bcrypt"
)

var (
	enableFlagKey       = []byte("authEnabled")
	authBucketName      = []byte("auth")
	authUsersBucketName = []byte("authUsers")

	plog = capnslog.NewPackageLogger("github.com/coreos/etcd", "auth")
)

type AuthStore interface {
	// AuthEnable() turns on the authentication feature
	AuthEnable()

	// Recover recovers the state of auth store from the given backend
	Recover(b backend.Backend)

	// UserAdd adds a new user
	UserAdd(r *pb.AuthUserAddRequest) (*pb.AuthUserAddResponse, error)
}

type authStore struct {
	be backend.Backend
}

func (as *authStore) AuthEnable() {
	value := []byte{1}

	b := as.be
	tx := b.BatchTx()
	tx.Lock()
	tx.UnsafePut(authBucketName, enableFlagKey, value)
	tx.Unlock()
	b.ForceCommit()

	plog.Noticef("Authentication enabled")
}

func (as *authStore) Recover(be backend.Backend) {
	as.be = be
	// TODO(mitake): recovery process
}

func (as *authStore) UserAdd(r *pb.AuthUserAddRequest) (*pb.AuthUserAddResponse, error) {
	plog.Noticef("adding a new user: %s", r.Name)

	hashed, err := bcrypt.GenerateFromPassword([]byte(r.Password), bcrypt.DefaultCost)
	if err != nil {
		plog.Errorf("failed to hash password: %s", err)
		return nil, err
	}

	tx := as.be.BatchTx()
	tx.Lock()
	defer tx.Unlock()

	_, vs := tx.UnsafeRange(authUsersBucketName, []byte(r.Name), nil, 0)
	if len(vs) != 0 {
		return &pb.AuthUserAddResponse{}, rpctypes.ErrUserAlreadyExist
	}

	newUser := authpb.User{
		Name:     []byte(r.Name),
		Password: hashed,
	}

	marshaledUser, merr := newUser.Marshal()
	if merr != nil {
		plog.Errorf("failed to marshal a new user data: %s", merr)
		return nil, merr
	}

	tx.UnsafePut(authUsersBucketName, []byte(r.Name), marshaledUser)

	plog.Noticef("added a new user: %s", r.Name)

	return &pb.AuthUserAddResponse{}, nil
}

func NewAuthStore(be backend.Backend) *authStore {
	tx := be.BatchTx()
	tx.Lock()

	tx.UnsafeCreateBucket(authBucketName)
	tx.UnsafeCreateBucket(authUsersBucketName)

	tx.Unlock()
	be.ForceCommit()

	return &authStore{
		be: be,
	}
}
