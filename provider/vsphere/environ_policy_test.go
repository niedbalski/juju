// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
)

type environPolSuite struct {
	EnvironFixture
}

var _ = gc.Suite(&environPolSuite{})

func (s *environPolSuite) TestConstraintsValidator(c *gc.C) {
	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorEmpty(c *gc.C) {
	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	unsupported, err := validator.Validate(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, gc.HasLen, 0)
}

func (s *environPolSuite) TestConstraintsValidatorUnsupported(c *gc.C) {
	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=amd64 tags=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unsupported, jc.SameContents, []string{"tags", "virt-type"})
}

func (s *environPolSuite) TestConstraintsValidatorVocabArch(c *gc.C) {
	validator, err := s.env.ConstraintsValidator(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Check(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are:.*")
}
