// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/lease"
)

type ExpireSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ExpireSuite{})

func (s *ExpireSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	logger := loggo.GetLogger("juju.worker.lease")
	logger.SetLogLevel(loggo.TRACE)
	logger = loggo.GetLogger("lease_test")
	logger.SetLogLevel(loggo.TRACE)
}

func (s *ExpireSuite) TestStartup_ExpiryInPast(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {Expiry: offset(-time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, _ *testclock.Clock) {})
}

func (s *ExpireSuite) TestStartup_ExpiryInFuture(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {Expiry: offset(time.Second)},
		},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, almostSeconds(1), 1)
	})
}

func (s *ExpireSuite) TestStartup_ExpiryInFuture_TimePasses(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, time.Second, 1)
	})
}

func (s *ExpireSuite) TestStartup_NoExpiry_NotLongEnough(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(_ *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, almostSeconds(3600), 1)
	})
}

func (s *ExpireSuite) TestStartup_NoExpiry_LongEnough(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("goose"): {Expiry: offset(3 * time.Hour)},
		},
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Expiry: offset(time.Minute),
				}
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, time.Hour, 1)
	})
}

func (s *ExpireSuite) TestExpire_ErrInvalid_Expired(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			err:    corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, time.Second, 1)
	})
}

func (s *ExpireSuite) TestAutoexpire(c *gc.C) {
	// Handles the claim, doesn't try to do anything about the expired
	// lease which will go away automatically.
	fix := &Fixture{
		autoexpire: true,
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "postgresql",
				},
				corelease.Request{"postgresql/0", time.Minute},
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, time.Second, 1)
		err := getClaimer(c, manager).Claim("postgresql", "postgresql/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ExpireSuite) TestExpire_ErrInvalid_Updated(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			err:    corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{Expiry: offset(time.Minute)}
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, time.Second, 1)
	})
}

func (s *ExpireSuite) TestExpire_OtherError(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			err:    errors.New("snarfblat hobalob"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, time.Second, 1)
		err := manager.Wait()
		c.Check(err, gc.ErrorMatches, "snarfblat hobalob")
	})
}

func (s *ExpireSuite) TestClaim_ExpiryInFuture(c *gc.C) {
	const newLeaseSecs = 63
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				key("redis"),
				corelease.Request{"redis/0", time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(newLeaseSecs * time.Second),
				}
			},
		}, {
			// We should call Refresh at 1 min because that was the requested
			// time, but we shouldn't expire the lease.
			method: "Refresh",
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// Ask for a minute, actually get 63s. Don't expire early.
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		// One waiters:
		// - the timer in the main loop
		waitAdvance(c, clock, almostSeconds(newLeaseSecs), 1)
	})
}

func (s *ExpireSuite) TestClaim_ExpiryInFuture_TimePasses(c *gc.C) {
	const newLeaseSecs = 63
	firstRelease := make(chan struct{})
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				key("redis"),
				corelease.Request{"redis/0", time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(newLeaseSecs * time.Second),
				}
			},
		}, {
			method: "Refresh",
			callback: func(leases map[corelease.Key]corelease.Info) {
				close(firstRelease)
			},
		}, {
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// Ask for a minute, actually get 63s. Expire on time.
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		// Move forward 1 minute, and we should wake up to Refresh, but this doesn't
		// trigger any expiration. It does cause us to notice that it will expire
		// in 3 more seconds
		waitAdvance(c, clock, time.Minute, 1)
		select {
		case <-firstRelease:
		case <-time.After(testing.LongWait):
			c.Errorf("waited too long")
		}
		// Now we move forward the remaining 3 seconds and wake up to refresh and expire
		waitAdvance(c, clock, 3*time.Second, 1)
	})
}

func (s *ExpireSuite) TestExtend_ExpiryInFuture(c *gc.C) {
	const newLeaseSecs = 63
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args: []interface{}{
				key("redis"),
				corelease.Request{"redis/0", time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(newLeaseSecs * time.Second),
				}
			},
		}, {
			// We do trigger a Refresh, but we don't expire the key.
			method: "Refresh",
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// Ask for a minute, actually get 63s. Don't expire early.
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		// Only the main loop waiter
		waitAdvance(c, clock, almostSeconds(newLeaseSecs), 1)
	})
}

func (s *ExpireSuite) TestExtend_ExpiryInFuture_TimePasses(c *gc.C) {
	const newLeaseSecs = 63
	firstRelease := make(chan struct{})
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args: []interface{}{
				key("redis"),
				corelease.Request{"redis/0", time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(newLeaseSecs * time.Second),
				}
			},
		}, {
			method: "Refresh",
			callback: func(leases map[corelease.Key]corelease.Info) {
				close(firstRelease)
			},
		}, {
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// Ask for a minute, actually get 63s. Expire on time.
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		// Notice the first wakeup, but no expirations, and queued up a second wakeup
		waitAdvance(c, clock, time.Minute, 1)
		select {
		case <-firstRelease:
		case <-time.After(testing.LongWait):
			c.Errorf("waited too long")
		}
		waitAdvance(c, clock, 3*time.Second, 1)
	})
}

func (s *ExpireSuite) TestExpire_Multiple(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
			key("store"): {
				Holder: "store/3",
				Expiry: offset(5 * time.Second),
			},
			key("tokumx"): {
				Holder: "tokumx/5",
				Expiry: offset(10 * time.Second), // will not expire.
			},
			key("ultron"): {
				Holder: "ultron/7",
				Expiry: offset(5 * time.Second),
			},
			key("vvvvvv"): {
				Holder: "vvvvvv/2",
				Expiry: offset(time.Second), // would expire, but errors first.
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("store")},
			err:    corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("store"))
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("ultron")},
			err:    errors.New("what is this?"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		waitAdvance(c, clock, 5*time.Second, 1)
		err := manager.Wait()
		c.Check(err, gc.ErrorMatches, "what is this\\?")
	})
}

func waitAdvance(c *gc.C, clock *testclock.Clock, amount time.Duration, waiters int) {
	err := clock.WaitAdvance(amount, coretesting.LongWait, waiters)
	c.Assert(err, jc.ErrorIsNil)
}
