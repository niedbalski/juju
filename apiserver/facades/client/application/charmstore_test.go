// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	gc "gopkg.in/check.v1"
	charm "gopkg.in/juju/charm.v6"
	charmrepo "gopkg.in/juju/charmrepo.v3"
	"gopkg.in/mgo.v2"

	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/application/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testcharms"
)

type CharmStoreSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CharmStoreSuite{})

func (s *CharmStoreSuite) TestAddCharmWithAuthorization(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cacheDir := c.MkDir()
	s.PatchValue(&charmrepo.CacheDir, cacheDir)

	url := "cs:~juju-qa/bionic/lxd-profile-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockInterface := mocks.NewMockInterface(ctrl)

	charm := testcharms.Repo.CharmArchive(cacheDir, "lxd-profile")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareStoreCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(gomock.Any()).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	iExp := mockInterface.EXPECT()
	iExp.Get(charmURL).Return(charm, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL: url,
	}, func() (charmrepo.Interface, error) {
		return mockInterface, nil
	})
	c.Assert(err, gc.IsNil)
}

func (s *CharmStoreSuite) TestAddCharmWithAuthorizationWithInvalidLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cacheDir := c.MkDir()
	s.PatchValue(&charmrepo.CacheDir, cacheDir)

	url := "cs:~juju-qa/bionic/lxd-profile-fail-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockInterface := mocks.NewMockInterface(ctrl)

	charm := testcharms.Repo.CharmArchive(cacheDir, "lxd-profile-fail")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareStoreCharmUpload(charmURL).Return(mockStateCharm, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	iExp := mockInterface.EXPECT()
	iExp.Get(charmURL).Return(charm, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL: url,
	}, func() (charmrepo.Interface, error) {
		return mockInterface, nil
	})
	c.Assert(err, gc.ErrorMatches, "cannot add charm: invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *CharmStoreSuite) TestAddCharmWithAuthorizationAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cacheDir := c.MkDir()
	s.PatchValue(&charmrepo.CacheDir, cacheDir)

	url := "cs:~juju-qa/bionic/lxd-profile-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockInterface := mocks.NewMockInterface(ctrl)

	charm := testcharms.Repo.CharmArchive(cacheDir, "lxd-profile")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareStoreCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(gomock.Any()).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	iExp := mockInterface.EXPECT()
	iExp.Get(charmURL).Return(charm, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL:   url,
		Force: true,
	}, func() (charmrepo.Interface, error) {
		return mockInterface, nil
	})
	c.Assert(err, gc.IsNil)
}

func (s *CharmStoreSuite) TestAddCharmWithAuthorizationWithInvalidLXDProfileAndForceStilSucceeds(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cacheDir := c.MkDir()
	s.PatchValue(&charmrepo.CacheDir, cacheDir)

	url := "cs:~juju-qa/bionic/lxd-profile-fail-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockInterface := mocks.NewMockInterface(ctrl)

	charm := testcharms.Repo.CharmArchive(cacheDir, "lxd-profile-fail")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareStoreCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(gomock.Any()).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	iExp := mockInterface.EXPECT()
	iExp.Get(charmURL).Return(charm, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL:   url,
		Force: true,
	}, func() (charmrepo.Interface, error) {
		return mockInterface, nil
	})
	c.Assert(err, gc.IsNil)
}

func (s *CharmStoreSuite) TestAddVersionedCharmWithAuthorization(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cacheDir := c.MkDir()
	s.PatchValue(&charmrepo.CacheDir, cacheDir)

	url := "cs:~juju-qa/bionic/versioned-0"
	charmURL, err := charm.ParseURL(url)
	c.Assert(err, gc.IsNil)

	mockState := mocks.NewMockState(ctrl)
	mockStateCharm := mocks.NewMockStateCharm(ctrl)
	mockStorage := mocks.NewMockStorage(ctrl)
	mockInterface := mocks.NewMockInterface(ctrl)

	charm := testcharms.Repo.CharmArchive(cacheDir, "versioned")

	// inject the mock as a back handed dependency
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) storage.Storage {
		return mockStorage
	})

	sExp := mockState.EXPECT()
	sExp.PrepareStoreCharmUpload(charmURL).Return(mockStateCharm, nil)
	sExp.ModelUUID().Return("model-id")
	sExp.MongoSession().Return(&mgo.Session{})
	sExp.UpdateUploadedCharm(charmVersionMatcher{"929903d"}).Return(nil, nil)

	cExp := mockStateCharm.EXPECT()
	cExp.IsUploaded().Return(false)

	stExp := mockStorage.EXPECT()
	stExp.Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	iExp := mockInterface.EXPECT()
	iExp.Get(charmURL).Return(charm, nil)

	err = application.AddCharmWithAuthorizationAndRepo(mockState, params.AddCharmWithAuthorization{
		URL: url,
	}, func() (charmrepo.Interface, error) {
		return mockInterface, nil
	})
	c.Assert(err, gc.IsNil)
}

type charmVersionMatcher struct {
	expVersion string
}

func (m charmVersionMatcher) Matches(x interface{}) bool {
	info, ok := x.(state.CharmInfo)
	if !ok {
		return false
	}

	return info.Version == m.expVersion
}

func (m charmVersionMatcher) String() string {
	return fmt.Sprintf("state.CharmInfo.Version == %q", m.expVersion)
}
